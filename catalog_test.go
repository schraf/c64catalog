package main

import (
	"math"
	"strings"
	"testing"
)

func testVersion(id string, media string) []Version {
	ext := map[string]string{mediaDisk: ".d64", mediaTape: ".t64", mediaCartridge: ".crt"}[media]
	return []Version{{
		Label: id, Media: media, Play: detailsURL(id), Items: []string{id},
		Files: []FileRef{{Name: id + ext, Size: 174848, URL: downloadURL(id, id+ext)}},
	}}
}

func sampleGames() []*Game {
	return []*Game{
		{ID: "zork", Name: "Zork", ReleaseYear: 1983, Genres: []string{"Interactive Fiction"}, Source: "Released",
			Media: []string{mediaDisk}, Rating: 4.0, RatingCount: 30, Versions: testVersion("zork", mediaDisk)},
		{ID: "impossible-mission", Name: "Impossible Mission", ReleaseYear: 1984, Genres: []string{"Action", "Platformer"}, Source: "Released",
			Media: []string{mediaDisk, mediaTape}, Rating: 4.6, RatingCount: 147, Versions: testVersion("im", mediaDisk)},
		{ID: "mystery", Name: "Mystery", ReleaseYear: 0, Genres: []string{otherGenre}, Source: unknownSource,
			Media: []string{mediaTape}, Versions: testVersion("mystery", mediaTape)},
		{ID: "homebrew", Name: "alpha homebrew", ReleaseYear: 2019, Genres: []string{"Action"}, Source: "Homebrew",
			Media: []string{mediaCartridge}, Rating: 5.0, RatingCount: 1, Versions: testVersion("hb", mediaCartridge)},
		{ID: "frogger-1984", Name: "Frogger", ReleaseYear: 1984, Genres: []string{"Arcade"}, Source: "Released",
			Media: []string{mediaDisk}, Versions: testVersion("f84", mediaDisk)},
		{ID: "frogger", Name: "Frogger", ReleaseYear: 1983, Genres: []string{"Arcade"}, Source: "Released",
			Media: []string{mediaDisk}, Versions: testVersion("f83", mediaDisk)},
	}
}

func TestBuildCatalogOrdersByNameThenID(t *testing.T) {
	c, err := buildCatalog(sampleGames())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, game := range c.games {
		names = append(names, game.ID)
	}
	want := "homebrew,frogger,frogger-1984,impossible-mission,mystery,zork"
	if got := strings.Join(names, ","); got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
}

func TestBuildCatalogValidates(t *testing.T) {
	versions := testVersion("x", mediaDisk)
	if _, err := buildCatalog(nil); err == nil {
		t.Fatal("empty catalog should fail")
	}
	if _, err := buildCatalog([]*Game{{ID: "", Name: "x", Versions: versions}}); err == nil {
		t.Fatal("empty ID should fail")
	}
	if _, err := buildCatalog([]*Game{{ID: "a", Name: "x", Versions: versions}, {ID: "a", Name: "y", Versions: versions}}); err == nil {
		t.Fatal("duplicate ID should fail")
	}
	if _, err := buildCatalog([]*Game{{ID: "a", Name: "x"}}); err == nil {
		t.Fatal("a game without versions should fail")
	}
}

func TestCatalogFacets(t *testing.T) {
	c, err := buildCatalog(sampleGames())
	if err != nil {
		t.Fatal(err)
	}
	if c.yearMin != 1983 || c.yearMax != 2019 {
		t.Fatalf("years = %d-%d; games without a year must not count", c.yearMin, c.yearMax)
	}

	genres := make([]string, len(c.genres))
	for i, facet := range c.genres {
		genres[i] = facet.Name
	}
	// Action and Arcade have two games each; "Other" always sorts last.
	if got := strings.Join(genres, ","); got != "Action,Arcade,Interactive Fiction,Platformer,Other" {
		t.Fatalf("genres = %s", got)
	}

	var media []string
	for _, facet := range c.media {
		media = append(media, facet.Name+":"+itoa(facet.Count))
	}
	if got := strings.Join(media, ","); got != "Disk:4,Tape:2,Cartridge:1" {
		t.Fatalf("media = %s", got)
	}
}

func TestWeightedRatings(t *testing.T) {
	c, err := buildCatalog(sampleGames())
	if err != nil {
		t.Fatal(err)
	}
	mean := (4.0 + 4.6 + 5.0) / 3
	want := (147*4.6 + priorWeight*mean) / (147 + priorWeight)
	if got := c.weighted["impossible-mission"]; math.Abs(got-want) > 1e-9 {
		t.Fatalf("weighted = %v, want %v", got, want)
	}
	if c.weighted["impossible-mission"] <= c.weighted["homebrew"] {
		t.Fatal("one 5-star vote must not outrank a well-supported 4.6")
	}
	if _, ok := c.weighted["mystery"]; ok {
		t.Fatal("unrated games have no weighted rating")
	}
}

func TestIndex(t *testing.T) {
	games := sampleGames()
	games[1].BoxArt = "images/box/impossible-mission.jpg"
	games[1].AltNames = []string{"Mission Impossible"}
	games[1].Versions = append(games[1].Versions, testVersion("im2", mediaTape)...)
	c, err := buildCatalog(games)
	if err != nil {
		t.Fatal(err)
	}
	var im IndexGame
	for _, game := range c.Index() {
		if game.ID == "impossible-mission" {
			im = game
		}
	}
	if im.Year != 1984 || im.BoxArt == "" || im.WeightedRating == 0 || len(im.AltNames) != 1 || im.Versions != 2 || len(im.Media) != 2 {
		t.Fatalf("index = %+v", im)
	}
	if c.Total() != len(games) || len(c.Index()) != len(games) {
		t.Fatalf("total = %d", c.Total())
	}
}
