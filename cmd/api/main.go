package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()

	// TODO: Use pgxpool
	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("unable to connect to database: %v\n", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			log.Printf("error closing database connection: %v", err)
		}
	}()

	queries := repository.New(conn)

	router := NewRouter(queries)
	router.SetupRoutes()

	s := http.Server{
		WriteTimeout: time.Second * 5,
		ReadTimeout:  time.Second * 5,
		Addr:         ":8080",
		Handler:      router.Handler(),
	}

	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}
