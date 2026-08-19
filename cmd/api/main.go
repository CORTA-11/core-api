package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/CORTA-11/core-api/cmd/api/handlers"
	"github.com/CORTA-11/core-api/internal/config"
	appMinio "github.com/CORTA-11/core-api/internal/minio"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	}))
	slog.SetDefault(logger)

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		return
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
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

	orgService := service.NewOrgService(pool, queries, cfg.DatabaseURL)
	teamService := service.NewTeamService(pool, queries)
	taskService := service.NewTaskService(pool, queries)
	tokenService := service.NewTokenService(cfg.JWTSecret)
	userService := service.NewUserService(pool, queries, tokenService)

	minioClient, err := appMinio.NewClient(cfg.MinIO.Endpoint, cfg.MinIO.AccessKey, cfg.MinIO.SecretKey, cfg.MinIO.UseSSL)
	if err != nil {
		slog.Error("unable to create MinIO client", "error", err)
		return
	}
	if err := appMinio.VerifyBucket(ctx, minioClient, cfg.MinIO.Bucket); err != nil {
		slog.Error("MinIO bucket is not ready", "error", err)
		return
	}

	fileService := service.NewFileService(minioClient, cfg.MinIO.Bucket)

	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		slog.Error("invalid Redis URL", "error", err)
		return
	}
	rdb := redis.NewClient(redisOptions)
	defer func() { _ = rdb.Close() }()

	cacheService := service.NewCacheService(rdb)

	cachedTeamService := service.NewCachedTeamService(teamService, cacheService)

	router := handlers.NewRouter(handlers.RouterConf{
		DB:           pool,
		Queries:      queries,
		OrgService:   &orgService,
		TeamService:  &cachedTeamService,
		TaskService:  &taskService,
		UserService:  &userService,
		FileService:  &fileService,
		TokenService: &tokenService,
	})

	router.SetupRoutes()

	s := http.Server{
		WriteTimeout: cfg.HTTPWriteTimeout,
		ReadTimeout:  cfg.HTTPReadTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
		Addr:         cfg.HTTPAddr,
		Handler:      router.Handler(),
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	slog.Info("core-api listening", "addr", s.Addr)
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
		return
	}
}
