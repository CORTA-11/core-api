package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/CORTA-11/core-api/internal/config"
	"github.com/CORTA-11/core-api/internal/logging"
	appMinio "github.com/CORTA-11/core-api/internal/minio"
)

// main runs the command.
func main() {
	slog.SetDefault(logging.New("bootstrap"))
	if err := run(context.Background()); err != nil {
		slog.Error("bootstrap failed", "error", err)
		os.Exit(1)
	}
}

// run runs the command workflow.
func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client, err := appMinio.NewClient(cfg.MinIO.Endpoint, cfg.MinIO.AccessKey, cfg.MinIO.SecretKey, cfg.MinIO.UseSSL)
	if err != nil {
		return err
	}
	if err := appMinio.EnsureBucket(ctx, client, cfg.MinIO.Bucket); err != nil {
		return err
	}
	slog.Info("MinIO bucket is ready", "bucket", cfg.MinIO.Bucket)
	return nil
}
