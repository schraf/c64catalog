package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	games := sampleGames()
	games[0].BoxArt = "images/box/" + games[0].ID + ".jpg"

	if err := writeSnapshot(dir, games, time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, snapshotName))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(raw), "\n"); lines != len(games)+2 {
		t.Fatalf("snapshot has %d lines, want one per game plus header and footer (%d)", lines, len(games)+2)
	}
	if !strings.Contains(string(raw), `"generated_at":"2026-11-08T12:00:00Z"`) {
		t.Fatalf("missing timestamp: %s", raw[:120])
	}

	loaded, err := loadSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, games) {
		t.Fatalf("round trip changed the games:\n got %+v\nwant %+v", loaded[0], games[0])
	}
}

func TestLoadSnapshotErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadSnapshot(dir); err == nil || !strings.Contains(err.Error(), "-refresh") {
		t.Fatalf("missing snapshot should point at -refresh, got %v", err)
	}

	path := filepath.Join(dir, snapshotName)
	os.WriteFile(path, []byte(`{"version":99,"games":[]}`), 0o644)
	if _, err := loadSnapshot(dir); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("wrong version, got %v", err)
	}

	os.WriteFile(path, []byte(`{not json`), 0o644)
	if _, err := loadSnapshot(dir); err == nil {
		t.Fatal("malformed snapshot should fail")
	}
}

func TestAttachExistingArtwork(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string) {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte("jpg"), 0o644)
	}
	write("images/box/a.jpg")
	write("images/shots/a-1.jpg")
	write("images/shots/b-1.jpg")
	write("images/shots/b-2.jpg")

	games := []*Game{
		{ID: "a", BoxArt: "stale.jpg"},
		{ID: "b"},
		{ID: "c", Screenshots: []string{"stale"}},
	}
	attachExistingArtwork(dir, games)

	if games[0].BoxArt != "images/box/a.jpg" || len(games[0].Screenshots) != 1 {
		t.Fatalf("a = %+v", games[0])
	}
	if games[1].BoxArt != "images/shots/b-1.jpg" || len(games[1].Screenshots) != 2 {
		t.Fatalf("b = %+v", games[1])
	}
	if games[2].BoxArt != "" || len(games[2].Screenshots) != 0 {
		t.Fatalf("c = %+v", games[2])
	}
}

func TestLimitMatches(t *testing.T) {
	matched := map[int][]*release{5: {{}}, 1: {{}}, 3: {{}}}
	got := limitMatches(matched, 2)
	if len(got) != 2 || got[1] == nil || got[3] == nil {
		t.Fatalf("got %v", got)
	}
}
