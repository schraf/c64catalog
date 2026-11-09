package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const launchBoxFixture = `<?xml version="1.0" standalone="yes"?>
<LaunchBox>
  <Game>
    <Name>Impossible Mission</Name>
    <ReleaseDate>1984-01-01T00:00:00-05:00</ReleaseDate>
    <Overview>Stay a while&nbsp;&mdash; stay forever!</Overview>
    <MaxPlayers>1</MaxPlayers>
    <ReleaseType>Released</ReleaseType>
    <Cooperative>false</Cooperative>
    <DatabaseID>100</DatabaseID>
    <CommunityRating>4.2</CommunityRating>
    <Platform>Commodore 64</Platform>
    <ESRB>Not Rated</ESRB>
    <CommunityRatingCount>55</CommunityRatingCount>
    <Genres>Action; Platform</Genres>
    <Developer>Epyx</Developer>
    <Publisher>Epyx</Publisher>
  </Game>
  <Game>
    <Name>Doom</Name>
    <ReleaseYear>1993</ReleaseYear>
    <DatabaseID>200</DatabaseID>
    <Platform>MS-DOS</Platform>
  </Game>
  <Game>
    <Name>The Great Giana Sisters</Name>
    <ReleaseYear>1987</ReleaseYear>
    <DatabaseID>300</DatabaseID>
    <Platform>Commodore 64</Platform>
    <WikipediaURL>https://en.wikipedia.org/wiki/The_Great_Giana_Sisters</WikipediaURL>
  </Game>
  <GameAlternateName>
    <AlternateName>Giana Sisters</AlternateName>
    <DatabaseID>300</DatabaseID>
  </GameAlternateName>
  <GameAlternateName>
    <AlternateName>The Great Giana Sisters</AlternateName>
    <DatabaseID>300</DatabaseID>
  </GameAlternateName>
  <GameImage>
    <DatabaseID>100</DatabaseID>
    <FileName>box.jpg</FileName>
    <Type>Box - Front</Type>
    <Region>Europe</Region>
  </GameImage>
  <GameImage>
    <DatabaseID>100</DatabaseID>
    <FileName>logo.png</FileName>
    <Type>Clear Logo</Type>
  </GameImage>
  <GameImage>
    <DatabaseID>200</DatabaseID>
    <FileName>doom.jpg</FileName>
    <Type>Box - Front</Type>
  </GameImage>
</LaunchBox>`

func TestParseLaunchBoxGames(t *testing.T) {
	games, err := parseLaunchBoxGames(strings.NewReader(launchBoxFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 {
		t.Fatalf("got %d games, want only the two C64 games", len(games))
	}
	mission, giana := games[0], games[1]
	if mission.Name != "Impossible Mission" || mission.year() != 1984 || mission.RatingCount != 55 {
		t.Fatalf("mission = %+v", mission)
	}
	if !strings.Contains(mission.Overview, "—") {
		t.Fatalf("HTML entities should be decoded: %q", mission.Overview)
	}
	if len(mission.Images) != 1 || mission.Images[0].url() != "https://images.launchbox-app.com/box.jpg" {
		t.Fatalf("images = %+v", mission.Images)
	}
	if giana.year() != 1987 || len(giana.AltNames) != 1 || giana.AltNames[0] != "Giana Sisters" {
		t.Fatalf("giana = %+v", giana)
	}
}

func TestReadLaunchBoxGamesFromZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Metadata.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, _ := archive.Create("Metadata.xml")
	entry.Write([]byte(launchBoxFixture))
	archive.Close()
	file.Close()

	games, err := readLaunchBoxGames(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 {
		t.Fatalf("got %d games", len(games))
	}
}

func TestReadLaunchBoxGamesRequiresMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Other.zip")
	file, _ := os.Create(path)
	archive := zip.NewWriter(file)
	archive.Create("Mame.xml")
	archive.Close()
	file.Close()
	if _, err := readLaunchBoxGames(path); err == nil {
		t.Fatal("a zip without Metadata.xml should fail")
	}
}

func TestReleaseYear(t *testing.T) {
	tests := map[string]int{"1984-01-01T00:00:00-05:00": 1984, "1987": 1987, "": 0, "19xx": 0, "1066": 0}
	for input, want := range tests {
		if got := releaseYear(input); got != want {
			t.Errorf("releaseYear(%q) = %d, want %d", input, got, want)
		}
	}
}
