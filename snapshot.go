package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	snapshotName    = "catalog.json"
	snapshotVersion = 1
)

type snapshot struct {
	Version     int     `json:"version"`
	GeneratedAt string  `json:"generated_at"`
	Games       []*Game `json:"games"`
}

// writeSnapshot stores the games with one game per line, which keeps diffs
// between refreshes readable.
func writeSnapshot(dataDir string, games []*Game, now time.Time) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "{\"version\":%d,\"generated_at\":%q,\"games\":[\n", snapshotVersion, now.UTC().Format(time.RFC3339))
	for i, game := range games {
		data, err := json.Marshal(game)
		if err != nil {
			return fmt.Errorf("encode %s: %w", game.ID, err)
		}
		buf.Write(data)
		if i < len(games)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("]}\n")
	return writeFileAtomic(filepath.Join(dataDir, snapshotName), buf.Bytes(), 0o644)
}

func loadSnapshot(dataDir string) ([]*Game, error) {
	file := filepath.Join(dataDir, snapshotName)
	data, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s does not exist; run with -refresh to create it", file)
		}
		return nil, err
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}
	if snap.Version != snapshotVersion {
		return nil, fmt.Errorf("%s has version %d; this build reads version %d, run with -refresh", file, snap.Version, snapshotVersion)
	}
	return snap.Games, nil
}

type refreshOptions struct {
	DataDir       string
	LaunchBoxPath string
	UseImages     bool
	ForceImages   bool
	Workers       int
	Limit         int
}

// refresh rebuilds the snapshot from the Internet Archive and LaunchBox and
// writes it to the data directory.
func refresh(ctx context.Context, f *fetcher, opts refreshOptions, logf func(string, ...any)) error {
	logf("listing %s", strings.Join(collectionIDs, ", "))
	items, err := fetchArchiveItems(ctx, f)
	if err != nil {
		return err
	}
	releases := groupReleases(items)
	logf("archive: %d items in %d releases", len(items), len(releases))

	logf("reading LaunchBox metadata")
	lb, err := fetchLaunchBoxGames(ctx, f, opts.LaunchBoxPath)
	if err != nil {
		return err
	}
	if len(lb) < minLaunchBoxGames {
		return fmt.Errorf("LaunchBox lists only %d %s games; has the export changed?", len(lb), launchBoxPlatform)
	}
	logf("launchbox: %d %s games", len(lb), launchBoxPlatform)

	matched, stats := matchReleases(releases, lb)
	logf("matching: %d of %d releases matched %d LaunchBox games", stats.Matched, stats.Releases, stats.Games)

	if opts.Limit > 0 && opts.Limit < len(matched) {
		matched = limitMatches(matched, opts.Limit)
		logf("limiting to %d games", len(matched))
	}

	cache, err := loadItemCache(opts.DataDir)
	if err != nil {
		return err
	}
	fetched, failed, err := cache.fill(ctx, f, matchedItems(matched), opts.Workers, logf)
	if err != nil {
		return err
	}
	if fetched > 0 || failed > 0 {
		logf("items: %d file lists fetched, %d failed", fetched, failed)
	}
	if err := cache.write(opts.DataDir); err != nil {
		return err
	}

	games := buildGames(lb, matched, cache)
	if len(games) == 0 {
		return errors.New("no games have downloadable files")
	}
	logf("catalog: %d games, %d versions", len(games), countVersions(games))

	if opts.UseImages {
		byID := map[int]*lbGame{}
		for _, game := range lb {
			byID[game.DatabaseID] = game
		}
		plans := map[string]artPlan{}
		for _, game := range games {
			plans[game.ID] = planArtwork(game, byID[game.LaunchBoxID], cache)
		}
		art := &artwork{
			fetcher: f,
			dataDir: opts.DataDir,
			force:   opts.ForceImages,
			workers: opts.Workers,
			logf:    logf,
		}
		stats, err := art.fill(ctx, games, plans)
		logArtworkStats(stats, len(games))
		if err != nil {
			return err
		}
	} else {
		attachExistingArtwork(opts.DataDir, games)
	}

	if err := writeSnapshot(opts.DataDir, games, time.Now()); err != nil {
		return err
	}
	logf("wrote %s", filepath.Join(opts.DataDir, snapshotName))
	return nil
}

// limitMatches keeps the n LaunchBox games with the lowest IDs, for trial
// runs.
func limitMatches(matched map[int][]*release, n int) map[int][]*release {
	ids := make([]int, 0, len(matched))
	for id := range matched {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	out := make(map[int][]*release, n)
	for _, id := range ids[:n] {
		out[id] = matched[id]
	}
	return out
}

func matchedItems(matched map[int][]*release) []string {
	var ids []string
	for _, releases := range matched {
		for _, rel := range releases {
			for _, item := range rel.items {
				ids = append(ids, item.Identifier)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

func countVersions(games []*Game) int {
	n := 0
	for _, game := range games {
		n += len(game.Versions)
	}
	return n
}

// attachExistingArtwork records thumbnails that are already on disk, for
// refreshes that skip downloading images.
func attachExistingArtwork(dataDir string, games []*Game) {
	exists := func(rel string) bool {
		info, err := os.Stat(filepath.Join(dataDir, filepath.FromSlash(rel)))
		return err == nil && info.Size() > 0
	}
	for _, game := range games {
		game.BoxArt = ""
		game.Screenshots = nil
		if rel := path.Join(boxDir, game.ID+".jpg"); exists(rel) {
			game.BoxArt = rel
		}
		for i := 1; i <= maxScreenshots; i++ {
			if rel := path.Join(shotsDir, game.ID+"-"+strconv.Itoa(i)+".jpg"); exists(rel) {
				game.Screenshots = append(game.Screenshots, rel)
			}
		}
		if game.BoxArt == "" && len(game.Screenshots) > 0 {
			game.BoxArt = game.Screenshots[0]
		}
	}
}
