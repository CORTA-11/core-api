package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/CORTA-11/core-api/internal/logging"
	"github.com/CORTA-11/core-api/internal/seeding"
	"github.com/jackc/pgx/v5"
)

// main runs the command.
func main() {
	slog.SetDefault(logging.New("seed"))
	ctx := context.Background()

	databaseURL := os.Getenv("MIGRATION_DATABASE_URL")
	if databaseURL == "" {
		slog.Error("MIGRATION_DATABASE_URL is required")
		os.Exit(1)
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		slog.Error("connect to database failed", "error", err)
		os.Exit(1)
	}
	defer func(conn *pgx.Conn, ctx context.Context) {
		err := conn.Close(ctx)
		if err != nil {
			slog.Error("close database connection failed", "error", err)
		}
	}(conn, ctx)

	if err := seeding.Apply(ctx, conn, "db/seeds"); err != nil {
		slog.Error("seeding failed", "error", err)
		os.Exit(1)
	}

	slog.Info("seeding completed")
}
