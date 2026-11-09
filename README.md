# C64 Game Catalog

A static, searchable catalog of Commodore 64 games. The game files come from two collections in the Internet Archive's [Commodore 64 software library](https://archive.org/details/softwarelibrary_c64): [games](https://archive.org/details/softwarelibrary_c64_games) and [cracks](https://archive.org/details/softwarelibrary_c64_cracks), and the descriptions, box art and ratings come from the [LaunchBox Games Database](https://gamesdb.launchbox-app.com/). Every game can be downloaded from the Internet Archive or played there in the browser.

The site is plain HTML, JavaScript and JSON. A Go program (standard library plus `golang.org/x/image` for resizing) builds it from a committed snapshot, and GitHub Pages hosts it. There is no server and no database. It follows the same design as [`doscatalog`](https://github.com/schraf/doscatalog).

## What it shows

- Box art (with a screenshot as the fallback) and up to two screenshots per game
- Search over titles, alternate titles, developers, publishers, genres and media types
- Filters for genre, media (disk, tape, cartridge, program), release year range, and minimum rating
- Sorting by name, year, rating, popularity, or genre
- A detail drawer with the overview, facts, rating, a "Play on archive.org" link, downloads, and the game's other versions
- Favorites stored in the browser, with export and import
- Deep links such as `#game=impossible-mission`

### Versions and downloads

The two collections hold about 44,000 items, often several per game. The `cracks` collection holds the cracked releases of commercial games, which is how most 80s and 90s commercial titles circulate. The `games` collection holds mostly originals, public-domain and homebrew titles. Other collections in the library (demos, disk magazines, music disks, cracktros, applications, compilation and public-domain disks, educational software, and the Boulder Dash level packs) are not used, so Boulder Dash itself is missing. Items whose TOSEC names differ only in the disk or side (`(Side A)`, `(Disk 1 of 3 Side B)(Game)`) are grouped into one **version**, and each version lists every disk image it has. Many archive items already bundle all of a release's disks.

One version is the default, picked in this order:

1. English, or no language given
2. The release type: an untouched dump, then a plain crack (`[cr]`, with fixes `[f]` or docs), then a hack (`[h]`, `[m]`, or a translation `[tr]`), then a trainer (`[t]`, the "+3" and "+5" cheat versions), then a bad dump (`[b]`). `[!]` verified dumps rank first within the dumps.
3. The most downloads on archive.org

Popular games can have dozens of cracks, so only the best 12 versions are kept for each game. The drawer says how many more there are, and the tile badge shows the full count.

The drawer shows the default version's "Play on archive.org" link (archive.org runs the game in VICE in the browser) and its disk, tape, cartridge or program files. The other versions are listed below it with their publisher or group, language, and flags spelled out ("Cracked by Fairlight", "Trainer (+1) by Arcade").

Only archive items that match a LaunchBox Commodore 64 game are in the catalog. Intros, slideshows, disk collections and most fan hacks have no LaunchBox entry, so they are left out.

## Building

```
go run . -serve 8080
```

`go run .` needs no network access. It reads `data/catalog.json` and the thumbnails in `data/images/`, and writes the site to `dist/`. With `-serve`, it then serves `dist/` at `http://localhost:8080/` until you press Ctrl-C. The page fetches JSON, so open it through a web server rather than from `file://`. Any static file server works too, for example `python3 -m http.server -d dist`.

Run the tests with `go test ./...` and `go vet ./...`.

### Flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `-data` | `data` | Directory with `catalog.json`, `items.json` and `images/` |
| `-export` | `dist` | Where to write the site |
| `-serve` | 0 (off) | After exporting, serve the site on `localhost` at this port until interrupted |
| `-refresh` | off | Rebuild the snapshot from the Internet Archive and LaunchBox first |
| `-images` | on | With `-refresh`: download artwork. `-images=false` keeps the thumbnails already on disk |
| `-refresh-images` | off | With `-refresh`: regenerate thumbnails that already exist |
| `-launchbox-metadata` | | Local LaunchBox `Metadata.xml` or `.zip`; downloaded when empty |
| `-workers` | 8 | With `-refresh`: concurrent item and artwork downloads |
| `-limit` | 0 | With `-refresh`: only the first N games. Use a separate `-data` directory |

## Refreshing the data

```
go run . -refresh
```

No API keys are needed. A refresh:

1. Lists every item in `softwarelibrary_c64_games` and `softwarelibrary_c64_cracks` with the Internet Archive's scrape API, dropping repeats: identifier, description, title, creator, year, language, downloads, and the file the emulator starts.
2. Parses each item's TOSEC name, which starts the item description (`Ultima III Exodus (1984)(Origin Systems)(Side B)(Scenario Disk)`), into title, year, publisher, language, video standard, disk label, and dump flags. When the description has no TOSEC name, the title, creator and year fields are used instead. Items are then grouped into releases.
3. Downloads LaunchBox's `Metadata.zip` (about 110 MB) and keeps the Commodore 64 games (about 14,000) along with their alternate names and pictures.
4. Matches each release to a LaunchBox game by title and year. See below.
5. Reads the file list of every matched item (`archive.org/metadata/<id>/files`) to find its disk images, sizes, and screenshots. The lists are cached in `data/items.json`, so later refreshes only read new items.
6. Picks artwork for each game, downloads it, and writes JPEG thumbnails to `data/images/`.
7. Writes `data/catalog.json`, one game per line so diffs stay readable.

Thumbnails that already exist are skipped, so a refresh after the first one only downloads artwork for new games. File lists are cached the same way, so a refresh only reads the lists of items that are new. The first full refresh reads tens of thousands of item file lists and downloads tens of thousands of images; with `-workers 12` it takes a few hours.

### Matching

Titles are compared after ignoring case, punctuation, accents, bracketed text, version numbers (`v2.0`), edition notes (`Disk Version`, `US Version`), a `Commodore 64:` prefix, and one leading or trailing article (`Great Giana Sisters, The` matches `The Great Giana Sisters`). Roman numerals II to VIII match digits, so `Pitfall 2 Lost Caverns` matches `Pitfall II: Lost Caverns`. LaunchBox alternate names count as titles too.

Among games with the title:

- A same-year game wins, then one a year off, then one where either year is unknown. Ties go to the game with the most ratings.
- A gap of up to five years is accepted when only one game has the title, or when the TOSEC publisher matches the game's publisher or developer.
- Otherwise there is no match, so sequels and remakes with the same title stay apart.

If that finds nothing, the title is tried again without a trailing `1` or `Part 1` (`Bards Tale 1`, `Double Dragon 1`), and then against LaunchBox names with their subtitle removed. A subtitle match is used only when it identifies one game, if necessary by publisher (`Indiana Jones and the Last Crusade` by U.S. Gold is the action game, not the adventure).



### Artwork

- Box art is LaunchBox's `Box - Front`, then `Box - Front - Reconstructed`. Unregioned pictures win, then Europe, the United Kingdom, World, North America, and the United States, since the C64 sold mostly in Europe.
- Screenshots are LaunchBox's first gameplay picture and its title screen, with more gameplay pictures if needed. The archive's own screenshots fill in when LaunchBox has fewer than two: the cover screenshot it picked for the default version, then one from the middle of its emulator captures. They come second because for many cracked releases they are captured while the game is still loading and show only the blank BASIC screen.
- Without box art, the first screenshot is used.
- Box art fits within 300×400 and screenshots within 480×360 (never enlarged), as JPEG at quality 75 and 70.

### Catalog rules

- Genres are folded from the LaunchBox genres into about 15 filter chips. The original genres appear as tags in the detail drawer.
- LaunchBox release types other than "Released" (such as "Homebrew" or "Unreleased") appear as a badge on the tile.
- IDs come from the LaunchBox name (`Impossible Mission` is `impossible-mission`). Games that share a name get the year appended, then the LaunchBox ID.
- The weighted rating used by "Top rated" is `(votes × rating + 10 × mean) / (votes + 10)`. Unrated games sort last.

## Output

```
dist/index.html
dist/.nojekyll
dist/static/{app.js,style.css,placeholder.svg}
dist/data/catalog.json
dist/data/games/<id>.json
dist/images/box/<id>.jpg
dist/images/shots/<id>-<n>.jpg
```

## Deployment

`.github/workflows/pages.yml` runs `go test`, `go vet` and `go run .` on pushes to `main` and publishes `dist/` to GitHub Pages. It builds from the committed snapshot and thumbnails, so it needs no secrets and no network access. Enable Pages with the "GitHub Actions" source in the repository settings.

The thumbnails are committed under `data/images/`. Keep them stable between refreshes (existing thumbnails are not rewritten) so the repository does not grow with every update.

## Attribution

Game files and screenshots are hosted by the [Internet Archive](https://archive.org/details/softwarelibrary_c64). Descriptions, box art and ratings come from the [LaunchBox Games Database](https://gamesdb.launchbox-app.com/). This project is not affiliated with either. All game titles, box art and screenshots belong to their respective owners.
