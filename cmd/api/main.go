package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/CORTA-11/core-api/cmd/api/handlers"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("unable to ping database: %v\n", err)
	}

	queries := repository.New(pool)

	orgService := service.NewOrgService(pool, queries)
	teamService := service.NewTeamService(pool, queries)

	router := handlers.NewRouter(pool, queries, orgService, teamService)
	router.SetupRoutes()

	s := http.Server{
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		Addr:         ":8080",
		Handler:      router.Handler(),
	}

	log.Printf("core-api listening on %s", s.Addr)
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}
