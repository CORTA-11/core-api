package main

import (
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate [up|up-all|down|down-all]")
	}

	m, err := migrate.New(
		"file://db/migrations",
		os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil {
			log.Println("source close error:", sourceErr)
		}
		if dbErr != nil {
			log.Println("database close error:", dbErr)
		}
	}()

	switch os.Args[1] {
	case "up-all":
		err = m.Up()
	case "down-all":
		err = m.Down()
	case "up":
		err = m.Steps(1)
	case "down":
		err = m.Steps(-1)
	default:
		// #nosec G706 -- command-line argument is only used by trusted developers
		log.Fatalf("unknown command: %s. Use up or down", os.Args[1])
	}

	if err != nil {
		log.Fatal(err)
	}

	// #nosec G706 -- command-line argument is only used by trusted developers
	log.Println("migration completed:", os.Args[1])
}
