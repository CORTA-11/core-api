package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CORTA-11/core-api/internal/querycheck"
)

const queryDirectory = "db/queries"

// main runs the command.
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

	repository, err := os.OpenRoot(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open repository: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = repository.Close() }()
	err = fs.WalkDir(repository.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (path == ".git" || path == ".cache") {
			return fs.SkipDir
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := repository.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, issue := range querycheck.CheckSchemaBoundary(filepath.ToSlash(path), source) {
			fmt.Fprintln(os.Stderr, issue.Error())
			issueCount++
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk production Go sources: %v\n", err)
		os.Exit(1)
	}
	if issueCount > 0 {
		os.Exit(1)
	}
}
