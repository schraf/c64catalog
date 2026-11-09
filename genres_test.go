package main

import (
	"strings"
	"testing"
)

func TestFoldGenres(t *testing.T) {
	tests := []struct {
		raw  []string
		want string
	}{
		{[]string{"Action", "First Person Shooter"}, "Action,Shooter"},
		{[]string{"Shooter / Scrolling shoot'em up", "Shooter / Fixed-screen shoot'em up"}, "Shooter"},
		{[]string{"Vehicle Simulation", "Flight Simulator", "Racing & Driving"}, "Simulation,Racing"},
		{[]string{"Board Game / Party Game", "Casino"}, "Board, Casino & Quiz"},
		{[]string{"Platform"}, "Platformer"},
		{[]string{"Something New"}, "Other"},
		{nil, "Other"},
	}
	for _, tt := range tests {
		if got := strings.Join(foldGenres(tt.raw), ","); got != tt.want {
			t.Errorf("foldGenres(%v) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestSortedKeys(t *testing.T) {
	counts := map[string]int{"b": 2, "a": 2, "c": 9, "Other": 50}
	got := strings.Join(sortedKeys(counts, "Other"), ",")
	if got != "c,a,b,Other" {
		t.Fatalf("got %q", got)
	}
}
