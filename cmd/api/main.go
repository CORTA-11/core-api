package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/CORTA-11/core-api/cmd/api/handlers"
	appMinio "github.com/CORTA-11/core-api/internal/minio"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	}))
	slog.SetDefault(logger)

	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("unable to load .env", "error", err)
	}

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
	tokenService := service.NewTokenService()
	userService := service.NewUserService(pool, queries, tokenService)

	minioEndpoint := os.Getenv("MINIO_ENDPOINT")
	minioAccessKey := os.Getenv("MINIO_ACCESS_KEY")
	minioSecretKey := os.Getenv("MINIO_SECRET_KEY")
	minioBucketName := os.Getenv("MINIO_BUCKET_NAME")
	minioUseSSL, _ := strconv.ParseBool(os.Getenv("MINIO_USE_SSL"))

	minioClient := appMinio.NewMinioClient(minioEndpoint, minioAccessKey, minioSecretKey, minioUseSSL)
	appMinio.CreateBucket(context.Background(), minioClient, minioBucketName)

	fileService := service.NewFileService(minioClient, minioBucketName)

	redisHost := os.Getenv("REDIS_HOST")
	redisPort := os.Getenv("REDIS_PORT")
	redisAddr := redisHost + ":" + redisPort
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
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
