package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer func(conn *pgx.Conn, ctx context.Context) {
		err := conn.Close(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}(conn, ctx)

	files, err := filepath.Glob("db/seeds/*.sql")
	if err != nil {
		log.Fatal(err)
	}

	sort.Strings(files)

	for _, file := range files {
		log.Println("running seed:", file)

		// #nosec G304 -- file paths are from a hardcoded migration list
		sql, err := os.ReadFile(file)
		if err != nil {
			log.Fatal(err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			log.Fatal(err)
		}

		_, err = tx.Exec(ctx, string(sql))
		if err != nil {
			err := tx.Rollback(ctx)
			if err != nil {
				log.Fatal(err)
				return
			}
			log.Fatalf("failed seed %s: %v", file, err)
		}

		if err := tx.Commit(ctx); err != nil {
			log.Fatal(err)
		}
	}

	log.Println("seeding completed")
}
