package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/CORTA-11/core-api/internal/logging"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const publicMigrationsDir = "file://db/migrations/public"

type migrator interface {
	Up() error
	Down() error
	Steps(int) error
	Version() (uint, bool, error)
	Close() (error, error)
}

type migratorFactory func(sourceURL, databaseURL string) (migrator, error)

// main runs the command.
func main() {
	slog.SetDefault(logging.New("migrate"))
	if err := run(os.Args[1:], os.Getenv, newMigrator); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

// newMigrator news migrator.
func newMigrator(sourceURL, databaseURL string) (migrator, error) {
	return migrate.New(sourceURL, databaseURL)
}

// run runs the command workflow.
func run(args []string, getenv func(string) string, factory migratorFactory) error {
	if len(args) != 1 {
		return errors.New("usage: migrate [up|up-all|down|down-all|status]")
	}

	databaseURL := getenv("MIGRATION_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("MIGRATION_DATABASE_URL is required")
	}
	m, err := factory(publicMigrationsDir, databaseURL)
	if err != nil {
		return fmt.Errorf("initialize migrator: %w", err)
	}
	defer func() {
		sourceErr, databaseErr := m.Close()
		if sourceErr != nil {
			slog.Error("migration source close failed", "error", sourceErr)
		}
		if databaseErr != nil {
			slog.Error("migration database close failed", "error", databaseErr)
		}
	}()

	switch args[0] {
	case "up-all":
		err = m.Up()
	case "down-all":
		err = m.Down()
	case "up":
		err = m.Steps(1)
	case "down":
		err = m.Steps(-1)
	case "status":
		var version uint
		var dirty bool
		version, dirty, err = m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			slog.Info("public migration status", "version", 0, "dirty", false)
			return nil
		}
		if err == nil {
			slog.Info("public migration status", "version", version, "dirty", dirty)
			// A dirty public registry cannot safely coordinate tenant lifecycle
			// transitions, so status is also a deployment gate.
			if dirty {
				return errors.New("public migration state is dirty")
			}
			return nil
		}
	default:
		return fmt.Errorf("unknown migration command %q", args[0])
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("%s migration: %w", args[0], err)
	}
	// #nosec G706 -- command is validated against the fixed switch cases above.
	slog.Info("migration completed", "command", args[0])
	return nil
}
