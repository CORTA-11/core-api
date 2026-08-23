// Package seeding applies ordered, idempotent development seed files.
package seeding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5"
)

// Beginner is the transaction boundary required by Apply.
type Beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// Apply executes each SQL file in lexical order and in its own transaction.
func Apply(ctx context.Context, database Beginner, directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read seed directory: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			files = append(files, filepath.Join(directory, entry.Name()))
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("seed directory %s contains no SQL files", directory)
	}
	sort.Strings(files)

	for _, file := range files {
		// #nosec G304 -- filenames come only from the configured seed directory.
		source, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read seed %s: %w", filepath.Base(file), err)
		}
		tx, err := database.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin seed %s: %w", filepath.Base(file), err)
		}
		if _, err := tx.Exec(ctx, string(source)); err != nil {
			_ = tx.Rollback(context.Background())
			return fmt.Errorf("apply seed %s: %w", filepath.Base(file), err)
		}
		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(context.Background())
			return fmt.Errorf("commit seed %s: %w", filepath.Base(file), err)
		}
	}
	return nil
}
