package main

import (
	"fmt"
	"sort"
	"strings"
)

// priorWeight is how many votes a game needs before its own score outweighs
// the catalog-wide average in the weighted rating. Most C64 games have few
// votes, so the prior is kept small.
const priorWeight = 10.0

// FacetCount is a filter chip: a value and how many games have it.
type FacetCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// IndexGame is the compact shape sent to the grid.
type IndexGame struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	AltNames       []string `json:"aka,omitempty"`
	Year           int      `json:"year"`
	Genres         []string `json:"genres"`
	Source         string   `json:"source"`
	Media          []string `json:"media"`
	Versions       int      `json:"versions"`
	Rating         float64  `json:"rating"`
	RatingCount    int      `json:"rating_count"`
	WeightedRating float64  `json:"weighted_rating"`
	Publisher      string   `json:"publisher"`
	Developer      string   `json:"developer"`
	MaxPlayers     int      `json:"max_players"`
	BoxArt         string   `json:"box_art,omitempty"`
}

// Catalog is the validated, immutable view of the snapshot.
type Catalog struct {
	games    []*Game
	byID     map[string]*Game
	weighted map[string]float64
	yearMin  int
	yearMax  int
	genres   []FacetCount
	media    []FacetCount
}

func buildCatalog(games []*Game) (*Catalog, error) {
	if len(games) == 0 {
		return nil, fmt.Errorf("catalog has no games")
	}

	c := &Catalog{
		games:    append([]*Game(nil), games...),
		byID:     make(map[string]*Game, len(games)),
		weighted: make(map[string]float64, len(games)),
	}
	for _, game := range c.games {
		if game.ID == "" || game.Name == "" {
			return nil, fmt.Errorf("game %q has an empty ID or name", game.ID)
		}
		if _, dup := c.byID[game.ID]; dup {
			return nil, fmt.Errorf("duplicate game ID %q", game.ID)
		}
		if len(game.Versions) == 0 {
			return nil, fmt.Errorf("game %q has no versions", game.ID)
		}
		c.byID[game.ID] = game
	}

	sort.SliceStable(c.games, func(i, j int) bool {
		a, b := strings.ToLower(c.games[i].Name), strings.ToLower(c.games[j].Name)
		if a != b {
			return a < b
		}
		return c.games[i].ID < c.games[j].ID
	})

	c.computeWeightedRatings()
	c.computeFacets()
	return c, nil
}

func (c *Catalog) computeWeightedRatings() {
	var sum float64
	var rated int
	for _, game := range c.games {
		if game.RatingCount > 0 && game.Rating > 0 {
			sum += game.Rating
			rated++
		}
	}
	if rated == 0 {
		return
	}
	mean := sum / float64(rated)
	for _, game := range c.games {
		if game.RatingCount > 0 && game.Rating > 0 {
			votes := float64(game.RatingCount)
			c.weighted[game.ID] = (votes*game.Rating + priorWeight*mean) / (votes + priorWeight)
		}
	}
}

func (c *Catalog) computeFacets() {
	genres := map[string]int{}
	media := map[string]int{}
	for _, game := range c.games {
		for _, genre := range game.Genres {
			genres[genre]++
		}
		for _, m := range game.Media {
			media[m]++
		}
		if game.ReleaseYear > 0 {
			if c.yearMin == 0 || game.ReleaseYear < c.yearMin {
				c.yearMin = game.ReleaseYear
			}
			if game.ReleaseYear > c.yearMax {
				c.yearMax = game.ReleaseYear
			}
		}
	}
	c.genres = facetList(genres, otherGenre)

	// Media types keep their natural order, not their popularity.
	for _, m := range mediaOrder {
		if media[m] > 0 {
			c.media = append(c.media, FacetCount{Name: m, Count: media[m]})
		}
	}
}

func facetList(counts map[string]int, last string) []FacetCount {
	keys := sortedKeys(counts, last)
	out := make([]FacetCount, 0, len(keys))
	for _, key := range keys {
		out = append(out, FacetCount{Name: key, Count: counts[key]})
	}
	return out
}

func (c *Catalog) Total() int { return len(c.games) }

// Index returns the compact grid records in display order.
func (c *Catalog) Index() []IndexGame {
	out := make([]IndexGame, 0, len(c.games))
	for _, g := range c.games {
		out = append(out, IndexGame{
			ID:             g.ID,
			Name:           g.Name,
			AltNames:       g.AltNames,
			Year:           g.ReleaseYear,
			Genres:         g.Genres,
			Source:         g.Source,
			Media:          g.Media,
			Versions:       len(g.Versions) + g.ExtraVersions,
			Rating:         g.Rating,
			RatingCount:    g.RatingCount,
			WeightedRating: c.weighted[g.ID],
			Publisher:      g.Publisher,
			Developer:      g.Developer,
			MaxPlayers:     g.MaxPlayers,
			BoxArt:         g.BoxArt,
		})
	}
	return out
}
