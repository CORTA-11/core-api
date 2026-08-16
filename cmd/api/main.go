package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/CORTA-11/core-api/cmd/api/handlers"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	}))
	slog.SetDefault(logger)

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("unable to connect to database", "error", err)
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("unable to ping database", "error", err)
		return
	}

	queries := repository.New(pool)

	orgService := service.NewOrgService(pool, queries)
	teamService := service.NewTeamService(pool, queries)
	taskService := service.NewTaskService(pool, queries)

	router := handlers.NewRouter(handlers.RouterConf{
		DB:          pool,
		Queries:     queries,
		OrgService:  &orgService,
		TeamService: &teamService,
		TaskService: &taskService,
	})

	router.SetupRoutes()

	s := http.Server{
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		Addr:         ":8080",
		Handler:      router.Handler(),
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	slog.Info("core-api listening", "addr", s.Addr)
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
		return
	}
}
