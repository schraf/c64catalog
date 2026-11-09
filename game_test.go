package main

import (
	"strings"
	"testing"
)

func TestTitleKey(t *testing.T) {
	same := [][2]string{
		{"Great Giana Sisters, The", "The Great Giana Sisters"},
		{"A Team, The", "The A-Team"},
		{"Ultima III Exodus", "Ultima III: Exodus"},
		{"Pitfall 2 Lost Caverns", "Pitfall II: Lost Caverns"},
		{"Beach Head 2", "Beach-Head II"},
		{"Commodore 64: Impossible Mission", "Impossible Mission"},
		{"1.000 Miler v2.0", "1.000 Miler"},
		{"Street Fighter US Version", "Street Fighter"},
		{"Spy vs Spy & Friends", "Spy vs Spy and Friends"},
		{"Abadía del Crimen, La", "La Abadia del Crimen"},
		{"Bards Tale", "The Bard's Tale"},
	}
	for _, pair := range same {
		if a, b := titleKey(pair[0]), titleKey(pair[1]); a != b {
			t.Errorf("titleKey(%q) = %q, titleKey(%q) = %q; want equal", pair[0], a, pair[1], b)
		}
	}
	if titleKey("Ultima II") == titleKey("Ultima III") {
		t.Error("sequels must not share a key")
	}
}

func lbFixture() []*lbGame {
	return []*lbGame{
		{DatabaseID: 1, Name: "The Bard's Tale", ReleaseYear: "1985", Publisher: "Electronic Arts", RatingCount: 40},
		{DatabaseID: 2, Name: "Frogger", ReleaseYear: "1983", Publisher: "Parker Brothers", RatingCount: 10},
		{DatabaseID: 3, Name: "Frogger", ReleaseYear: "1984", Publisher: "Carl Muller", RatingCount: 1},
		{DatabaseID: 4, Name: "Indiana Jones and the Last Crusade: The Action Game", ReleaseYear: "1989", Publisher: "U.S. Gold"},
		{DatabaseID: 5, Name: "Indiana Jones and the Last Crusade: The Graphic Adventure", ReleaseYear: "1989", Publisher: "Lucasfilm Games"},
		{DatabaseID: 6, Name: "Karateka", ReleaseYear: "1985"},
		{DatabaseID: 7, Name: "Antiriad", AltNames: []string{"Rad Warrior"}, ReleaseYear: "1986"},
		{DatabaseID: 8, Name: "Summer Games", ReleaseYear: "1984"},
		{DatabaseID: 9, Name: "Summer Games II", ReleaseYear: "1985"},
	}
}

func TestMatcher(t *testing.T) {
	m := newMatcher(lbFixture())
	tests := []struct {
		title     string
		year      int
		publisher string
		want      int
	}{
		{"Bards Tale 1", 1985, "Electronic Arts", 1}, // "1" dropped
		{"Frogger", 1983, "", 2},                     // same year
		{"Frogger", 1984, "", 3},                     // same year beats a year off
		{"Frogger", 0, "", 2},                        // unknown year: best known
		{"Frogger", 1987, "Parker Brothers", 2},      // gap allowed for the same publisher
		{"Frogger", 1990, "Sega", 0},                 // too far apart
		{"Karateka", 1990, "", 6},                    // unique title, small gap
		{"Karateka", 1995, "", 0},                    // unique title, big gap
		{"Indiana Jones and the Last Crusade", 1989, "U.S. Gold", 4},
		{"Indiana Jones and the Last Crusade", 1989, "Unknown", 0},
		{"Rad Warrior", 1986, "", 7}, // alternate name
		{"Summer Games 2", 1985, "", 9},
		{"Summer Games", 1984, "Epyx", 8},
		{"Winter Games", 1985, "", 0},
	}
	for _, tt := range tests {
		got := m.match(tt.title, tt.year, tt.publisher)
		gotID := 0
		if got != nil {
			gotID = got.DatabaseID
		}
		if gotID != tt.want {
			t.Errorf("match(%q, %d, %q) = %d, want %d", tt.title, tt.year, tt.publisher, gotID, tt.want)
		}
	}
}

func item(id, description string, downloads int) archiveItem {
	return archiveItem{Identifier: id, Description: flexString(description), Downloads: downloads, EmulatorStart: flexString(id + ".d64")}
}

func TestGroupReleases(t *testing.T) {
	releases := groupReleases([]archiveItem{
		item("u3b", "Ultima III Exodus (1984)(Origin Systems)(Side B)(Scenario Disk)", 10),
		item("u3a", "Ultima III Exodus (1984)(Origin Systems)(Side A)", 20),
		item("u3cr", "Ultima III Exodus (1984)(Origin Systems)[cr XYZ]", 5),
		{Identifier: "blank"},
	})
	if len(releases) != 2 {
		t.Fatalf("got %d releases", len(releases))
	}
	if ids := releases[0].items[0].Identifier + "," + releases[0].items[1].Identifier; ids != "u3a,u3b" {
		t.Fatalf("sides should be ordered: %s", ids)
	}
}

func TestBuildGames(t *testing.T) {
	lb := []*lbGame{
		{DatabaseID: 10, Name: "Ultima III: Exodus", ReleaseYear: "1984", Genres: "Role-Playing", ReleaseType: "Released",
			Rating: 4.5, RatingCount: 20, ESRB: "Not Rated", MaxPlayers: "1", Wikipedia: "https://example.org/u3", VideoURL: "javascript:alert(1)"},
		{DatabaseID: 11, Name: "Ultima III Exodus", ReleaseYear: "1990"},
		{DatabaseID: 12, Name: "Unmatched", ReleaseYear: "1984"},
	}
	items := []archiveItem{
		item("u3a", "Ultima III Exodus (1984)(Origin Systems)(Side A)", 20),
		item("u3b", "Ultima III Exodus (1984)(Origin Systems)(Side B)(Scenario Disk)", 10),
		item("u3cr", "Ultima III Exodus (1984)(Origin Systems)[cr XYZ]", 500),
		item("u3de", "Ultima III Exodus (1984)(Origin Systems)(de)", 900),
		item("u3late", "Ultima III Exodus (1990)(Origin Systems)", 1),
	}
	cache := itemCache{
		// The first side's item bundles both sides, as many archive items do.
		"u3a":  {Disks: []itemFile{{Name: "U3_Side_A.d64", Size: 100}, {Name: "U3_Side_B.d64", Size: 200}}, Shots: []string{"00_coverscreenshot.jpg"}},
		"u3b":  {Disks: []itemFile{{Name: "U3_Side_B.d64", Size: 200}}},
		"u3cr": {Disks: []itemFile{{Name: "U3_cr.t64", Size: 50}}},
		// u3de and u3late are not cached and fall back to the emulator file.
	}
	matched, stats := matchReleases(groupReleases(items), lb)
	if stats.Releases != 4 || stats.Matched != 4 || stats.Games != 2 {
		t.Fatalf("stats = %+v", stats)
	}

	games := buildGames(lb, matched, cache)
	if len(games) != 2 {
		t.Fatalf("got %d games", len(games))
	}
	u3 := games[0]
	if u3.ID != "ultima-iii-exodus" || games[1].ID != "ultima-iii-exodus-1990" {
		t.Fatalf("ids = %s, %s", u3.ID, games[1].ID)
	}
	if u3.Name != "Ultima III: Exodus" || u3.Genres[0] != "RPG" || u3.Rating != 4.5 || u3.AgeRating != "" || u3.MaxPlayers != 1 {
		t.Fatalf("u3 = %+v", u3)
	}
	if u3.Wikipedia == "" || u3.Video != "" {
		t.Fatalf("links: wikipedia=%q video=%q", u3.Wikipedia, u3.Video)
	}
	if len(u3.Versions) != 3 {
		t.Fatalf("versions = %+v", u3.Versions)
	}

	// The clean English release wins over more popular cracks and translations.
	best := u3.Versions[0]
	if best.Label != "Ultima III Exodus (1984)(Origin Systems)" || best.Play != "https://archive.org/details/u3a" {
		t.Fatalf("default version = %+v", best)
	}
	if len(best.Files) != 2 || best.Files[1].URL != "https://archive.org/download/u3a/U3_Side_B.d64" || best.Downloads != 30 {
		t.Fatalf("default files = %+v", best)
	}
	if u3.Versions[1].Notes[0] != "Cracked by XYZ" || u3.Versions[2].Language != "German" {
		t.Fatalf("version order = %+v", u3.Versions)
	}
	if got := u3.Versions[2].Files[0]; got.Name != "u3de.d64" || got.Size != 0 {
		t.Fatalf("uncached item should use the emulator file: %+v", got)
	}
	if strings.Join(u3.Media, ",") != "Disk,Tape" {
		t.Fatalf("media = %v", u3.Media)
	}
}

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"Impossible Mission":         "impossible-mission",
		"The Bard's Tale":            "the-bards-tale",
		"Spy vs Spy & Friends":       "spy-vs-spy-and-friends",
		"$2000-$3FFF":                "2000-3fff",
		"Ültima: Exodus!":            "ultima-exodus",
		"Lemmings+":                  "lemmings-plus",
		"   ":                        "",
		"Jordan vs Bird: One On One": "jordan-vs-bird-one-on-one",
	}
	for input, want := range tests {
		if got := slugify(input); got != want {
			t.Errorf("slugify(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUniqueID(t *testing.T) {
	used := map[string]bool{}
	a := uniqueID(used, &lbGame{DatabaseID: 1, Name: "Frogger", ReleaseYear: "1983"})
	b := uniqueID(used, &lbGame{DatabaseID: 2, Name: "Frogger", ReleaseYear: "1984"})
	c := uniqueID(used, &lbGame{DatabaseID: 3, Name: "Frogger", ReleaseYear: "1984"})
	d := uniqueID(used, &lbGame{DatabaseID: 4, Name: "???"})
	if a != "frogger" || b != "frogger-1984" || c != "frogger-3" || d != "game" {
		t.Fatalf("ids = %s %s %s %s", a, b, c, d)
	}
}

func TestVersionRanking(t *testing.T) {
	lb := []*lbGame{{DatabaseID: 1, Name: "Pitfall", ReleaseYear: "1984"}}
	names := map[string]string{
		"trainer": "Pitfall (1984)(Activision)[cr Kevlar][t +2 Kevlar]",
		"hack":    "Pitfall (1984)(Activision)[cr Kevlar][h Samar]",
		"crack":   "Pitfall (1984)(Activision)[cr Fairlight]",
		"german":  "Pitfall (1984)(Activision)(de)[cr Zeta]",
		"dump":    "Pitfall (1984)(Activision)",
		"bad":     "Pitfall (1984)(Activision)[cr X][b2]",
	}
	var items []archiveItem
	downloads := 1000
	for id, desc := range names {
		downloads += 100 // later items are more popular, so only the class explains the order
		items = append(items, item(id, desc, downloads))
	}
	matched, _ := matchReleases(groupReleases(items), lb)
	games := buildGames(lb, matched, itemCache{})
	var got []string
	for _, v := range games[0].Versions {
		got = append(got, v.Items[0])
	}
	if want := "dump,crack,hack,trainer,bad,german"; strings.Join(got, ",") != want {
		t.Fatalf("order = %s, want %s", strings.Join(got, ","), want)
	}
}

func TestVersionRankingFallsBackToDownloads(t *testing.T) {
	lb := []*lbGame{{DatabaseID: 1, Name: "Pitfall", ReleaseYear: "1984"}}
	items := []archiveItem{
		item("quiet", "Pitfall (1984)(Activision)[cr Zeta]", 10),
		item("popular", "Pitfall (1984)(Activision)[cr Fairlight]", 500),
	}
	matched, _ := matchReleases(groupReleases(items), lb)
	games := buildGames(lb, matched, itemCache{})
	if games[0].Versions[0].Items[0] != "popular" {
		t.Fatalf("default = %+v", games[0].Versions[0])
	}
}

func TestVersionCap(t *testing.T) {
	lb := []*lbGame{{DatabaseID: 1, Name: "Pitfall", ReleaseYear: "1984"}}
	var items []archiveItem
	for i := 0; i < maxVersions+5; i++ {
		id := "crack" + itoa(i)
		items = append(items, item(id, "Pitfall (1984)(Activision)[cr Group"+itoa(i)+"]", 1000-i))
	}
	matched, _ := matchReleases(groupReleases(items), lb)
	games := buildGames(lb, matched, itemCache{})
	game := games[0]
	if len(game.Versions) != maxVersions || game.ExtraVersions != 5 {
		t.Fatalf("versions = %d, extra = %d", len(game.Versions), game.ExtraVersions)
	}
	if game.Versions[0].Items[0] != "crack0" {
		t.Fatalf("the most popular crack should lead: %+v", game.Versions[0])
	}

	c, err := buildCatalog(games)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Index()[0].Versions; got != maxVersions+5 {
		t.Fatalf("index versions = %d, want the full count", got)
	}
}
