package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/CORTA-11/core-api/internal/querycheck"
)

const queryDirectory = "db/queries"

func main() {
	root, err := os.OpenRoot(queryDirectory)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open query directory: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = root.Close() }()

	var paths []string
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && filepath.Ext(path) == ".sql" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk query sources: %v\n", err)
		os.Exit(1)
	}

	sort.Strings(paths)
	issueCount := 0
	for _, path := range paths {
		source, readErr := root.ReadFile(path)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", filepath.Join(queryDirectory, path), readErr)
			os.Exit(1)
		}
		for _, issue := range querycheck.Check(filepath.Join(queryDirectory, path), source) {
			fmt.Fprintln(os.Stderr, issue.Error())
			issueCount++
		}
	}
	if issueCount > 0 {
		os.Exit(1)
	}
}
