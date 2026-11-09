package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

	xdraw "golang.org/x/image/draw"
)

const (
	kindBoxFront = "Box - Front"
	kindBoxRecon = "Box - Front - Reconstructed"
	kindGameplay = "Screenshot - Gameplay"
	kindTitle    = "Screenshot - Game Title"

	boxDir   = "images/box"
	shotsDir = "images/shots"

	maxImageDownload = 64 << 20

	// maxScreenshots is the number of screenshots kept per game.
	maxScreenshots = 2
)

// imageSpec describes how an image is scaled and compressed.
type imageSpec struct {
	MaxWidth  int
	MaxHeight int
	Quality   int
}

var (
	boxSpec  = imageSpec{MaxWidth: 300, MaxHeight: 400, Quality: 75}
	shotSpec = imageSpec{MaxWidth: 480, MaxHeight: 360, Quality: 70}
)

// thumbBackground fills transparent areas of box art.
var thumbBackground = color.RGBA{R: 0x15, G: 0x16, B: 0x1c, A: 0xff}

func regionRank(region string) int {
	switch region {
	case "":
		return 0
	case "Europe":
		return 1
	case "United Kingdom":
		return 2
	case "World":
		return 3
	case "North America":
		return 4
	case "United States":
		return 5
	default:
		return 6
	}
}

// rankedImages returns a game's LaunchBox pictures of one kind, best first:
// preferred regions, then file name for a stable choice. The C64 was mostly a
// European market, so European box art is preferred.
func rankedImages(images []lbImage, kind string) []lbImage {
	var out []lbImage
	for _, img := range images {
		if img.Type == kind {
			out = append(out, img)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := regionRank(a.Region), regionRank(b.Region); ra != rb {
			return ra < rb
		}
		if a.Region != b.Region {
			return a.Region < b.Region
		}
		return a.FileName < b.FileName
	})
	return out
}

// artPlan lists candidate picture URLs for a game, best first. Candidates
// are tried in order until one downloads.
type artPlan struct {
	Box   []string
	Shots []string
}

// planArtwork picks box art and screenshots from LaunchBox. The archive's
// own screenshots are only a fallback: for many cracked releases they are
// captured while the game is still loading and show a blank BASIC screen.
func planArtwork(game *Game, lb *lbGame, cache itemCache) artPlan {
	var plan artPlan
	var archive []string
	if len(game.Versions) > 0 && len(game.Versions[0].Items) > 0 {
		identifier := game.Versions[0].Items[0]
		for _, name := range archiveShotPicks(cache[identifier].Shots) {
			archive = append(archive, downloadURL(identifier, name))
		}
	}

	if lb == nil {
		plan.Shots = archive
		return plan
	}
	for _, kind := range []string{kindBoxFront, kindBoxRecon} {
		for _, img := range rankedImages(lb.Images, kind) {
			plan.Box = append(plan.Box, img.url())
		}
	}

	titles := rankedImages(lb.Images, kindTitle)
	gameplay := rankedImages(lb.Images, kindGameplay)
	first := func(images []lbImage) []lbImage { return images[:min(1, len(images))] }
	rest := func(images []lbImage) []lbImage { return images[min(1, len(images)):] }
	for _, images := range [][]lbImage{first(gameplay), first(titles)} {
		for _, img := range images {
			plan.Shots = append(plan.Shots, img.url())
		}
	}
	plan.Shots = append(plan.Shots, archive...)
	for _, img := range append(rest(gameplay), rest(titles)...) {
		plan.Shots = append(plan.Shots, img.url())
	}
	return plan
}

// archiveShotPicks chooses up to two archive screenshots: the first (the
// archive's cover screenshot when there is one) and one from the middle of
// the emulator captures, which is usually past the loading screens.
func archiveShotPicks(shots []string) []string {
	switch len(shots) {
	case 0:
		return nil
	case 1:
		return shots
	}
	return []string{shots[0], shots[len(shots)/2]}
}

// makeThumbnail decodes a picture and returns it as a JPEG that fits within
// the spec's bounds. Pictures are never enlarged.
func makeThumbnail(data []byte, spec imageSpec) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, errors.New("image is empty")
	}

	targetW, targetH := width, height
	if width > spec.MaxWidth || height > spec.MaxHeight {
		scale := min(float64(spec.MaxWidth)/float64(width), float64(spec.MaxHeight)/float64(height))
		targetW = max(1, int(float64(width)*scale+0.5))
		targetH = max(1, int(float64(height)*scale+0.5))
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: thumbBackground}, image.Point{}, draw.Src)
	if targetW == width && targetH == height {
		draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Over)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, xdraw.Over, nil)
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: spec.Quality}); err != nil {
		return nil, fmt.Errorf("encode thumbnail: %w", err)
	}
	return out.Bytes(), nil
}

// artwork fetches and stores thumbnails under dataDir/images.
type artwork struct {
	fetcher *fetcher
	dataDir string
	force   bool
	workers int
	logf    func(string, ...any)
}

type artworkStats struct {
	Boxes       int
	Shots       int
	Downloaded  int
	Failed      int
	NoArtwork   int
	BoxFallback int
}

func (s *artworkStats) add(o artworkStats) {
	s.Boxes += o.Boxes
	s.Shots += o.Shots
	s.Downloaded += o.Downloaded
	s.Failed += o.Failed
	s.NoArtwork += o.NoArtwork
	s.BoxFallback += o.BoxFallback
}

// fill downloads what is missing for every game and records the thumbnail
// paths on the games. It never fails the whole run for a single bad picture;
// those games are left without that image and retried next time.
func (a *artwork) fill(ctx context.Context, games []*Game, plans map[string]artPlan) (artworkStats, error) {
	jobs := make(chan *Game)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var stats artworkStats
	var done atomic.Int64
	total := len(games)

	workers := max(1, a.workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for game := range jobs {
				local := a.fillGame(ctx, game, plans[game.ID])
				mu.Lock()
				stats.add(local)
				mu.Unlock()
				if n := done.Add(1); n%250 == 0 || int(n) == total {
					a.logf("images: %d/%d games", n, total)
				}
			}
		}()
	}
feed:
	for _, game := range games {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- game:
		}
	}
	close(jobs)
	wg.Wait()
	return stats, ctx.Err()
}

func (a *artwork) fillGame(ctx context.Context, game *Game, plan artPlan) artworkStats {
	var stats artworkStats
	game.BoxArt = ""
	game.Screenshots = nil

	rel := path.Join(boxDir, game.ID+".jpg")
	if downloaded, ok := a.first(ctx, game.ID, "box", plan.Box, rel, boxSpec, &stats); ok {
		game.BoxArt = rel
		stats.Boxes++
		if downloaded {
			stats.Downloaded++
		}
	}

	next := 0
	for i := 1; i <= maxScreenshots && next < len(plan.Shots); i++ {
		rel := path.Join(shotsDir, fmt.Sprintf("%s-%d.jpg", game.ID, i))
		// Each slot takes the next candidate that works.
		for next < len(plan.Shots) {
			url := plan.Shots[next]
			next++
			downloaded, err := a.ensure(ctx, url, rel, shotSpec)
			if err != nil {
				a.logf("images: %s screenshot %d: %v", game.ID, i, err)
				stats.Failed++
				continue
			}
			game.Screenshots = append(game.Screenshots, rel)
			stats.Shots++
			if downloaded {
				stats.Downloaded++
			}
			break
		}
	}

	// Games without a box picture use their first screenshot as tile art.
	if game.BoxArt == "" && len(game.Screenshots) > 0 {
		game.BoxArt = game.Screenshots[0]
		stats.BoxFallback++
	}
	if game.BoxArt == "" {
		stats.NoArtwork++
	}
	return stats
}

// first stores the first candidate that downloads and decodes.
func (a *artwork) first(ctx context.Context, id, what string, urls []string, rel string, spec imageSpec, stats *artworkStats) (downloaded, ok bool) {
	for _, url := range urls {
		downloaded, err := a.ensure(ctx, url, rel, spec)
		if err == nil {
			return downloaded, true
		}
		a.logf("images: %s %s: %v", id, what, err)
		stats.Failed++
	}
	return false, false
}

// ensure makes sure rel exists under the data directory, downloading and
// scaling the source picture when needed. It reports whether it downloaded.
func (a *artwork) ensure(ctx context.Context, url, rel string, spec imageSpec) (bool, error) {
	dest := filepath.Join(a.dataDir, filepath.FromSlash(rel))
	if !a.force {
		if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
			return false, nil
		}
	}

	data, err := a.fetcher.getBytes(ctx, url, maxImageDownload)
	if err != nil {
		return false, err
	}
	thumb, err := makeThumbnail(data, spec)
	if err != nil {
		return false, fmt.Errorf("%s: %w", url, err)
	}
	if err := writeFileAtomic(dest, thumb, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func logArtworkStats(stats artworkStats, games int) {
	log.Printf("images: %d games; %d box pictures, %d screenshots (%d new downloads, %d failed); %d use a screenshot as box art, %d have no artwork",
		games, stats.Boxes, stats.Shots, stats.Downloaded, stats.Failed, stats.BoxFallback, stats.NoArtwork)
}
