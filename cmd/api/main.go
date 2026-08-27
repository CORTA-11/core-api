package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CORTA-11/core-api/cmd/api/handlers"
	"github.com/CORTA-11/core-api/internal/config"
	"github.com/CORTA-11/core-api/internal/httpx"
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
		"minio": func(checkCtx context.Context) error {
			return appMinio.VerifyBucket(checkCtx, minioClient, cfg.MinIO.Bucket)
		},
	}
	router := handlers.NewRouter(handlers.RouterConf{
		OrgService: &orgService, TeamService: &teamService,
		TaskService: &taskService, UserService: &userService, FileService: &fileService,
		TokenService: &tokenService, OrgUserService: &orgUserService, ReadinessChecks: readiness,
		ReadinessTimeout: cfg.DependencyTimeout,
		OrgAvailability:  availability, TenantResolver: tenantResolver,
		TrustedProxies: cfg.TrustedProxies,
	})
	router.SetupRoutes()
	server := httpx.NewServer(cfg.HTTPAddr, router.Handler(), httpx.ServerTimeouts{
		ReadHeader: cfg.HTTPReadHeaderTimeout, Read: cfg.HTTPReadTimeout,
		Write: cfg.HTTPWriteTimeout, Idle: cfg.HTTPIdleTimeout,
	}, logger)
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTPAddr, err)
	}
	slog.Info("core-api listening", "addr", listener.Addr().String())
	bindings := []serverBinding{{name: "API", server: server, listener: listener}}
	if cfg.PprofEnabled {
		diagnostic, diagnosticErr := newDiagnosticServer(cfg, logger)
		if diagnosticErr != nil {
			_ = listener.Close()
			return diagnosticErr
		}
		diagnosticListener, listenErr := net.Listen("tcp", cfg.PprofAddr)
		if listenErr != nil {
			_ = listener.Close()
			return fmt.Errorf("listen for diagnostics on %s: %w", cfg.PprofAddr, listenErr)
		}
		slog.Info("diagnostics listening", "addr", diagnosticListener.Addr().String())
		bindings = append(bindings, serverBinding{name: "diagnostics", server: diagnostic, listener: diagnosticListener})
	}
	return serveAll(ctx, bindings, cfg.ShutdownTimeout)
}

func newDiagnosticServer(cfg config.Config, logger *slog.Logger) (*http.Server, error) {
	if !cfg.PprofEnabled || cfg.Environment == "production" {
		return nil, errors.New("diagnostics are not permitted")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("POST /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	return httpx.NewServer(cfg.PprofAddr, mux, httpx.ServerTimeouts{
		ReadHeader: cfg.HTTPReadHeaderTimeout, Read: cfg.HTTPReadTimeout,
		Write: cfg.HTTPWriteTimeout, Idle: cfg.HTTPIdleTimeout,
	}, logger), nil
}

func dependencyCheck(parent context.Context, timeout time.Duration, check func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return check(ctx)
}

func serve(ctx context.Context, server *http.Server, listener net.Listener, shutdownTimeout time.Duration) error {
	return serveAll(ctx, []serverBinding{{name: "HTTP", server: server, listener: listener}}, shutdownTimeout)
}

type serverBinding struct {
	name     string
	server   *http.Server
	listener net.Listener
}

type serverResult struct {
	name string
	err  error
}

func serveAll(ctx context.Context, bindings []serverBinding, shutdownTimeout time.Duration) error {
	serveResult := make(chan serverResult, len(bindings))
	for _, binding := range bindings {
		go func() { serveResult <- serverResult{binding.name, binding.server.Serve(binding.listener)} }()
	}
	var runtimeResult *serverResult
	select {
	case result := <-serveResult:
		runtimeResult = &result
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	for _, binding := range bindings {
		if err := binding.server.Shutdown(shutdownCtx); err != nil {
			for _, closing := range bindings {
				_ = closing.server.Close()
			}
			return fmt.Errorf("shutdown %s server: %w", binding.name, err)
		}
	}
	remaining := len(bindings)
	if runtimeResult != nil {
		remaining--
	}
	for range remaining {
		result := <-serveResult
		if !errors.Is(result.err, http.ErrServerClosed) && runtimeResult == nil {
			runtimeResult = &result
		}
	}
	if runtimeResult != nil && !errors.Is(runtimeResult.err, http.ErrServerClosed) {
		return fmt.Errorf("serve %s HTTP: %w", runtimeResult.name, runtimeResult.err)
	}
	return nil
}
