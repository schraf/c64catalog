package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// FileRef is a downloadable disk, tape, cartridge, or program image.
type FileRef struct {
	Name string `json:"name"`
	Size int64  `json:"size,omitempty"`
	URL  string `json:"url"`
}

// Version is one release of a game in the archive: an original, a crack, a
// hack, or a re-release. Its files are the release's disks or sides.
type Version struct {
	Label     string    `json:"label"`
	Publisher string    `json:"publisher,omitempty"`
	Year      int       `json:"year,omitempty"`
	Language  string    `json:"language,omitempty"`
	Video     string    `json:"video,omitempty"`
	Notes     []string  `json:"notes,omitempty"`
	Media     string    `json:"media"`
	Play      string    `json:"play"`
	Downloads int       `json:"downloads"`
	Items     []string  `json:"items"`
	Files     []FileRef `json:"files"`

	// class and verified rank the release; they are only used while building
	// the catalog.
	class    int
	verified bool
}

// Game is one record in the catalog snapshot and in its detail file.
type Game struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	AltNames    []string `json:"alt_names,omitempty"`
	ReleaseYear int      `json:"release_year"`
	Developer   string   `json:"developer"`
	Publisher   string   `json:"publisher"`
	Genres      []string `json:"genres"`
	Tags        []string `json:"tags,omitempty"`
	Source      string   `json:"source"`
	Media       []string `json:"media"`
	MaxPlayers  int      `json:"max_players"`
	Cooperative bool     `json:"cooperative,omitempty"`
	AgeRating   string   `json:"age_rating,omitempty"`
	Overview    string   `json:"overview"`
	Wikipedia   string   `json:"wikipedia,omitempty"`
	Video       string   `json:"video,omitempty"`

	LaunchBoxID int     `json:"launchbox_id"`
	Rating      float64 `json:"rating"`
	RatingCount int     `json:"rating_count"`

	BoxArt      string   `json:"box_art,omitempty"`
	Screenshots []string `json:"screenshots,omitempty"`

	// Versions are ordered best first; the first is the default download.
	// Popular games have more releases than are worth listing, so only the
	// best maxVersions are kept; ExtraVersions counts the rest.
	Versions      []Version `json:"versions"`
	ExtraVersions int       `json:"extra_versions,omitempty"`
}

// maxVersions is the most versions kept for one game.
const maxVersions = 12

const unknownSource = "Unknown"

var (
	keyParensRE  = regexp.MustCompile(`\([^)]*\)|\[[^\]]*\]`)
	keyTrailArt  = regexp.MustCompile(`,\s*(the|a|an|la|le|les|el|los|las|der|die|das|il)$`)
	keyLeadArt   = regexp.MustCompile(`^(the|a|an|la|le|les|el|los|las|der|die|das|il)\s+`)
	keyVersionRE = regexp.MustCompile(`\s+v\.?\d+(?:\.\d+)*[a-z]?$`)
	keyEditionRE = regexp.MustCompile(`\s+(?:us|uk|usa|eu|european|american|disk|disc|tape|cassette|cartridge|pal|ntsc|c64)\s+version$`)
	keyPrefixRE  = regexp.MustCompile(`^(?:commodore 64|c64)\s*:\s*`)
	keySpaceRE   = regexp.MustCompile(`\s+`)
	keyRomanRE   = regexp.MustCompile(`\b(ii|iii|iv|vi|vii|viii)\b`)
	romanDigits  = map[string]string{"ii": "2", "iii": "3", "iv": "4", "vi": "6", "vii": "7", "viii": "8"}
	keyOneRE     = regexp.MustCompile(`\s(?:part\s)?(?:1|i)\b`)
	keyFoldTable = strings.NewReplacer(
		"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a",
		"ç", "c",
		"è", "e", "é", "e", "ê", "e", "ë", "e",
		"ì", "i", "í", "i", "î", "i", "ï", "i",
		"ñ", "n",
		"ò", "o", "ó", "o", "ô", "o", "õ", "o", "ö", "o", "ø", "o",
		"ù", "u", "ú", "u", "û", "u", "ü", "u",
		"ý", "y", "ÿ", "y",
		"ß", "ss", "æ", "ae", "œ", "oe",
		"&", " and ", "+", " plus ",
	)
)

// normalizeTitle reduces a title to lowercase letters and digits.
func normalizeTitle(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// titleKey reduces a title so that LaunchBox names and TOSEC names compare
// equal. It ignores case, punctuation, accents, bracketed text, version
// numbers ("v2.0"), edition notes ("Disk Version"), roman numerals ("II"
// matches "2"), and a leading or trailing article ("Great Giana Sisters,
// The" matches "The Great Giana Sisters").
func titleKey(title string) string {
	return normalizeTitle(cleanTitle(title))
}

func cleanTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	title = keyPrefixRE.ReplaceAllString(title, "")
	title = keyFoldTable.Replace(title)
	title = keyParensRE.ReplaceAllString(title, " ")
	title = strings.NewReplacer("-", " ", ":", " ", "'", "", "’", "").Replace(title)
	title = keySpaceRE.ReplaceAllString(strings.TrimSpace(title), " ")
	title = keyVersionRE.ReplaceAllString(title, "")
	title = keyEditionRE.ReplaceAllString(title, "")
	// Only one article is dropped, so "A Team, The" and "The A-Team" agree.
	if trimmed := keyTrailArt.ReplaceAllString(title, ""); trimmed != title {
		title = trimmed
	} else {
		title = keyLeadArt.ReplaceAllString(title, "")
	}
	title = keyRomanRE.ReplaceAllStringFunc(title, func(numeral string) string { return romanDigits[numeral] })
	return title
}

// withoutPartOne drops a "1" or "Part 1" from a title's key: the archive
// numbers first games in a series ("Bards Tale 1", "Double Dragon 1") that
// LaunchBox lists without a number.
func withoutPartOne(title string) string {
	cleaned := cleanTitle(title)
	if stripped := keyOneRE.ReplaceAllString(cleaned, ""); stripped != cleaned {
		return normalizeTitle(stripped)
	}
	return ""
}

// baseTitle is the part of a LaunchBox name before a subtitle:
// "Indiana Jones and the Last Crusade: The Action Game" becomes
// "Indiana Jones and the Last Crusade".
func baseTitle(name string) string {
	for _, sep := range []string{": ", " - "} {
		if i := strings.Index(name, sep); i > 0 {
			return name[:i]
		}
	}
	return ""
}

// matcher finds the LaunchBox game for a TOSEC title and year.
type matcher struct {
	index map[string][]*lbGame
	// base indexes LaunchBox names without their subtitles. It is only used
	// when it identifies a single game.
	base map[string][]*lbGame
}

func newMatcher(games []*lbGame) *matcher {
	m := &matcher{index: map[string][]*lbGame{}, base: map[string][]*lbGame{}}
	for _, game := range games {
		seen := map[string]bool{}
		names := append([]string{game.Name}, game.AltNames...)
		for _, name := range names {
			key := titleKey(name)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			m.index[key] = append(m.index[key], game)
		}
		seenBase := map[string]bool{}
		for _, name := range names {
			key := titleKey(baseTitle(name))
			if key == "" || seen[key] || seenBase[key] {
				continue
			}
			seenBase[key] = true
			m.base[key] = append(m.base[key], game)
		}
	}
	return m
}

// maxUniqueYearGap is how far apart the years may be when only one LaunchBox
// game has the title, or when the publisher agrees. Release years in the two
// sources often differ.
const maxUniqueYearGap = 5

// match returns the LaunchBox game for a release. A same-year game wins, then
// one a year off, then one where either year is unknown. Games further apart
// are rejected so that sequels and remakes with the same title stay apart,
// unless the publisher agrees or the title is unique, and the gap is small.
// Ties go to the game with the most ratings. When the title has no match, it
// is retried without a "1" and then against names without subtitles.
func (m *matcher) match(title string, year int, publisher string) *lbGame {
	if game := chooseGame(m.index[titleKey(title)], year, publisher, true); game != nil {
		return game
	}
	if key := withoutPartOne(title); key != "" {
		if game := chooseGame(m.index[key], year, publisher, true); game != nil {
			return game
		}
	}
	candidates := m.base[titleKey(title)]
	if len(candidates) > 1 {
		// "Indiana Jones and the Last Crusade" is both an action game and an
		// adventure; the publisher tells them apart.
		var same []*lbGame
		for _, candidate := range candidates {
			if samePublisher(candidate, publisher) {
				same = append(same, candidate)
			}
		}
		candidates = same
	}
	if len(candidates) == 1 {
		return chooseGame(candidates, year, publisher, false)
	}
	return nil
}

func chooseGame(candidates []*lbGame, year int, publisher string, allowGap bool) *lbGame {
	var best *lbGame
	bestRank := -1
	for _, candidate := range candidates {
		rank := 2
		if gameYear := candidate.year(); gameYear != 0 && year != 0 {
			rank = absInt(gameYear - year)
			if rank > 1 {
				continue
			}
		}
		if best == nil || rank < bestRank || rank == bestRank && betterKnown(candidate, best) {
			best, bestRank = candidate, rank
		}
	}
	if best != nil || !allowGap {
		return best
	}
	for _, candidate := range candidates {
		if absInt(candidate.year()-year) > maxUniqueYearGap {
			continue
		}
		if len(candidates) == 1 || samePublisher(candidate, publisher) {
			if best == nil || betterKnown(candidate, best) {
				best = candidate
			}
		}
	}
	return best
}

// samePublisher reports whether a TOSEC publisher names the LaunchBox game's
// publisher or developer ("U.S. Gold" and "U.S. Gold Ltd.").
func samePublisher(game *lbGame, publisher string) bool {
	want := normalizeTitle(publisher)
	if len(want) < 3 {
		return false
	}
	for _, field := range []string{game.Publisher, game.Developer, game.Name} {
		for _, name := range splitList(field) {
			if have := normalizeTitle(name); len(have) >= 3 && (strings.Contains(have, want) || strings.Contains(want, have)) {
				return true
			}
		}
	}
	return false
}

func betterKnown(a, b *lbGame) bool {
	if a.RatingCount != b.RatingCount {
		return a.RatingCount > b.RatingCount
	}
	return a.DatabaseID < b.DatabaseID
}

// release is a group of archive items holding the disks or sides of one
// release.
type release struct {
	name  tosecName
	items []archiveItem
}

// groupReleases groups items whose TOSEC names differ only in the disk part.
func groupReleases(items []archiveItem) []*release {
	byKey := map[string]*release{}
	var out []*release
	for _, item := range items {
		name := item.name()
		if name.Title == "" {
			continue
		}
		key := name.releaseKey()
		rel, ok := byKey[key]
		if !ok {
			rel = &release{name: name}
			byKey[key] = rel
			out = append(out, rel)
		}
		rel.items = append(rel.items, item)
	}
	for _, rel := range out {
		sort.SliceStable(rel.items, func(i, j int) bool {
			a, b := rel.items[i].name().Disk, rel.items[j].name().Disk
			if a != b {
				return naturalLess(a, b)
			}
			return rel.items[i].Identifier < rel.items[j].Identifier
		})
	}
	return out
}

type matchStats struct {
	Releases int
	Matched  int
	Games    int
}

// matchReleases assigns releases to LaunchBox games. Releases that match no
// game are dropped.
func matchReleases(releases []*release, games []*lbGame) (map[int][]*release, matchStats) {
	m := newMatcher(games)
	out := map[int][]*release{}
	stats := matchStats{Releases: len(releases)}
	for _, rel := range releases {
		game := m.match(rel.name.Title, rel.name.Year, rel.name.Publisher)
		if game == nil {
			continue
		}
		out[game.DatabaseID] = append(out[game.DatabaseID], rel)
		stats.Matched++
	}
	stats.Games = len(out)
	return out, stats
}

// buildGames turns matched LaunchBox games and their releases into catalog
// games. Files come from the item cache; items whose file list could not be
// read fall back to the file the archive's emulator starts.
func buildGames(lb []*lbGame, matched map[int][]*release, cache itemCache) []*Game {
	sorted := make([]*lbGame, 0, len(matched))
	for _, game := range lb {
		if len(matched[game.DatabaseID]) > 0 {
			sorted = append(sorted, game)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].DatabaseID < sorted[j].DatabaseID })

	ids := map[string]bool{}
	var games []*Game
	for _, entry := range sorted {
		var versions []Version
		for _, rel := range matched[entry.DatabaseID] {
			if version, ok := buildVersion(rel, cache); ok {
				versions = append(versions, version)
			}
		}
		if len(versions) == 0 {
			continue
		}
		rankVersions(versions)
		extra := 0
		if len(versions) > maxVersions {
			extra = len(versions) - maxVersions
			versions = versions[:maxVersions]
		}

		rawGenres := splitList(entry.Genres)
		game := &Game{
			ID:          uniqueID(ids, entry),
			Name:        entry.Name,
			AltNames:    entry.AltNames,
			ReleaseYear: entry.year(),
			Developer:   strings.TrimSpace(entry.Developer),
			Publisher:   strings.TrimSpace(entry.Publisher),
			Genres:      foldGenres(rawGenres),
			Tags:        rawGenres,
			Source:      sourceLabel(entry.ReleaseType),
			MaxPlayers:  atoi(entry.MaxPlayers),
			Cooperative: strings.EqualFold(strings.TrimSpace(entry.Cooperative), "true"),
			AgeRating:   ageRating(entry.ESRB),
			Overview:    strings.TrimSpace(entry.Overview),
			Wikipedia:   httpURL(entry.Wikipedia),
			Video:       httpURL(entry.VideoURL),
			LaunchBoxID: entry.DatabaseID,
			Versions:    versions,

			ExtraVersions: extra,
		}
		if entry.RatingCount > 0 && entry.Rating > 0 {
			game.Rating = entry.Rating
			game.RatingCount = entry.RatingCount
		}
		if game.ReleaseYear == 0 {
			game.ReleaseYear = versions[0].Year
		}
		game.Media = gameMedia(versions)
		games = append(games, game)
	}
	sort.Slice(games, func(i, j int) bool { return games[i].ID < games[j].ID })
	return games
}

func buildVersion(rel *release, cache itemCache) (Version, bool) {
	name := rel.name
	version := Version{
		Label:     name.label(),
		Publisher: name.Publisher,
		Year:      name.Year,
		Language:  languageLabel(name.Language),
		Video:     name.Video,
		Notes:     append([]string(nil), name.Extras...),
	}
	for _, flag := range name.Flags {
		version.Notes = append(version.Notes, flagLabel(flag))
	}
	version.class = name.releaseClass()
	version.verified = name.isVerified()

	seen := map[string]bool{}
	for _, item := range rel.items {
		var files []itemFile
		if cached, ok := cache[item.Identifier]; ok {
			files = cached.Disks
		} else if start := string(item.EmulatorStart); start != "" && mediaType(start) != "" {
			files = []itemFile{{Name: start}}
		}
		if len(files) == 0 {
			continue
		}
		version.Items = append(version.Items, item.Identifier)
		version.Downloads += item.Downloads
		for _, file := range files {
			if seen[strings.ToLower(file.Name)] {
				continue
			}
			seen[strings.ToLower(file.Name)] = true
			version.Files = append(version.Files, FileRef{
				Name: file.Name,
				Size: file.Size,
				URL:  downloadURL(item.Identifier, file.Name),
			})
		}
	}
	if len(version.Files) == 0 {
		return Version{}, false
	}
	version.Play = detailsURL(version.Items[0])
	version.Media = mediaType(version.Files[0].Name)
	return version, true
}

// versionScore ranks releases for the default download: English releases
// first, then plain dumps, plain cracks, hacks, and trainers, in that order,
// with verified dumps ahead of other dumps.
func versionScore(v Version) int {
	score := (classBad - v.class) * 10
	if v.Language == "" || strings.Contains(v.Language, "English") {
		score += 100
	}
	if v.verified {
		score++
	}
	return score
}

func rankVersions(versions []Version) {
	sort.SliceStable(versions, func(i, j int) bool {
		a, b := versions[i], versions[j]
		if sa, sb := versionScore(a), versionScore(b); sa != sb {
			return sa > sb
		}
		if a.Downloads != b.Downloads {
			return a.Downloads > b.Downloads
		}
		return a.Label < b.Label
	})
}

func gameMedia(versions []Version) []string {
	has := map[string]bool{}
	for _, version := range versions {
		for _, file := range version.Files {
			has[mediaType(file.Name)] = true
		}
	}
	var out []string
	for _, media := range mediaOrder {
		if has[media] {
			out = append(out, media)
		}
	}
	return out
}

// uniqueID derives a URL-safe ID from the game's name ("Impossible Mission"
// becomes "impossible-mission"). Games sharing a name get the year, then the
// LaunchBox ID, appended.
func uniqueID(used map[string]bool, game *lbGame) string {
	base := slugify(game.Name)
	if base == "" {
		base = "game"
	}
	candidates := []string{base}
	if year := game.year(); year > 0 {
		candidates = append(candidates, base+"-"+strconv.Itoa(year))
	}
	candidates = append(candidates, base+"-"+strconv.Itoa(game.DatabaseID))
	for _, id := range candidates {
		if !used[id] {
			used[id] = true
			return id
		}
	}
	// The LaunchBox ID is unique, so this is not reached in practice.
	id := base + "-lb" + strconv.Itoa(game.DatabaseID)
	used[id] = true
	return id
}

func slugify(name string) string {
	name = keyFoldTable.Replace(strings.ToLower(name))
	var b strings.Builder
	dash := false
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		if r == '\'' || r == '’' {
			continue
		}
		dash = true
	}
	return b.String()
}

func atoi(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func sourceLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return unknownSource
	}
	return value
}

func ageRating(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "Not Rated") {
		return ""
	}
	return value
}

func httpURL(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") {
		return value
	}
	return ""
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
