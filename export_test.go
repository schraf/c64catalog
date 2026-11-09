package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func testWebRoot() fstest.MapFS {
	return fstest.MapFS{
		"index.html":      {Data: []byte("<html></html>")},
		"app.js":          {Data: []byte("// app")},
		"style.css":       {Data: []byte("/* css */")},
		"placeholder.svg": {Data: []byte("<svg/>")},
	}
}

func exportFixture(t *testing.T) (dataDir string, games []*Game) {
	t.Helper()
	dataDir = t.TempDir()
	games = sampleGames()[:2]
	for _, game := range games {
		game.BoxArt = "images/box/" + game.ID + ".jpg"
		game.Screenshots = []string{"images/shots/" + game.ID + "-1.jpg"}
		for _, rel := range append([]string{game.BoxArt}, game.Screenshots...) {
			path := filepath.Join(dataDir, filepath.FromSlash(rel))
			os.MkdirAll(filepath.Dir(path), 0o755)
			os.WriteFile(path, []byte("jpeg:"+rel), 0o644)
		}
	}
	return dataDir, games
}

func TestExportStatic(t *testing.T) {
	dataDir, games := exportFixture(t)
	catalog, err := buildCatalog(games)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "dist")
	if err := exportStatic(out, dataDir, testWebRoot(), catalog); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{
		"index.html", ".nojekyll", "static/app.js", "static/style.css", "static/placeholder.svg",
		"data/catalog.json", "data/games/zork.json", "data/games/impossible-mission.json",
		"images/box/zork.jpg", "images/shots/zork-1.jpg", "images/box/impossible-mission.jpg",
	} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "static", "index.html")); err == nil {
		t.Error("index.html must be at the site root, not under static/")
	}

	data, err := os.ReadFile(filepath.Join(out, "data", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var response catalogResponse
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 2 || len(response.Games) != 2 || response.YearMin != 1983 || response.YearMax != 1984 {
		t.Fatalf("response = %+v", response)
	}
	if len(response.Genres) == 0 || len(response.Media) != 2 {
		t.Fatalf("facets missing: %+v", response)
	}

	detail, err := os.ReadFile(filepath.Join(out, "data", "games", "zork.json"))
	if err != nil {
		t.Fatal(err)
	}
	var game Game
	if err := json.Unmarshal(detail, &game); err != nil {
		t.Fatal(err)
	}
	if len(game.Versions) != 1 || game.Versions[0].Files[0].URL == "" || game.Versions[0].Play == "" {
		t.Fatalf("detail = %+v", game)
	}
}

func TestExportStaticReplacesPreviousOutput(t *testing.T) {
	dataDir, games := exportFixture(t)
	catalog, _ := buildCatalog(games)

	out := filepath.Join(t.TempDir(), "dist")
	os.MkdirAll(out, 0o755)
	os.WriteFile(filepath.Join(out, "stale.txt"), []byte("old"), 0o644)

	if err := exportStatic(out, dataDir, testWebRoot(), catalog); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "stale.txt")); err == nil {
		t.Fatal("stale files should be removed")
	}
}

func TestExportStaticFailsOnMissingImage(t *testing.T) {
	dataDir, games := exportFixture(t)
	os.Remove(filepath.Join(dataDir, "images", "box", "zork.jpg"))
	catalog, _ := buildCatalog(games)

	err := exportStatic(filepath.Join(t.TempDir(), "dist"), dataDir, testWebRoot(), catalog)
	if err == nil || !strings.Contains(err.Error(), "zork") {
		t.Fatalf("err = %v", err)
	}
}

func TestExportStaticRefusesUnsafeTargets(t *testing.T) {
	dataDir, games := exportFixture(t)
	catalog, _ := buildCatalog(games)

	for _, target := range []string{".", "..", "/", "../elsewhere"} {
		if err := exportStatic(target, dataDir, testWebRoot(), catalog); err == nil {
			t.Errorf("exporting to %q should be refused", target)
		}
	}

	withGit := t.TempDir()
	os.Mkdir(filepath.Join(withGit, ".git"), 0o755)
	if err := exportStatic(withGit, dataDir, testWebRoot(), catalog); err == nil {
		t.Error("a directory containing .git should be refused")
	}

	withSnapshot := t.TempDir()
	os.WriteFile(filepath.Join(withSnapshot, snapshotName), []byte("{}"), 0o644)
	if err := exportStatic(withSnapshot, dataDir, testWebRoot(), catalog); err == nil {
		t.Error("a directory containing the snapshot should be refused")
	}
}

func TestCopyImageRejectsEscapingPaths(t *testing.T) {
	dataDir := t.TempDir()
	out := t.TempDir()
	for _, rel := range []string{"../secret", "images/../../secret", "other/file.jpg", "/etc/passwd"} {
		if err := copyImage(dataDir, out, rel); err == nil {
			t.Errorf("copyImage(%q) should fail", rel)
		}
	}
}
