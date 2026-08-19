package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const publicMigrationsDir = "file://db/migrations/public"

type migrator interface {
	Up() error
	Down() error
	Steps(int) error
	Close() (error, error)
}

type migratorFactory func(sourceURL, databaseURL string) (migrator, error)

func main() {
	if err := run(os.Args[1:], os.Getenv, newMigrator); err != nil {
		log.Printf("migration failed: %v", err)
		os.Exit(1)
	}
}

func newMigrator(sourceURL, databaseURL string) (migrator, error) {
	return migrate.New(sourceURL, databaseURL)
}

func run(args []string, getenv func(string) string, factory migratorFactory) error {
	if len(args) != 1 {
		return errors.New("usage: migrate [up|up-all|down|down-all]")
	}

	m, err := factory(publicMigrationsDir, getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("initialize migrator: %w", err)
	}
	defer func() {
		sourceErr, databaseErr := m.Close()
		if sourceErr != nil {
			log.Printf("migration source close failed: %v", sourceErr)
		}
		if databaseErr != nil {
			log.Printf("migration database close failed: %v", databaseErr)
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
	default:
		return fmt.Errorf("unknown migration command %q", args[0])
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("%s migration: %w", args[0], err)
	}
	// #nosec G706 -- command is validated against the fixed switch cases above.
	log.Printf("migration completed: %s", args[0])
	return nil
}
