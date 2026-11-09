package main

import (
	"sort"
	"strings"
)

// genreGroups folds the LaunchBox genre vocabulary (about 30 top-level genres,
// plus sub-genres such as "Shooter / Scrolling shoot'em up") into the handful
// of chips shown in the filter bar. The original strings are kept as tags.
var genreGroups = map[string]string{
	"Action":                                 "Action",
	"Adventure":                              "Adventure",
	"Interactive Movie":                      "Adventure",
	"Interactive Fiction":                    "Interactive Fiction",
	"Text-Based":                             "Interactive Fiction",
	"Strategy":                               "Strategy",
	"Puzzle":                                 "Puzzle",
	"Platform":                               "Platformer",
	"Shooter":                                "Shooter",
	"First Person Shooter":                   "Shooter",
	"Fighting":                               "Fighting",
	"Role-Playing":                           "RPG",
	"Simulation":                             "Simulation",
	"Vehicle Simulation":                     "Simulation",
	"Flight Simulator":                       "Simulation",
	"Construction and Management Simulation": "Simulation",
	"Racing & Driving":                       "Racing",
	"Sports":                                 "Sports",
	"Arcade":                                 "Arcade",
	"Paddle & Pong":                          "Arcade",
	"Pinball":                                "Arcade",
	"Board Game":                             "Board, Casino & Quiz",
	"Casino":                                 "Board, Casino & Quiz",
	"Quiz":                                   "Board, Casino & Quiz",
	"Game Show":                              "Board, Casino & Quiz",
	"Education":                              "Education",
	"Creativity":                             "Education",
	"Reference":                              "Education",
}

const otherGenre = "Other"

// splitList splits a LaunchBox semicolon-separated list, dropping blanks and
// repeats while keeping the original order.
func splitList(value string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(value, ";") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

// foldGenres maps raw LaunchBox genres to catalog genres. A game with no
// genre at all is filed under "Other" so every game appears under a chip.
func foldGenres(raw []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, genre := range raw {
		top, _, _ := strings.Cut(genre, " / ")
		group, ok := genreGroups[strings.TrimSpace(top)]
		if !ok {
			group = otherGenre
		}
		if !seen[group] {
			seen[group] = true
			out = append(out, group)
		}
	}
	if len(out) == 0 {
		out = []string{otherGenre}
	}
	return out
}

// sortedKeys returns the keys of a count map ordered by descending count and
// then by name, with the given name sorted last when present.
func sortedKeys(counts map[string]int, last string) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if last != "" && (a == last) != (b == last) {
			return b == last
		}
		if counts[a] != counts[b] {
			return counts[a] > counts[b]
		}
		return a < b
	})
	return keys
}
