package main

import (
	"context"
	"log"
	"os"

	"github.com/CORTA-11/core-api/internal/seeding"
	"github.com/jackc/pgx/v5"
)

// main runs the command.
func main() {
	ctx := context.Background()

	databaseURL := os.Getenv("MIGRATION_DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("MIGRATION_DATABASE_URL is required")
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer func(conn *pgx.Conn, ctx context.Context) {
		err := conn.Close(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}(conn, ctx)

	if err := seeding.Apply(ctx, conn, "db/seeds"); err != nil {
		log.Fatal(err)
	}

	log.Println("seeding completed")
}
