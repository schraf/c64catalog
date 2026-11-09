package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestFlexString(t *testing.T) {
	var item archiveItem
	data := `{"identifier":"x","title":["First","Second"],"year":1984,"language":"ger\r","creator":null}`
	if err := json.Unmarshal([]byte(data), &item); err != nil {
		t.Fatal(err)
	}
	if item.Title != "First" || item.Year != "1984" || item.Language != "ger" || item.Creator != "" {
		t.Fatalf("item = %+v", item)
	}
}

func TestArchiveItemName(t *testing.T) {
	fromDescription := archiveItem{
		Title:       "10-Pin Bowling [h CHR]",
		Description: "10 Pin Bowling (1984)(CDS Microsystems)[h CHR] Released 1984",
	}
	if name := fromDescription.name(); name.Title != "10 Pin Bowling" || name.Year != 1984 || len(name.Flags) != 1 {
		t.Fatalf("description name = %+v", name)
	}

	fromFields := archiveItem{
		Title:   "Joe Gunn - Gold 27-08-2007 - English",
		Creator: "Georg Rottensteiner",
		Year:    "2007",
	}
	if name := fromFields.name(); name.Title != "Joe Gunn - Gold 27-08-2007 - English" || name.Year != 2007 || name.Publisher != "Georg Rottensteiner" {
		t.Fatalf("field name = %+v", name)
	}

	fromDate := archiveItem{Title: "Giana Sister's Collection 1 (Disk 1)", Date: "1990-01-01T00:00:00Z"}
	if name := fromDate.name(); name.Title != "Giana Sister's Collection 1" || name.Year != 1990 {
		t.Fatalf("date name = %+v", name)
	}
}

func TestParseItemFiles(t *testing.T) {
	files := []metadataFile{
		{Name: "00_coverscreenshot.jpg", Source: "original"},
		{Name: "00_coverscreenshot_thumb.jpg", Source: "derivative"},
		{Name: "Game_Side_B.d64", Source: "original", Size: "175531"},
		{Name: "Game_Side_A.d64", Source: "original", Size: "174848"},
		{Name: "history/files/Game_Side_B.d64.~1~", Source: "original"},
		{Name: "Game_archive.torrent", Source: "metadata"},
		{Name: "Game_screenshot.gif", Source: "original"},
		{Name: "screenshot_10.jpg", Source: "original"},
		{Name: "screenshot_02.jpg", Source: "original"},
		{Name: "screenshot_02_thumb.jpg", Source: "derivative"},
		{Name: "Game.CRT", Source: "original", Size: "16384"},
	}
	got := parseItemFiles(files)
	want := itemFiles{
		Disks: []itemFile{
			{Name: "Game.CRT", Size: 16384},
			{Name: "Game_Side_A.d64", Size: 174848},
			{Name: "Game_Side_B.d64", Size: 175531},
		},
		Shots: []string{"00_coverscreenshot.jpg", "screenshot_02.jpg", "screenshot_10.jpg"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestMediaType(t *testing.T) {
	tests := map[string]string{
		"a.d64": mediaDisk, "a.G64": mediaDisk, "a.t64": mediaTape, "a.tap": mediaTape,
		"a.crt": mediaCartridge, "a.prg": mediaProgram, "a.jpg": "", "a": "",
	}
	for name, want := range tests {
		if got := mediaType(name); got != want {
			t.Errorf("mediaType(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestNaturalLess(t *testing.T) {
	names := []string{"Disk_10.d64", "disk_2.d64", "Disk_1_Side_B.d64", "Disk_1_Side_A.d64", "Disk_1.d64"}
	sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
	want := "Disk_1.d64,Disk_1_Side_A.d64,Disk_1_Side_B.d64,disk_2.d64,Disk_10.d64"
	if got := strings.Join(names, ","); got != want {
		t.Fatalf("got %s", got)
	}
}

func TestItemCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	empty, err := loadItemCache(dir)
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing cache: %v %v", empty, err)
	}

	cache := itemCache{
		"b": {Disks: []itemFile{{Name: "b.d64", Size: 1}}},
		"a": {Disks: []itemFile{{Name: "a.t64", Size: 2}}, Shots: []string{"screenshot_00.jpg"}},
	}
	if err := cache.write(dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, itemsName))
	if lines := strings.Count(string(raw), "\n"); lines != len(cache)+2 {
		t.Fatalf("items.json has %d lines, want one per item plus header and footer", lines)
	}
	loaded, err := loadItemCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, cache) {
		t.Fatalf("round trip: %+v", loaded)
	}
}

func TestItemCacheFill(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metadata/new/files":
			w.Write([]byte(`{"result":[{"name":"new.d64","source":"original","size":"174848"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cache := itemCache{"cached": {Disks: []itemFile{{Name: "cached.d64"}}}}
	fetched, failed, err := cache.fill(context.Background(), testFetcher(server), []string{"cached", "new", "gone"}, 2, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	if fetched != 1 || failed != 1 {
		t.Fatalf("fetched=%d failed=%d", fetched, failed)
	}
	if got := cache["new"].Disks; len(got) != 1 || got[0].Size != 174848 {
		t.Fatalf("new = %+v", cache["new"])
	}
	if _, ok := cache["gone"]; ok {
		t.Fatal("failed items must not be cached")
	}
}

func TestFetchArchiveItemsFollowsCursorAndDedupes(t *testing.T) {
	makeItems := func(prefix string, n int) []map[string]any {
		var items []map[string]any
		for i := 0; i < n; i++ {
			items = append(items, map[string]any{"identifier": prefix + itoa(i)})
		}
		return items
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "collection:(softwarelibrary_c64_games OR softwarelibrary_c64_cracks)" {
			t.Errorf("q = %q", got)
		}
		if r.URL.Query().Get("cursor") == "" {
			json.NewEncoder(w).Encode(map[string]any{"items": makeItems("p1-", 6000), "cursor": "next"})
			return
		}
		// The second page repeats the last 500 items of the first.
		second := append(makeItems("p1-", 6000)[5500:], makeItems("p2-", 5000)...)
		json.NewEncoder(w).Encode(map[string]any{"items": second})
	}))
	defer server.Close()

	items, err := fetchArchiveItems(context.Background(), testFetcher(server))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 11000 {
		t.Fatalf("got %d items", len(items))
	}
}

func TestFetchArchiveItemsRejectsSmallListing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[{"identifier":"only"}]}`))
	}))
	defer server.Close()
	if _, err := fetchArchiveItems(context.Background(), testFetcher(server)); err == nil {
		t.Fatal("a tiny listing should fail")
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
