package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"
)

//go:embed web
var webFiles embed.FS

func main() {
	dataDir := flag.String("data", "data", "directory containing catalog.json, items.json and images/")
	exportDir := flag.String("export", "dist", "directory to write the static site")
	refreshFlag := flag.Bool("refresh", false, "rebuild the snapshot from the Internet Archive and LaunchBox before exporting")
	useImages := flag.Bool("images", true, "with -refresh, download and scale artwork (false keeps thumbnails already on disk)")
	forceImages := flag.Bool("refresh-images", false, "with -refresh, regenerate thumbnails that already exist")
	launchBoxMetadata := flag.String("launchbox-metadata", "", "local LaunchBox Metadata.xml or .zip; downloads the current export when empty")
	workers := flag.Int("workers", 8, "with -refresh, concurrent item and artwork downloads")
	limit := flag.Int("limit", 0, "with -refresh, only process the first N games (for trying things out; use a separate -data directory)")
	servePort := flag.Int("serve", 0, "after exporting, serve the site on localhost at this port until interrupted")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *refreshFlag {
		opts := refreshOptions{
			DataDir:       *dataDir,
			LaunchBoxPath: *launchBoxMetadata,
			UseImages:     *useImages,
			ForceImages:   *forceImages,
			Workers:       *workers,
			Limit:         *limit,
		}
		if err := refresh(ctx, newFetcher(), opts, log.Printf); err != nil {
			log.Fatal(err)
		}
	}

	games, err := loadSnapshot(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	catalog, err := buildCatalog(games)
	if err != nil {
		log.Fatal(err)
	}

	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}
	if err := exportStatic(*exportDir, *dataDir, webRoot, catalog); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d games)", *exportDir, catalog.Total())

	if *servePort != 0 {
		if err := serveDir(ctx, *exportDir, *servePort, log.Printf); err != nil {
			log.Fatal(err)
		}
	}
}
