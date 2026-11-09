package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// collectionIDs are the Internet Archive collections the catalog draws on:
// the games, and the cracked releases of commercial games.
var collectionIDs = []string{"softwarelibrary_c64_games", "softwarelibrary_c64_cracks"}

const (
	scrapeURL   = "https://archive.org/services/search/v1/scrape"
	metadataURL = "https://archive.org/metadata/"

	scrapeFields = "identifier,title,description,creator,year,date,language,downloads,emulator_start,emulator_ext"
	scrapePage   = 10000

	// minArchiveItems guards against an empty or truncated listing. The
	// collections hold about 44,000 items.
	minArchiveItems = 10000

	itemsName    = "items.json"
	itemsVersion = 1
)

// flexString decodes a metadata field that the search API returns as a
// string, a number, or a list of strings. Lists keep their first value.
type flexString string

func (s *flexString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	switch {
	case len(data) == 0 || string(data) == "null":
		*s = ""
	case data[0] == '"':
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*s = flexString(strings.TrimSpace(value))
	case data[0] == '[':
		var values []flexString
		if err := json.Unmarshal(data, &values); err != nil {
			return err
		}
		*s = ""
		if len(values) > 0 {
			*s = values[0]
		}
	default:
		*s = flexString(strings.TrimSpace(string(data)))
	}
	return nil
}

// archiveItem is one item of the collection as listed by the search API.
type archiveItem struct {
	Identifier    string     `json:"identifier"`
	Title         flexString `json:"title"`
	Description   flexString `json:"description"`
	Creator       flexString `json:"creator"`
	Year          flexString `json:"year"`
	Date          flexString `json:"date"`
	Language      flexString `json:"language"`
	Downloads     int        `json:"downloads"`
	EmulatorStart flexString `json:"emulator_start"`
	EmulatorExt   flexString `json:"emulator_ext"`
}

// name returns the item's parsed TOSEC name. The description starts with the
// full name; when it does not, the title, creator, and year fields are used.
func (item archiveItem) name() tosecName {
	if name, ok := parseTosecName(string(item.Description)); ok {
		return name
	}
	if name, ok := parseTosecName(string(item.Title)); ok {
		return name
	}
	name := tosecName{
		Title:     stripTrailingTags(string(item.Title)),
		Publisher: string(item.Creator),
		Year:      tosecYear(string(item.Year)),
	}
	if name.Year == 0 {
		name.Year = tosecYear(string(item.Date))
	}
	return name
}

type scrapeResponse struct {
	Items  []archiveItem `json:"items"`
	Cursor string        `json:"cursor"`
	Total  int           `json:"total"`
}

// collectionQuery matches an item in any of the collections.
func collectionQuery() string {
	return "collection:(" + strings.Join(collectionIDs, " OR ") + ")"
}

// fetchArchiveItems lists every item in the collections, following the
// scrape API's cursor. An item in several collections is listed once.
func fetchArchiveItems(ctx context.Context, f *fetcher) ([]archiveItem, error) {
	var items []archiveItem
	seen := map[string]bool{}
	cursor := ""
	for {
		query := url.Values{}
		query.Set("q", collectionQuery())
		query.Set("fields", scrapeFields)
		query.Set("count", strconv.Itoa(scrapePage))
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		data, err := f.getBytes(ctx, scrapeURL+"?"+query.Encode(), 256<<20)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", strings.Join(collectionIDs, ", "), err)
		}
		var page scrapeResponse
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, fmt.Errorf("parse listing: %w", err)
		}
		for _, item := range page.Items {
			if item.Identifier != "" && !seen[item.Identifier] {
				seen[item.Identifier] = true
				items = append(items, item)
			}
		}
		if page.Cursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.Cursor
	}
	if len(items) < minArchiveItems {
		return nil, fmt.Errorf("the collections list only %d items; has the search API changed?", len(items))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Identifier < items[j].Identifier })
	return items, nil
}

// diskExtensions are the file types VICE loads, by media type.
var diskExtensions = map[string]string{
	"d64": mediaDisk, "g64": mediaDisk, "d71": mediaDisk, "d81": mediaDisk,
	"d80": mediaDisk, "d82": mediaDisk, "x64": mediaDisk, "nib": mediaDisk,
	"t64": mediaTape, "tap": mediaTape,
	"crt": mediaCartridge,
	"prg": mediaProgram, "p00": mediaProgram,
}

const (
	mediaDisk      = "Disk"
	mediaTape      = "Tape"
	mediaCartridge = "Cartridge"
	mediaProgram   = "Program"
)

// mediaOrder is the display order of media types.
var mediaOrder = []string{mediaDisk, mediaTape, mediaCartridge, mediaProgram}

func mediaType(fileName string) string {
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(fileName)), ".")
	return diskExtensions[ext]
}

// itemFile is a file in an item, with its size.
type itemFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// itemFiles lists the parts of an item the catalog uses.
type itemFiles struct {
	Disks []itemFile `json:"disks"`
	// Shots are screenshot file names, best first: the cover screenshot the
	// archive picked, then the emulator captures in order.
	Shots []string `json:"shots,omitempty"`
}

type metadataFile struct {
	Name   string     `json:"name"`
	Source string     `json:"source"`
	Size   flexString `json:"size"`
}

// parseItemFiles picks the disk images and screenshots out of an item's file
// list. Old copies kept under history/ and derived thumbnails are skipped.
func parseItemFiles(files []metadataFile) itemFiles {
	var out itemFiles
	var shots []string
	cover := ""
	for _, file := range files {
		if file.Source != "" && file.Source != "original" {
			continue
		}
		if strings.Contains(file.Name, "/") {
			continue
		}
		lower := strings.ToLower(file.Name)
		switch {
		case mediaType(lower) != "":
			size, _ := strconv.ParseInt(string(file.Size), 10, 64)
			out.Disks = append(out.Disks, itemFile{Name: file.Name, Size: size})
		case lower == "00_coverscreenshot.jpg":
			cover = file.Name
		case strings.HasPrefix(lower, "screenshot_") && strings.HasSuffix(lower, ".jpg") && !strings.HasSuffix(lower, "_thumb.jpg"):
			shots = append(shots, file.Name)
		}
	}
	sort.Slice(out.Disks, func(i, j int) bool { return naturalLess(out.Disks[i].Name, out.Disks[j].Name) })
	sort.Slice(shots, func(i, j int) bool { return naturalLess(shots[i], shots[j]) })
	if cover != "" {
		out.Shots = append(out.Shots, cover)
	}
	out.Shots = append(out.Shots, shots...)
	if len(out.Shots) > maxStoredShots {
		out.Shots = out.Shots[:maxStoredShots]
	}
	return out
}

// maxStoredShots bounds the screenshot names kept per item in items.json.
const maxStoredShots = 12

// fetchItemFiles reads one item's file list.
func fetchItemFiles(ctx context.Context, f *fetcher, identifier string) (itemFiles, error) {
	data, err := f.getBytes(ctx, metadataURL+escapeSegment(identifier)+"/files", 32<<20)
	if err != nil {
		return itemFiles{}, err
	}
	var doc struct {
		Result []metadataFile `json:"result"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return itemFiles{}, fmt.Errorf("parse %s files: %w", identifier, err)
	}
	if len(doc.Result) == 0 {
		return itemFiles{}, permanentError{fmt.Errorf("%s: %w", identifier, errNotFound)}
	}
	return parseItemFiles(doc.Result), nil
}

// itemCache stores item file lists in data/items.json so later refreshes only
// fetch new items.
type itemCache map[string]itemFiles

func loadItemCache(dataDir string) (itemCache, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, itemsName))
	if errors.Is(err, os.ErrNotExist) {
		return itemCache{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Version int       `json:"version"`
		Items   itemCache `json:"items"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", itemsName, err)
	}
	if doc.Version != itemsVersion || doc.Items == nil {
		return itemCache{}, nil
	}
	return doc.Items, nil
}

// write stores the cache with one item per line, sorted by identifier.
func (c itemCache) write(dataDir string) error {
	ids := make([]string, 0, len(c))
	for id := range c {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "{\"version\":%d,\"items\":{\n", itemsVersion)
	for i, id := range ids {
		key, _ := json.Marshal(id)
		value, err := json.Marshal(c[id])
		if err != nil {
			return err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(value)
		if i < len(ids)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("}}\n")
	return writeFileAtomic(filepath.Join(dataDir, itemsName), buf.Bytes(), 0o644)
}

// fill fetches the file lists of the wanted items that are not cached yet.
// Items that fail are logged and left out; they are retried next time.
func (c itemCache) fill(ctx context.Context, f *fetcher, ids []string, workers int, logf func(string, ...any)) (fetched, failed int, err error) {
	var missing []string
	for _, id := range ids {
		if _, ok := c[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return 0, 0, nil
	}
	logf("items: fetching file lists for %d items (%d cached)", len(missing), len(ids)-len(missing))

	jobs := make(chan string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var done atomic.Int64
	var failures atomic.Int64
	start := time.Now()

	workers = max(1, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for id := range jobs {
				files, err := fetchItemFiles(ctx, f, id)
				if err != nil {
					if ctx.Err() == nil {
						logf("items: %s: %v", id, err)
						failures.Add(1)
					}
				} else {
					mu.Lock()
					c[id] = files
					mu.Unlock()
				}
				if n := done.Add(1); n%500 == 0 || int(n) == len(missing) {
					logf("items: %d/%d (%s)", n, len(missing), time.Since(start).Round(time.Second))
				}
			}
		}()
	}
feed:
	for _, id := range missing {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- id:
		}
	}
	close(jobs)
	wg.Wait()
	failed = int(failures.Load())
	return len(missing) - failed, failed, ctx.Err()
}

// naturalLess compares strings with embedded numbers by value, so
// "Disk_2" sorts before "Disk_10".
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		ca, cb := a[0], b[0]
		if isDigit(ca) && isDigit(cb) {
			na, ra := leadingNumber(a)
			nb, rb := leadingNumber(b)
			if na != nb {
				return na < nb
			}
			a, b = ra, rb
			continue
		}
		la, lb := toLowerByte(ca), toLowerByte(cb)
		if la != lb {
			return la < lb
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func leadingNumber(s string) (int, string) {
	i := 0
	n := 0
	for i < len(s) && isDigit(s[i]) {
		if n < 1e8 {
			n = n*10 + int(s[i]-'0')
		}
		i++
	}
	return n, s[i:]
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func toLowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}
