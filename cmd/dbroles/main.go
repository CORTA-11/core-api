// Command dbroles assigns local/deployment secrets to the fixed operational
// roles after the administrator has applied the public role migration.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/CORTA-11/core-api/internal/dbroles"
	"github.com/CORTA-11/core-api/internal/logging"
	"github.com/joho/godotenv"
)

// main runs the command.
func main() {
	_ = godotenv.Load()
	slog.SetDefault(logging.New("dbroles"))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config, err := dbroles.LoadConfig(os.LookupEnv)
	if err != nil {
		slog.Error("database role bootstrap failed", "error", err)
		os.Exit(1)
	}
	if err := dbroles.Configure(ctx, config); err != nil {
		slog.Error("database role bootstrap failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database role passwords are configured")
}
