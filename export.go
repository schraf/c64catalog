package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// catalogResponse is the JSON shape consumed by the static frontend.
type catalogResponse struct {
	Games   []IndexGame  `json:"games"`
	Genres  []FacetCount `json:"genres"`
	Media   []FacetCount `json:"media"`
	YearMin int          `json:"year_min"`
	YearMax int          `json:"year_max"`
	Total   int          `json:"total"`
}

// exportStatic writes a self-contained static site to outDir. Thumbnails are
// copied from dataDir/images.
func exportStatic(outDir, dataDir string, webRoot fs.FS, catalog *Catalog) error {
	outDir = filepath.Clean(outDir)
	if outDir == "." || outDir == ".." || outDir == string(filepath.Separator) {
		return fmt.Errorf("refusing to export into %q", outDir)
	}
	if strings.HasPrefix(outDir, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to export outside the working directory: %q", outDir)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".git")); err == nil {
		return fmt.Errorf("refusing to replace %s: it contains a .git directory", outDir)
	}
	if _, err := os.Stat(filepath.Join(outDir, snapshotName)); err == nil {
		return fmt.Errorf("refusing to replace %s: it contains the catalog snapshot", outDir)
	}

	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("clear %s: %w", outDir, err)
	}
	for _, dir := range []string{
		filepath.Join(outDir, "static"),
		filepath.Join(outDir, "data", "games"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	if err := copyWebFiles(webRoot, outDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, ".nojekyll"), nil, 0o644); err != nil {
		return fmt.Errorf("write .nojekyll: %w", err)
	}

	response := catalogResponse{
		Games:   catalog.Index(),
		Genres:  catalog.genres,
		Media:   catalog.media,
		YearMin: catalog.yearMin,
		YearMax: catalog.yearMax,
		Total:   catalog.Total(),
	}
	if err := writeJSONFile(filepath.Join(outDir, "data", "catalog.json"), response); err != nil {
		return err
	}

	copied := map[string]bool{}
	for _, game := range catalog.games {
		if err := writeJSONFile(filepath.Join(outDir, "data", "games", game.ID+".json"), game); err != nil {
			return err
		}
		images := append([]string{game.BoxArt}, game.Screenshots...)
		for _, rel := range images {
			if rel == "" || copied[rel] {
				continue
			}
			copied[rel] = true
			if err := copyImage(dataDir, outDir, rel); err != nil {
				return fmt.Errorf("%s: %w", game.ID, err)
			}
		}
	}
	return nil
}

// copyImage copies one thumbnail, rejecting paths that escape the images tree.
func copyImage(dataDir, outDir, rel string) error {
	if !strings.HasPrefix(rel, "images/") || strings.Contains(rel, "..") {
		return fmt.Errorf("unexpected image path %q", rel)
	}
	data, err := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(rel)))
	if err != nil {
		return fmt.Errorf("read image: %w (run with -refresh to rebuild the images)", err)
	}
	dest := filepath.Join(outDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

func copyWebFiles(webRoot fs.FS, outDir string) error {
	entries, err := fs.ReadDir(webRoot, ".")
	if err != nil {
		return fmt.Errorf("read embedded web files: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(webRoot, entry.Name())
		if err != nil {
			return fmt.Errorf("read web/%s: %w", entry.Name(), err)
		}
		dest := filepath.Join(outDir, "static", entry.Name())
		if entry.Name() == "index.html" {
			dest = filepath.Join(outDir, "index.html")
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
