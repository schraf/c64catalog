package main

import (
	"reflect"
	"testing"
)

func TestParseTosecName(t *testing.T) {
	tests := []struct {
		input string
		want  tosecName
	}{
		{
			"Library Adventure (1984)(Loadstar)",
			tosecName{Title: "Library Adventure", Year: 1984, Publisher: "Loadstar"},
		},
		{
			"10 Pin Bowling (1984)(CDS Microsystems)[h CHR] Released 1984 Published by CDS Microsystems",
			tosecName{Title: "10 Pin Bowling", Year: 1984, Publisher: "CDS Microsystems", Flags: []string{"h CHR"}},
		},
		{
			"Ultima III Exodus (1984)(Origin Systems)(Side B)(Scenario Disk) Developed by ORIGIN",
			tosecName{Title: "Ultima III Exodus", Year: 1984, Publisher: "Origin Systems", Disk: "Side B (Scenario Disk)"},
		},
		{
			"Ultima VI The False Prophet (1990)(Origin Systems)(Disk 1 of 3 Side A)(Game) NOTE: This emulation",
			tosecName{Title: "Ultima VI The False Prophet", Year: 1990, Publisher: "Origin Systems", Disk: "Disk 1 of 3 Side A (Game)"},
		},
		{
			"0 Grad Nord (1985)(Ariolasoft)(de)(Side A) NOTE: This emulation has multiple disk images.",
			tosecName{Title: "0 Grad Nord", Year: 1985, Publisher: "Ariolasoft", Language: "de", Disk: "Side A"},
		},
		{
			"1.000 Miles (19xx)( )",
			tosecName{Title: "1.000 Miles"},
		},
		{
			"12 O'Clock (1991)(CP Verlag)[f NTSC Empire][t +1 Arcade]",
			tosecName{Title: "12 O'Clock", Year: 1991, Publisher: "CP Verlag", Flags: []string{"f NTSC Empire", "t +1 Arcade"}},
		},
		{
			"126 P Part 2 (1992 09 28)(Tomasz Golczewski)",
			tosecName{Title: "126 P Part 2", Year: 1992, Publisher: "Tomasz Golczewski"},
		},
		{
			"Pool of Radiance v1.1 (1988)(SSI)(PAL)(Budget)",
			tosecName{Title: "Pool of Radiance v1.1", Year: 1988, Publisher: "SSI", Video: "PAL", Extras: []string{"Budget"}},
		},
		{
			"Great Giana Sisters, The (1987)(Rainbow Arts)[!]",
			tosecName{Title: "Great Giana Sisters, The", Year: 1987, Publisher: "Rainbow Arts", Flags: []string{"!"}},
		},
	}
	for _, tt := range tests {
		got, ok := parseTosecName(tt.input)
		if !ok {
			t.Errorf("parseTosecName(%q) failed", tt.input)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseTosecName(%q)\n got %+v\nwant %+v", tt.input, got, tt.want)
		}
	}

	for _, input := range []string{"", "Smasher", "Joe Gunn - Gold 27-08-2007 - English"} {
		if _, ok := parseTosecName(input); ok {
			t.Errorf("parseTosecName(%q) should fail", input)
		}
	}
}

func TestReleaseKeyIgnoresDisk(t *testing.T) {
	a, _ := parseTosecName("Ultima III Exodus (1984)(Origin Systems)(Side A)")
	b, _ := parseTosecName("Ultima III Exodus (1984)(Origin Systems)(Side B)(Scenario Disk)")
	c, _ := parseTosecName("Ultima III Exodus (1984)(Origin Systems)[cr XYZ]")
	if a.releaseKey() != b.releaseKey() {
		t.Error("sides of one release must share a key")
	}
	if a.releaseKey() == c.releaseKey() {
		t.Error("a crack is a different release")
	}
	if got := c.label(); got != "Ultima III Exodus (1984)(Origin Systems)[cr XYZ]" {
		t.Errorf("label = %q", got)
	}
}

func TestStripTrailingTags(t *testing.T) {
	tests := map[string]string{
		"10 Pin Bowling (1984)(CDS Microsystems)[h D+D]": "10 Pin Bowling",
		"Joe Gunn":               "Joe Gunn",
		"1-2-3... (Disk 1 of 2)": "1-2-3...",
	}
	for input, want := range tests {
		if got := stripTrailingTags(input); got != want {
			t.Errorf("stripTrailingTags(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFlagLabel(t *testing.T) {
	tests := map[string]string{
		"h CHR":         "Hacked by CHR",
		"cr Fairlight":  "Cracked by Fairlight",
		"t +1 Arcade":   "Trainer (+1) by Arcade",
		"t +4":          "Trainer (+4)",
		"f NTSC Empire": "Fixed: NTSC Empire",
		"a":             "Alternate",
		"Docs":          "Includes docs",
		"a2":            "Alternate",
		"b":             "Bad dump",
		"!":             "Verified dump",
		"Key Steppa":    "Key Steppa",
	}
	for input, want := range tests {
		if got := flagLabel(input); got != want {
			t.Errorf("flagLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestReleaseClass(t *testing.T) {
	tests := map[string]int{
		"A (1984)(B)":                             classDump,
		"A (1984)(B)[!]":                          classDump,
		"A (1984)(B)[cr Fairlight]":               classCrack,
		"A (1984)(B)[cr Fairlight][f NTSC][Docs]": classCrack,
		"A (1984)(B)[cr Fairlight][t +2 GOD]":     classTrainer,
		"A (1984)(B)[cr Fairlight][h Samar]":      classHack,
		"A (1984)(B)[h Samar][t +1]":              classTrainer,
		"A (1984)(B)[cr X][b2]":                   classBad,
		"A (1984)(B)[cr X][Created with SEUCK]":   classCrack,
		"A (1984)(B)[Created with SEUCK]":         classDump,
	}
	for input, want := range tests {
		name, ok := parseTosecName(input)
		if !ok {
			t.Fatalf("parse %q", input)
		}
		if got := name.releaseClass(); got != want {
			t.Errorf("releaseClass(%q) = %d, want %d", input, got, want)
		}
	}
	verified, _ := parseTosecName("A (1984)(B)[!]")
	plain, _ := parseTosecName("A (1984)(B)")
	if !verified.isVerified() || plain.isVerified() {
		t.Fatal("only [!] is verified")
	}
}

func TestLanguageLabel(t *testing.T) {
	if got := languageLabel("de-en"); got != "German, English" {
		t.Fatalf("got %q", got)
	}
	if got := languageLabel("xx"); got != "xx" {
		t.Fatalf("got %q", got)
	}
}
