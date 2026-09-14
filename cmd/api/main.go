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

	v1 "github.com/CORTA-11/core-api/cmd/api/handlers/v1"
	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/config"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/invitation"
	"github.com/CORTA-11/core-api/internal/logging"
	appMinio "github.com/CORTA-11/core-api/internal/minio"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/ratelimit"
	"github.com/CORTA-11/core-api/internal/realtime"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
)

func main() { os.Exit(realMain()) }

func realMain() int {
	logger := logging.New("core-api")
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

type dependencySet struct {
	pool        *pgxpool.Pool
	rdb         *redis.Client
	minioClient *minio.Client
}

func setupDependencies(ctx context.Context, cfg config.Config) (*dependencySet, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	if err := dependencyCheck(ctx, cfg.DependencyTimeout, pool.Ping); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	rdb := redis.NewClient(redisOptions)

	minioClient, err := appMinio.NewClient(cfg.MinIO.Endpoint, cfg.MinIO.AccessKey, cfg.MinIO.SecretKey, cfg.MinIO.UseSSL)
	if err != nil {
		pool.Close()
		_ = rdb.Close()
		return nil, err
	}
	if err := dependencyCheck(ctx, cfg.DependencyTimeout, func(checkCtx context.Context) error {
		return appMinio.VerifyBucket(checkCtx, minioClient, cfg.MinIO.Bucket)
	}); err != nil {
		pool.Close()
		_ = rdb.Close()
		return nil, fmt.Errorf("verify MinIO: %w", err)
	}

	return &dependencySet{
		pool:        pool,
		rdb:         rdb,
		minioClient: minioClient,
	}, nil
}

type domainModels struct {
	organizations    *service.OrganizationApplication
	invitations      *service.InvitationApplication
	teamTasks        *service.TeamTaskApplication
	documents        *service.DocumentApplication
	resourceBookings *service.ResourceApplication
	keyService       service.KeyService
	keyAccess        service.KeyAccessRequestService
	fileService      service.FileService
	chat             *service.ChatApplication
}

func setupDomainModels(cfg config.Config, deps *dependencySet) (*domainModels, error) {
	migrationSource, err := tenancy.EmbeddedMigrations()
	if err != nil {
		return nil, err
	}

	tenantExecutor := tenancy.NewExecutor(deps.pool)
	tenantResolver := tenancy.NewResolver(deps.pool, migrationSource)
	authorizer := authorization.NewAuthorizer(tenantResolver, tenantExecutor)

	cursorConfig := pagination.CodecConfig{Active: pagination.Key{
		ID: cfg.Cursor.ActiveKeyID, Secret: []byte(cfg.Cursor.ActiveSecret),
	}}
	if cfg.Cursor.PreviousKeyID != "" {
		cursorConfig.Previous = &pagination.Key{
			ID: cfg.Cursor.PreviousKeyID, Secret: []byte(cfg.Cursor.PreviousSecret),
		}
	}
	cursorCodec, err := pagination.NewCodec(cursorConfig)
	if err != nil {
		return nil, fmt.Errorf("configure cursor codec: %w", err)
	}

	organizations := service.NewOrganizationApplication(deps.pool, cursorCodec)

	invitationBinding, err := invitation.NewBinding([]byte(cfg.InvitationBindingSecret))
	if err != nil {
		return nil, fmt.Errorf("configure invitation binding: %w", err)
	}
	invitations := service.NewInvitationApplication(deps.pool, invitationBinding)

	teamTasks := service.NewTeamTaskApplication(authorizer, cursorCodec)

	roomCloser, err := realtime.NewDocumentRoomCloser(
		os.Getenv("COLLABORATION_INTERNAL_URL"), cfg.CollaborationServiceSecret,
		&http.Client{Timeout: cfg.DependencyTimeout},
	)
	if err != nil {
		return nil, fmt.Errorf("configure collaboration room closer: %w", err)
	}

	documents := service.NewDocumentApplication(authorizer, service.SocketTicketSecret(os.Getenv("JWT_SECRET")), roomCloser)

	resourceBookings := service.NewResourceApplication(authorizer)

	keyService := service.NewKeyService(deps.pool, authorizer)

	keyAccess := service.NewKeyAccessApplication(authorizer)

	fileService := service.NewFileService(deps.minioClient, cfg.MinIO.Bucket, authorizer)

	chat := service.NewChatApplication(authorizer, realtime.NewChatPublisherFromEnv(deps.rdb), service.SocketTicketSecret(os.Getenv("JWT_SECRET")))

	return &domainModels{
		organizations:    organizations,
		invitations:      invitations,
		teamTasks:        teamTasks,
		documents:        documents,
		resourceBookings: resourceBookings,
		keyService:       keyService,
		keyAccess:        keyAccess,
		fileService:      fileService,
		chat:             chat,
	}, nil
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	deps, err := setupDependencies(ctx, cfg)
	if err != nil {
		return err
	}
	defer deps.pool.Close()
	defer func() { _ = deps.rdb.Close() }()

	domain, err := setupDomainModels(cfg, deps)
	if err != nil {
		return err
	}
	go runInvitationCleanup(ctx, logger, domain.invitations)

	pool := deps.pool
	rdb := deps.rdb
	minioClient := deps.minioClient

	publicQueries := publicdb.New(pool)
	passwordHasher, err := identity.NewPasswordHasher(identity.HashConfig{})
	if err != nil {
		return fmt.Errorf("configure password hasher: %w", err)
	}
	credentialStore := identity.NewPostgresCredentialStore(publicQueries)
	credentialVerifier, err := identity.NewCredentialVerifier(ctx, credentialStore, passwordHasher)
	if err != nil {
		return fmt.Errorf("initialize credential verifier: %w", err)
	}
	sessionManager, err := session.NewManager(pool, []byte(cfg.CSRFSecret))
	if err != nil {
		return fmt.Errorf("configure session manager: %w", err)
	}
	rateLimiter, err := ratelimit.NewRedis(rdb, []byte(cfg.RateLimitSecret), cfg.RateLimitTimeout)
	if err != nil {
		return fmt.Errorf("configure rate limiter: %w", err)
	}
	loginGuard, err := ratelimit.NewLoginGuard(rateLimiter, cfg.RateLimits)
	if err != nil {
		return fmt.Errorf("configure login rate limit: %w", err)
	}
	registrationGuard, err := ratelimit.NewRegistrationGuard(rateLimiter, cfg.RateLimits.RegistrationIP)
	if err != nil {
		return fmt.Errorf("configure registration rate limit: %w", err)
	}
	administrative, err := ratelimit.NewAdministrativeMiddleware(rateLimiter, cfg.RateLimits.Administrative)
	if err != nil {
		return fmt.Errorf("configure administrative rate limit: %w", err)
	}
	readiness := map[string]v1.ReadinessCheck{
		"postgres": pool.Ping,
		"minio": func(checkCtx context.Context) error {
			return appMinio.VerifyBucket(checkCtx, minioClient, cfg.MinIO.Bucket)
		},
	}

	routerConfig := v1.RouterConfig{
		Manager:                    sessionManager,
		Verifier:                   credentialVerifier,
		Hasher:                     passwordHasher,
		Organizations:              domain.organizations,
		OrganizationMembers:        domain.organizations,
		TeamTasks:                  domain.teamTasks,
		Documents:                  domain.documents,
		Invitations:                domain.invitations,
		ResourceBookings:           domain.resourceBookings,
		Keys:                       domain.keyService,
		Files:                      domain.fileService,
		Chat:                       domain.chat,
		KeyAccess:                  domain.keyAccess,
		Environment:                cfg.Environment,
		Origins:                    cfg.HTTPOrigins,
		TrustedProxies:             cfg.TrustedProxies,
		Logger:                     logger,
		LoginGuard:                 loginGuard,
		RegistrationGuard:          registrationGuard,
		Administrative:             administrative,
		ReadinessChecks:            readiness,
		ReadinessTimeout:           cfg.DependencyTimeout,
		CollaborationServiceSecret: []byte(cfg.CollaborationServiceSecret),
	}

	router := v1.NewRouter(routerConfig)

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
	return serveAll(ctx, bindings, cfg.ShutdownTimeout)
}

func runInvitationCleanup(ctx context.Context, logger *slog.Logger, invitations *service.InvitationApplication) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := invitations.Cleanup(ctx)
			if err != nil {
				logger.Error("invitation cleanup failed", "error", err)
				continue
			}
			if count > 0 {
				logger.Info("expired invitations removed", "count", count)
			}
		}
	}
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
