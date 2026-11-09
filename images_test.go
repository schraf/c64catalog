package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func testPNG(t *testing.T, width, height int, c color.Color) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeJPEG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("output is not a JPEG: %v", err)
	}
	return img
}

func TestMakeThumbnailScalesDownKeepingAspectRatio(t *testing.T) {
	data := testPNG(t, 800, 1200, color.NRGBA{R: 200, G: 40, B: 40, A: 255})
	out, err := makeThumbnail(data, boxSpec)
	if err != nil {
		t.Fatal(err)
	}
	bounds := decodeJPEG(t, out).Bounds()
	if bounds.Dx() != 267 || bounds.Dy() != 400 {
		t.Fatalf("size = %dx%d, want 267x400", bounds.Dx(), bounds.Dy())
	}
}

func TestMakeThumbnailNeverEnlarges(t *testing.T) {
	data := testPNG(t, 320, 200, color.NRGBA{R: 10, G: 200, B: 10, A: 255})
	out, err := makeThumbnail(data, shotSpec)
	if err != nil {
		t.Fatal(err)
	}
	bounds := decodeJPEG(t, out).Bounds()
	if bounds.Dx() != 320 || bounds.Dy() != 200 {
		t.Fatalf("size = %dx%d, want 320x200", bounds.Dx(), bounds.Dy())
	}
}

func TestMakeThumbnailFlattensTransparency(t *testing.T) {
	data := testPNG(t, 40, 40, color.NRGBA{A: 0})
	out, err := makeThumbnail(data, boxSpec)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := decodeJPEG(t, out).At(20, 20).RGBA()
	// Fully transparent pixels take the dark thumbnail background.
	if r>>8 > 0x40 || g>>8 > 0x40 || b>>8 > 0x40 {
		t.Fatalf("transparent area is not dark: %d %d %d", r>>8, g>>8, b>>8)
	}
}

func TestMakeThumbnailDecodesGIF(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	img := image.NewPaletted(image.Rect(0, 0, 640, 400), palette)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	out, err := makeThumbnail(buf.Bytes(), shotSpec)
	if err != nil {
		t.Fatal(err)
	}
	if bounds := decodeJPEG(t, out).Bounds(); bounds.Dx() != 480 || bounds.Dy() != 300 {
		t.Fatalf("size = %dx%d, want 480x300", bounds.Dx(), bounds.Dy())
	}
}

func TestMakeThumbnailRejectsGarbage(t *testing.T) {
	if _, err := makeThumbnail([]byte("not an image"), boxSpec); err == nil {
		t.Fatal("expected a decode error")
	}
}

func TestRankedImages(t *testing.T) {
	images := []lbImage{
		{FileName: "us.jpg", Type: kindBoxFront, Region: "United States"},
		{FileName: "logo.png", Type: "Clear Logo"},
		{FileName: "eu.jpg", Type: kindBoxFront, Region: "Europe"},
		{FileName: "none.jpg", Type: kindBoxFront},
		{FileName: "jp.jpg", Type: kindBoxFront, Region: "Japan"},
	}
	var names []string
	for _, img := range rankedImages(images, kindBoxFront) {
		names = append(names, img.FileName)
	}
	if got := strings.Join(names, ","); got != "none.jpg,eu.jpg,us.jpg,jp.jpg" {
		t.Fatalf("order = %s", got)
	}
}

func TestArchiveShotPicks(t *testing.T) {
	if got := archiveShotPicks(nil); got != nil {
		t.Fatalf("got %v", got)
	}
	if got := archiveShotPicks([]string{"a"}); len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	shots := []string{"00_coverscreenshot.jpg", "s0", "s1", "s2", "s3"}
	if got := strings.Join(archiveShotPicks(shots), ","); got != "00_coverscreenshot.jpg,s1" {
		t.Fatalf("got %s", got)
	}
}

func TestPlanArtwork(t *testing.T) {
	game := &Game{ID: "im", Versions: []Version{{Items: []string{"im_item"}}}}
	lb := &lbGame{Images: []lbImage{
		{FileName: "recon.jpg", Type: kindBoxRecon},
		{FileName: "box.jpg", Type: kindBoxFront},
		{FileName: "title.jpg", Type: kindTitle},
		{FileName: "play1.jpg", Type: kindGameplay},
		{FileName: "play2.jpg", Type: kindGameplay},
	}}
	cache := itemCache{"im_item": {Shots: []string{"00_coverscreenshot.jpg", "screenshot_00.jpg", "screenshot_01.jpg"}}}

	plan := planArtwork(game, lb, cache)
	box := strings.Join(plan.Box, " ")
	if box != "https://images.launchbox-app.com/box.jpg https://images.launchbox-app.com/recon.jpg" {
		t.Fatalf("box = %s", box)
	}
	want := []string{
		"https://images.launchbox-app.com/play1.jpg",
		"https://images.launchbox-app.com/title.jpg",
		"https://archive.org/download/im_item/00_coverscreenshot.jpg",
		"https://archive.org/download/im_item/screenshot_00.jpg",
		"https://images.launchbox-app.com/play2.jpg",
	}
	if strings.Join(plan.Shots, " ") != strings.Join(want, " ") {
		t.Fatalf("shots = %v", plan.Shots)
	}

	// Without LaunchBox pictures the archive's screenshots are used.
	noLB := planArtwork(game, nil, cache)
	if len(noLB.Box) != 0 || len(noLB.Shots) != 2 || !strings.Contains(noLB.Shots[0], "00_coverscreenshot.jpg") {
		t.Fatalf("no LaunchBox plan = %+v", noLB)
	}

	if plan := planArtwork(&Game{ID: "x"}, nil, cache); len(plan.Box) != 0 || len(plan.Shots) != 0 {
		t.Fatalf("empty plan = %+v", plan)
	}
}

func TestArtworkFill(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch path.Base(r.URL.Path) {
		case "box.jpg":
			w.Write(testPNG(t, 600, 800, color.NRGBA{R: 255, A: 255}))
		case "shot.jpg", "shot2.jpg":
			w.Write(testPNG(t, 384, 272, color.NRGBA{B: 255, A: 255}))
		case "garbage.jpg":
			w.Write([]byte("not an image"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	art := &artwork{fetcher: testFetcher(server), dataDir: dir, workers: 2, logf: t.Logf}
	games := []*Game{{ID: "full"}, {ID: "shots-only"}, {ID: "nothing"}}
	plans := map[string]artPlan{
		"full": {
			Box:   []string{"https://images.launchbox-app.com/missing.jpg", "https://images.launchbox-app.com/box.jpg"},
			Shots: []string{"https://archive.org/download/x/garbage.jpg", "https://archive.org/download/x/shot.jpg", "https://archive.org/download/x/shot2.jpg"},
		},
		"shots-only": {Shots: []string{"https://archive.org/download/y/shot.jpg"}},
	}

	stats, err := art.fill(context.Background(), games, plans)
	if err != nil {
		t.Fatal(err)
	}
	if games[0].BoxArt != "images/box/full.jpg" || len(games[0].Screenshots) != 2 {
		t.Fatalf("full = %+v", games[0])
	}
	if games[1].BoxArt != "images/shots/shots-only-1.jpg" || len(games[1].Screenshots) != 1 {
		t.Fatalf("shots-only = %+v", games[1])
	}
	if games[2].BoxArt != "" || games[2].Screenshots != nil {
		t.Fatalf("nothing = %+v", games[2])
	}
	if stats.Boxes != 1 || stats.Shots != 3 || stats.Failed != 2 || stats.BoxFallback != 1 || stats.NoArtwork != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	if _, err := os.Stat(filepath.Join(dir, "images", "box", "full.jpg")); err != nil {
		t.Fatal(err)
	}

	// A second run reuses the thumbnails on disk.
	before := requests.Load()
	if _, err := art.fill(context.Background(), games, plans); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != before {
		t.Fatalf("second run made %d requests", requests.Load()-before)
	}
}
