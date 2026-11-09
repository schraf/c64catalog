package main

import (
	"regexp"
	"strconv"
	"strings"
)

// tosecName is a parsed TOSEC-style file name such as
// "Ultima III Exodus (1984)(Origin Systems)(Side B)(Scenario Disk)" or
// "10 Pin Bowling (1984)(CDS Microsystems)[h CHR]". The Internet Archive's
// C64 library uses these names as item descriptions.
type tosecName struct {
	Title     string
	Year      int
	Publisher string
	// Disk is the disk, side, or tape label, including any name that follows
	// it ("Side B (Scenario Disk)"). Items of one release differ only here.
	Disk     string
	Language string
	Video    string
	// Extras are other bracketed notes such as "v1.1" or "Budget".
	Extras []string
	// Flags are the square-bracketed dump flags: "h CHR", "t +1 Arcade", "a".
	Flags []string
}

var (
	tosecRE       = regexp.MustCompile(`^(.+?)\s+\(([0-9x?]{4}(?:[ -][0-9x?]{2}){0,2})\)((?:\s?(?:\([^()]*\)|\[[^\[\]]*\]))*)`)
	tosecGroupRE  = regexp.MustCompile(`\(([^()]*)\)|\[([^\[\]]*)\]`)
	tosecDiskRE   = regexp.MustCompile(`(?i)^(?:(?:disk|disc|tape|part|cart|cartridge|file) \d+(?: of \d+)?(?: side [a-z])?|side [a-z0-9])$`)
	tosecLangRE   = regexp.MustCompile(`^[a-z]{2}(?:-[a-z]{2})*$`)
	tosecVideoRE  = regexp.MustCompile(`^(?:PAL|NTSC|PAL-NTSC|NTSC-PAL)$`)
	trailingTagRE = regexp.MustCompile(`\s*(?:\([^()]*\)|\[[^\[\]]*\])\s*$`)
)

// parseTosecName parses a TOSEC name. It reports false when the text does not
// start with a title followed by a bracketed year.
func parseTosecName(text string) (tosecName, bool) {
	match := tosecRE.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return tosecName{}, false
	}
	name := tosecName{
		Title: strings.TrimSpace(match[1]),
		Year:  tosecYear(match[2]),
	}

	groups := tosecGroupRE.FindAllStringSubmatch(match[3], -1)
	sawPublisher := false
	afterDisk := false
	for _, group := range groups {
		if group[0][0] == '[' {
			if flag := strings.TrimSpace(group[2]); flag != "" {
				name.Flags = append(name.Flags, flag)
			}
			afterDisk = false
			continue
		}
		value := strings.TrimSpace(group[1])
		if !sawPublisher {
			sawPublisher = true
			if value != "-" {
				name.Publisher = value
			}
			continue
		}
		switch {
		case value == "":
		case tosecDiskRE.MatchString(value):
			name.Disk = joinNonEmpty(" ", name.Disk, value)
			afterDisk = true
			continue
		case tosecLangRE.MatchString(value):
			name.Language = value
		case tosecVideoRE.MatchString(value):
			name.Video = value
		case afterDisk:
			// A name right after the disk ("Side B)(Scenario Disk") labels it.
			name.Disk += " (" + value + ")"
		default:
			name.Extras = append(name.Extras, value)
		}
		afterDisk = false
	}
	return name, true
}

func tosecYear(value string) int {
	if len(value) < 4 {
		return 0
	}
	year, err := strconv.Atoi(value[:4])
	if err != nil || year < 1970 || year > 2100 {
		return 0
	}
	return year
}

// releaseKey identifies a release: items that share it are the disks or sides
// of one release and are grouped together.
func (n tosecName) releaseKey() string {
	parts := []string{
		normalizeTitle(n.Title),
		strconv.Itoa(n.Year),
		strings.ToLower(n.Publisher),
		n.Language,
		n.Video,
		strings.ToLower(strings.Join(n.Extras, "|")),
		strings.ToLower(strings.Join(n.Flags, "|")),
	}
	return strings.Join(parts, "\x00")
}

// label is the release's TOSEC name without the disk part, for display.
func (n tosecName) label() string {
	var b strings.Builder
	b.WriteString(n.Title)
	if n.Year > 0 {
		b.WriteString(" (" + strconv.Itoa(n.Year) + ")")
	}
	if n.Publisher != "" {
		b.WriteString("(" + n.Publisher + ")")
	}
	for _, value := range []string{n.Language, n.Video} {
		if value != "" {
			b.WriteString("(" + value + ")")
		}
	}
	for _, extra := range n.Extras {
		b.WriteString("(" + extra + ")")
	}
	for _, flag := range n.Flags {
		b.WriteString("[" + flag + "]")
	}
	return b.String()
}

// stripTrailingTags removes trailing "(...)" and "[...]" groups, which the
// archive's item titles sometimes carry.
func stripTrailingTags(title string) string {
	for {
		next := trailingTagRE.ReplaceAllString(title, "")
		if next == title {
			return strings.TrimSpace(title)
		}
		title = next
	}
}

// Release classes, best first. Most commercial games circulate as cracks, so
// a plain crack ranks just below an untouched dump.
const (
	classDump = iota
	classCrack
	classHack
	classTrainer
	classBad
)

// releaseClass sorts a release by its dump flags: "cr" is a crack, "h" a
// hack, "t" a trainer, and "b" a bad dump. The worst flag decides.
func (n tosecName) releaseClass() int {
	class := classDump
	for _, flag := range n.Flags {
		code, _, _ := strings.Cut(flag, " ")
		var flagClass int
		switch {
		case code == "!":
			continue
		case code == "cr" || code == "f" || code == "o" || code == "a" || code == "u" || strings.EqualFold(code, "docs"):
			flagClass = classCrack
		case code == "t":
			flagClass = classTrainer
		case code == "b" || strings.HasPrefix(code, "b") && isDigits(code[1:]):
			flagClass = classBad
		case code == "h" || code == "m" || code == "tr":
			flagClass = classHack
		default:
			// Notes such as "Created with SEUCK" or "One Side Version" say
			// nothing about how the dump was changed.
			continue
		}
		class = max(class, flagClass)
	}
	return class
}

// isVerified reports whether the release is a verified dump ("[!]").
func (n tosecName) isVerified() bool {
	for _, flag := range n.Flags {
		if flag == "!" {
			return true
		}
	}
	return false
}

// flagLabel turns a TOSEC dump flag into words.
func flagLabel(flag string) string {
	code, rest, _ := strings.Cut(flag, " ")
	rest = strings.TrimSpace(rest)
	by := func(text string) string {
		if rest == "" {
			return text
		}
		return text + " by " + rest
	}
	switch {
	case code == "!":
		return "Verified dump"
	case code == "cr":
		return by("Cracked")
	case code == "h":
		return by("Hacked")
	case code == "f":
		if rest == "" {
			return "Fixed"
		}
		return "Fixed: " + rest
	case code == "m":
		return by("Modified")
	case code == "tr":
		if rest == "" {
			return "Translated"
		}
		return "Translated: " + rest
	case code == "t":
		count, who, _ := strings.Cut(rest, " ")
		if strings.HasPrefix(count, "+") {
			label := "Trainer (" + count + ")"
			if who != "" {
				label += " by " + who
			}
			return label
		}
		return by("Trainer")
	case code == "o":
		return "Overdump"
	case code == "p":
		return by("Pirate")
	case strings.EqualFold(code, "docs"):
		return "Includes docs"
	case code == "b" || strings.HasPrefix(code, "b") && isDigits(code[1:]):
		return "Bad dump"
	case code == "a" || strings.HasPrefix(code, "a") && isDigits(code[1:]):
		return "Alternate"
	}
	return flag
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// languageNames covers the TOSEC codes that occur in the C64 library.
var languageNames = map[string]string{
	"en": "English", "de": "German", "fr": "French", "it": "Italian",
	"es": "Spanish", "nl": "Dutch", "sv": "Swedish", "da": "Danish",
	"no": "Norwegian", "fi": "Finnish", "pl": "Polish", "hu": "Hungarian",
	"cs": "Czech", "pt": "Portuguese", "tr": "Turkish", "el": "Greek",
	"is": "Icelandic", "sk": "Slovak", "sl": "Slovenian", "hr": "Croatian",
	"ru": "Russian", "ja": "Japanese",
}

// languageLabel names a TOSEC language code; "de-en" becomes "German, English".
func languageLabel(code string) string {
	if code == "" {
		return ""
	}
	var names []string
	for _, part := range strings.Split(code, "-") {
		if name, ok := languageNames[part]; ok {
			names = append(names, name)
		} else {
			names = append(names, part)
		}
	}
	return strings.Join(names, ", ")
}

func joinNonEmpty(sep string, values ...string) string {
	var out []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return strings.Join(out, sep)
}
