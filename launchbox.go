package main

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	launchBoxMetadataURL = "https://gamesdb.launchbox-app.com/Metadata.zip"
	launchBoxImageBase   = "https://images.launchbox-app.com/"
	launchBoxPlatform    = "Commodore 64"

	// minLaunchBoxGames guards against a changed export. LaunchBox lists
	// about 14,000 C64 games.
	minLaunchBoxGames = 3000
)

// lbGame is a Commodore 64 game in the LaunchBox Games Database.
type lbGame struct {
	DatabaseID  int     `xml:"DatabaseID"`
	Name        string  `xml:"Name"`
	ReleaseDate string  `xml:"ReleaseDate"`
	ReleaseYear string  `xml:"ReleaseYear"`
	Overview    string  `xml:"Overview"`
	MaxPlayers  string  `xml:"MaxPlayers"`
	ReleaseType string  `xml:"ReleaseType"`
	Cooperative string  `xml:"Cooperative"`
	VideoURL    string  `xml:"VideoURL"`
	Wikipedia   string  `xml:"WikipediaURL"`
	Rating      float64 `xml:"CommunityRating"`
	RatingCount int     `xml:"CommunityRatingCount"`
	Platform    string  `xml:"Platform"`
	ESRB        string  `xml:"ESRB"`
	Genres      string  `xml:"Genres"`
	Developer   string  `xml:"Developer"`
	Publisher   string  `xml:"Publisher"`

	AltNames []string  `xml:"-"`
	Images   []lbImage `xml:"-"`
}

// year is the game's release year, from the date or the year field.
func (g *lbGame) year() int {
	if year := releaseYear(g.ReleaseDate); year > 0 {
		return year
	}
	return releaseYear(g.ReleaseYear)
}

// lbImage is one picture of a game in the LaunchBox image CDN.
type lbImage struct {
	DatabaseID int    `xml:"DatabaseID"`
	FileName   string `xml:"FileName"`
	Type       string `xml:"Type"`
	Region     string `xml:"Region"`
}

func (img lbImage) url() string {
	return launchBoxImageBase + escapePath(img.FileName)
}

type lbAlternateName struct {
	AlternateName string `xml:"AlternateName"`
	DatabaseID    int    `xml:"DatabaseID"`
}

// usefulImageTypes are the LaunchBox picture types the catalog can use.
var usefulImageTypes = map[string]bool{
	kindBoxFront: true,
	kindBoxRecon: true,
	kindGameplay: true,
	kindTitle:    true,
}

// fetchLaunchBoxGames returns LaunchBox's Commodore 64 games with their
// alternate names and pictures. A local Metadata.xml or Metadata.zip can be
// supplied to skip the ~110 MB download.
func fetchLaunchBoxGames(ctx context.Context, f *fetcher, localPath string) ([]*lbGame, error) {
	path := localPath
	if path == "" {
		var err error
		path, err = f.download(ctx, launchBoxMetadataURL, "", "launchbox-metadata-*.zip")
		if err != nil {
			return nil, fmt.Errorf("download LaunchBox metadata: %w", err)
		}
		defer os.Remove(path)
	}
	return readLaunchBoxGames(path)
}

func readLaunchBoxGames(path string) ([]*lbGame, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		return parseLaunchBoxGames(file)
	}

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(file, info.Size())
	if err != nil {
		return nil, fmt.Errorf("open LaunchBox archive: %w", err)
	}
	for _, entry := range archive.File {
		if filepath.Base(entry.Name) != "Metadata.xml" {
			continue
		}
		body, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("open LaunchBox Metadata.xml: %w", err)
		}
		defer body.Close()
		return parseLaunchBoxGames(body)
	}
	return nil, errors.New("LaunchBox archive does not contain Metadata.xml")
}

// parseLaunchBoxGames streams Metadata.xml (about 500 MB) and keeps the
// Commodore 64 games. Alternate names and pictures are listed separately
// from the games, so they are collected for every game and attached at the
// end, without relying on the order of the file.
func parseLaunchBoxGames(r io.Reader) ([]*lbGame, error) {
	decoder := xml.NewDecoder(r)
	// Overviews contain HTML entities such as &nbsp; and &mdash;.
	decoder.Strict = false
	decoder.Entity = xml.HTMLEntity

	var games []*lbGame
	alternates := map[int][]string{}
	images := map[int][]lbImage{}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse LaunchBox metadata: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "Game":
			var game lbGame
			if err := decoder.DecodeElement(&game, &start); err != nil {
				return nil, fmt.Errorf("parse LaunchBox game: %w", err)
			}
			if strings.TrimSpace(game.Platform) == launchBoxPlatform && game.DatabaseID > 0 && strings.TrimSpace(game.Name) != "" {
				games = append(games, &game)
			}
		case "GameAlternateName":
			var alt lbAlternateName
			if err := decoder.DecodeElement(&alt, &start); err != nil {
				return nil, fmt.Errorf("parse LaunchBox alternate name: %w", err)
			}
			if name := strings.TrimSpace(alt.AlternateName); name != "" && alt.DatabaseID > 0 {
				alternates[alt.DatabaseID] = append(alternates[alt.DatabaseID], name)
			}
		case "GameImage":
			var img lbImage
			if err := decoder.DecodeElement(&img, &start); err != nil {
				return nil, fmt.Errorf("parse LaunchBox image: %w", err)
			}
			img.Type = strings.TrimSpace(img.Type)
			img.Region = strings.TrimSpace(img.Region)
			img.FileName = strings.TrimSpace(img.FileName)
			if usefulImageTypes[img.Type] && img.FileName != "" && img.DatabaseID > 0 {
				images[img.DatabaseID] = append(images[img.DatabaseID], img)
			}
		}
	}

	for _, game := range games {
		game.Name = strings.TrimSpace(game.Name)
		game.AltNames = uniqueAltNames(game.Name, alternates[game.DatabaseID])
		game.Images = images[game.DatabaseID]
	}
	return games, nil
}

func releaseYear(date string) int {
	date = strings.TrimSpace(date)
	if len(date) < 4 {
		return 0
	}
	year, err := strconv.Atoi(date[:4])
	if err != nil || year < 1970 || year > 2100 {
		return 0
	}
	return year
}

// uniqueAltNames drops alternate titles that repeat the main title.
func uniqueAltNames(title string, names []string) []string {
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(title)): true}
	var out []string
	for _, name := range names {
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}
