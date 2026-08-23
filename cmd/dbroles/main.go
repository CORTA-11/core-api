// Command dbroles assigns local/deployment secrets to the fixed operational
// roles after the administrator has applied the public role migration.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/CORTA-11/core-api/internal/dbroles"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config, err := dbroles.LoadConfig(os.LookupEnv)
	if err != nil {
		log.Printf("database role bootstrap failed: %v", err)
		os.Exit(1)
	}
	if err := dbroles.Configure(ctx, config); err != nil {
		log.Printf("database role bootstrap failed: %v", err)
		os.Exit(1)
	}
	log.Print("database role passwords are configured")
}
