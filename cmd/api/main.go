package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CORTA-11/core-api/cmd/api/handlers"
	"github.com/CORTA-11/core-api/internal/config"
	"github.com/CORTA-11/core-api/internal/identity"
	appMinio "github.com/CORTA-11/core-api/internal/minio"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() { os.Exit(realMain()) }

func realMain() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo, AddSource: true}))
	slog.SetDefault(logger)
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("unable to load .env", "error", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil {
		slog.Error("core-api stopped with an error", "error", err)
		return 1
	}
	return 0
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	defer pool.Close()
	if err := dependencyCheck(ctx, cfg.DependencyTimeout, pool.Ping); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}

	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("parse Redis URL: %w", err)
	}
	rdb := redis.NewClient(redisOptions)
	defer func() { _ = rdb.Close() }()
	if err := dependencyCheck(ctx, cfg.DependencyTimeout, func(checkCtx context.Context) error { return rdb.Ping(checkCtx).Err() }); err != nil {
		return fmt.Errorf("ping Redis: %w", err)
	}

	minioClient, err := appMinio.NewClient(cfg.MinIO.Endpoint, cfg.MinIO.AccessKey, cfg.MinIO.SecretKey, cfg.MinIO.UseSSL)
	if err != nil {
		return err
	}
	if err := dependencyCheck(ctx, cfg.DependencyTimeout, func(checkCtx context.Context) error {
		return appMinio.VerifyBucket(checkCtx, minioClient, cfg.MinIO.Bucket)
	}); err != nil {
		return fmt.Errorf("verify MinIO: %w", err)
	}

	publicQueries := publicdb.New(pool)
	// The API derives availability from its own embedded migration set, so a
	// rolling deployment fails tenant requests closed until schemas match the
	// binary that will serve them.
	migrationSource, err := tenancy.EmbeddedMigrations()
	if err != nil {
		return err
	}
	availability := tenancy.NewAvailabilityChecker(pool, migrationSource)
	orgService := service.NewOrgService(pool, publicQueries)
	tenantExecutor := tenancy.NewExecutor(pool)
	tenantResolver := tenancy.NewResolver(pool, migrationSource)
	teamService := service.NewTeamService(tenantExecutor)
	taskService := service.NewTaskService(tenantExecutor)
	tokenService := service.NewTokenService(cfg.JWTSecret)
	passwordHasher, err := identity.NewPasswordHasher(identity.HashConfig{})
	if err != nil {
		return fmt.Errorf("configure password hasher: %w", err)
	}
	credentialStore := identity.NewPostgresCredentialStore(publicQueries)
	credentialVerifier, err := identity.NewCredentialVerifier(ctx, credentialStore, passwordHasher)
	if err != nil {
		return fmt.Errorf("initialize credential verifier: %w", err)
	}
	userService := service.NewUserService(publicQueries, tokenService, passwordHasher, credentialVerifier)
	orgUserService := service.NewOrgUserService(pool, publicQueries)
	fileService := service.NewFileService(minioClient, cfg.MinIO.Bucket)
	readiness := map[string]handlers.ReadinessCheck{
		"postgres": pool.Ping,
		"redis":    func(checkCtx context.Context) error { return rdb.Ping(checkCtx).Err() },
		"minio": func(checkCtx context.Context) error {
			return appMinio.VerifyBucket(checkCtx, minioClient, cfg.MinIO.Bucket)
		},
	}
	router := handlers.NewRouter(handlers.RouterConf{
		OrgService: &orgService, TeamService: &teamService,
		TaskService: &taskService, UserService: &userService, FileService: &fileService,
		TokenService: &tokenService, OrgUserService: &orgUserService, ReadinessChecks: readiness,
		ReadinessTimeout: cfg.DependencyTimeout, PprofEnabled: cfg.PprofEnabled,
		OrgAvailability: availability, TenantResolver: tenantResolver,
	})
	router.SetupRoutes()
	server := &http.Server{
		Addr: cfg.HTTPAddr, Handler: router.Handler(), ReadTimeout: cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout, IdleTimeout: cfg.HTTPIdleTimeout,
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTPAddr, err)
	}
	slog.Info("core-api listening", "addr", listener.Addr().String())
	return serve(ctx, server, listener, cfg.ShutdownTimeout)
}

func dependencyCheck(parent context.Context, timeout time.Duration, check func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return check(ctx)
}

func serve(ctx context.Context, server *http.Server, listener net.Listener, shutdownTimeout time.Duration) error {
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		if err := <-serveResult; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP during shutdown: %w", err)
		}
		return nil
	}
}
