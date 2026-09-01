package main

import (
	"context"
	"log"
	"os"

	"github.com/CORTA-11/core-api/internal/config"
	appMinio "github.com/CORTA-11/core-api/internal/minio"
)

// main runs the command.
func main() {
	if err := run(context.Background()); err != nil {
		log.Printf("bootstrap failed: %v", err)
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
	log.Printf("MinIO bucket %q is ready", cfg.MinIO.Bucket)
	return nil
}
