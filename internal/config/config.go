package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/ratelimit"
	"github.com/redis/go-redis/v9"
)

const (
	DevelopmentCSRFSecret          = "development-only-csrf-secret-change-me"
	DevelopmentCursorKeyID         = "development-v1"
	DevelopmentCursorSecret        = "development-only-cursor-secret-change-me"
	DevelopmentRateLimitSecret     = "development-only-rate-limit-secret-change-me"
	DevelopmentInvitationSecret    = "development-only-invitation-secret-change-me"
	DevelopmentCollaborationSecret = "development-collaboration-service-secret-change-me"
)

type Config struct {
	Environment                string
	HTTPAddr                   string
	HTTPOrigins                httpx.OriginPolicy
	TrustedProxies             httpx.TrustedProxies
	HTTPReadHeaderTimeout      time.Duration
	HTTPReadTimeout            time.Duration
	HTTPWriteTimeout           time.Duration
	HTTPIdleTimeout            time.Duration
	ShutdownTimeout            time.Duration
	DependencyTimeout          time.Duration
	DatabaseURL                string
	RedisURL                   string
	MinIO                      MinIO
	CSRFSecret                 string
	Cursor                     CursorKeys
	RateLimitSecret            string
	InvitationBindingSecret    string
	CollaborationServiceSecret string
	RateLimitTimeout           time.Duration
	RateLimits                 ratelimit.Policies
	PprofEnabled               bool
	PprofAddr                  string
}

type CursorKeys struct {
	ActiveKeyID    string
	ActiveSecret   string
	PreviousKeyID  string
	PreviousSecret string
}

type MinIO struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type lookupFunc func(string) (string, bool)

func Load() (Config, error) {
	return LoadFrom(runtimeLookup(os.LookupEnv))
}

// runtimeLookup preserves explicit environment values, then resolves local or
// mounted secret files. It also assembles DATABASE_URL because PostgreSQL
// passwords cannot be consumed independently by pgx.
func runtimeLookup(environment lookupFunc) lookupFunc {
	mountedFiles := map[string]string{ // #nosec G101 -- values are mounted secret filenames, not credentials.
		"DB_RUNTIME_PASSWORD":       "db_runtime_password.txt",
		"MINIO_ACCESS_KEY":          "minio_access_key",
		"MINIO_SECRET_KEY":          "minio_secret_key.txt",
		"RATE_LIMIT_SECRET":         "redis_limit_secret.txt",
		"INVITATION_BINDING_SECRET": "redis_invitation_binding_secret.txt",
		"CSRF_SECRET":               "csrf_secret.txt",
	}
	secretDir := ".local_secrets"
	if configured, ok := environment("LOCAL_SECRETS_DIR"); ok && configured != "" {
		secretDir = configured
	} else if _, err := os.Stat(secretDir); errors.Is(err, os.ErrNotExist) {
		if _, mountedErr := os.Stat("/run/secrets"); mountedErr == nil {
			secretDir = "/run/secrets"
		}
	}

	lookup := func(name string) (string, bool) {
		if raw, ok := environment(name); ok && raw != "" {
			return raw, true
		}
		filename, ok := mountedFiles[name]
		if !ok {
			return "", false
		}
		raw, err := os.ReadFile(secretDir + string(os.PathSeparator) + filename)
		if err != nil {
			return "", false
		}
		return strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r"), true
	}

	return func(name string) (string, bool) {
		if raw, ok := lookup(name); ok {
			return raw, true
		}
		if name != "DATABASE_URL" {
			return "", false
		}
		host, hostOK := lookup("DB_HOST")
		port, portOK := lookup("DB_PORT")
		database, databaseOK := lookup("DB_NAME")
		password, passwordOK := lookup("DB_RUNTIME_PASSWORD")
		if !hostOK || !portOK || !databaseOK || !passwordOK {
			return "", false
		}
		databaseURL := url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword("synodus_runtime", password),
			Host:     net.JoinHostPort(host, port),
			Path:     database,
			RawQuery: "sslmode=disable",
		}
		return databaseURL.String(), true
	}
}

func LoadFrom(lookup lookupFunc) (Config, error) {
	var problems []error
	config := Config{
		Environment:                valueOrDefault(lookup, "APP_ENV", "development"),
		HTTPAddr:                   valueOrDefault(lookup, "HTTP_ADDR", ":8080"),
		CSRFSecret:                 valueOrDefault(lookup, "CSRF_SECRET", DevelopmentCSRFSecret),
		RateLimitSecret:            valueOrDefault(lookup, "RATE_LIMIT_SECRET", DevelopmentRateLimitSecret),
		InvitationBindingSecret:    valueOrDefault(lookup, "INVITATION_BINDING_SECRET", DevelopmentInvitationSecret),
		CollaborationServiceSecret: valueOrDefault(lookup, "COLLABORATION_SERVICE_SECRET", DevelopmentCollaborationSecret),
		RateLimitTimeout:           250 * time.Millisecond,
		RateLimits:                 ratelimit.DefaultPolicies(),
		PprofAddr:                  valueOrDefault(lookup, "PPROF_ADDR", "127.0.0.1:6060"),
		Cursor: CursorKeys{
			ActiveKeyID:    valueOrDefault(lookup, "CURSOR_KEY_ID", DevelopmentCursorKeyID),
			ActiveSecret:   valueOrDefault(lookup, "CURSOR_SECRET", DevelopmentCursorSecret),
			PreviousKeyID:  value(lookup, "CURSOR_PREVIOUS_KEY_ID"),
			PreviousSecret: value(lookup, "CURSOR_PREVIOUS_SECRET"),
		},
		DatabaseURL:           value(lookup, "DATABASE_URL"),
		RedisURL:              value(lookup, "REDIS_URL"),
		HTTPReadHeaderTimeout: 5 * time.Second,
		HTTPReadTimeout:       15 * time.Second,
		HTTPWriteTimeout:      30 * time.Second,
		HTTPIdleTimeout:       60 * time.Second,
		ShutdownTimeout:       10 * time.Second,
		DependencyTimeout:     3 * time.Second,
		MinIO: MinIO{
			Endpoint:  value(lookup, "MINIO_ENDPOINT"),
			AccessKey: value(lookup, "MINIO_ACCESS_KEY"),
			SecretKey: value(lookup, "MINIO_SECRET_KEY"),
			Bucket:    value(lookup, "MINIO_BUCKET_NAME"),
		},
	}

	var err error
	config.HTTPOrigins, err = httpx.ParseOriginPolicy(value(lookup, "HTTP_ALLOWED_ORIGINS"), config.Environment)
	if err != nil {
		problems = append(problems, fmt.Errorf("HTTP_ALLOWED_ORIGINS is invalid: %w", err))
	}
	config.TrustedProxies, err = httpx.ParseTrustedProxies(value(lookup, "HTTP_TRUSTED_PROXY_CIDRS"))
	if err != nil {
		problems = append(problems, fmt.Errorf("HTTP_TRUSTED_PROXY_CIDRS is invalid: %w", err))
	}

	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{{"HTTP_READ_HEADER_TIMEOUT", &config.HTTPReadHeaderTimeout}, {"HTTP_READ_TIMEOUT", &config.HTTPReadTimeout}, {"HTTP_WRITE_TIMEOUT", &config.HTTPWriteTimeout}, {"HTTP_IDLE_TIMEOUT", &config.HTTPIdleTimeout}, {"SHUTDOWN_TIMEOUT", &config.ShutdownTimeout}, {"DEPENDENCY_TIMEOUT", &config.DependencyTimeout}} {
		if raw, ok := lookup(setting.name); ok && strings.TrimSpace(raw) != "" {
			duration, err := time.ParseDuration(raw)
			if err != nil || duration <= 0 || duration > 5*time.Minute {
				problems = append(problems, fmt.Errorf("%s must be a duration between 0 and 5m", setting.name))
				continue
			}
			*setting.target = duration
		}
	}
	if raw, ok := lookup("RATE_LIMIT_TIMEOUT"); ok && strings.TrimSpace(raw) != "" {
		duration, parseErr := time.ParseDuration(raw)
		if parseErr != nil || duration <= 0 || duration > 5*time.Second {
			problems = append(problems, errors.New("RATE_LIMIT_TIMEOUT must be a duration between 0 and 5s"))
		} else {
			config.RateLimitTimeout = duration
		}
	}
	parseRatePolicy(lookup, "RATE_LIMIT_LOGIN_IP", &config.RateLimits.LoginIP, &problems)
	parseRatePolicy(lookup, "RATE_LIMIT_REGISTRATION_IP", &config.RateLimits.RegistrationIP, &problems)
	parseRatePolicy(lookup, "RATE_LIMIT_ACCOUNT_FAILURE", &config.RateLimits.AccountFailure, &problems)
	parseRatePolicy(lookup, "RATE_LIMIT_ADMIN", &config.RateLimits.Administrative, &problems)

	if raw, ok := lookup("MINIO_USE_SSL"); ok && strings.TrimSpace(raw) != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			problems = append(problems, errors.New("MINIO_USE_SSL must be a boolean"))
		} else {
			config.MinIO.UseSSL = parsed
		}
	}
	if raw, ok := lookup("PPROF_ENABLED"); ok && strings.TrimSpace(raw) != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			problems = append(problems, errors.New("PPROF_ENABLED must be a boolean"))
		} else {
			config.PprofEnabled = parsed
		}
	}

	for name, setting := range map[string]string{
		"DATABASE_URL":      config.DatabaseURL,
		"REDIS_URL":         config.RedisURL,
		"MINIO_ENDPOINT":    config.MinIO.Endpoint,
		"MINIO_ACCESS_KEY":  config.MinIO.AccessKey,
		"MINIO_SECRET_KEY":  config.MinIO.SecretKey,
		"MINIO_BUCKET_NAME": config.MinIO.Bucket,
	} {
		if strings.TrimSpace(setting) == "" {
			problems = append(problems, fmt.Errorf("%s is required", name))
		}
	}

	if config.Environment != "development" && config.Environment != "test" && config.Environment != "production" {
		problems = append(problems, errors.New("APP_ENV must be development, test, or production"))
	}
	if _, parseErr := redis.ParseURL(config.RedisURL); parseErr != nil {
		problems = append(problems, errors.New("REDIS_URL must be a valid redis or rediss URL"))
	}
	if (config.Cursor.PreviousKeyID == "") != (config.Cursor.PreviousSecret == "") {
		problems = append(problems, errors.New("CURSOR_PREVIOUS_KEY_ID and CURSOR_PREVIOUS_SECRET must be configured together"))
	}
	if config.Cursor.PreviousKeyID != "" && config.Cursor.PreviousKeyID == config.Cursor.ActiveKeyID {
		problems = append(problems, errors.New("CURSOR_PREVIOUS_KEY_ID must differ from CURSOR_KEY_ID"))
	}
	cursorCodecConfig := pagination.CodecConfig{Active: pagination.Key{
		ID: config.Cursor.ActiveKeyID, Secret: []byte(config.Cursor.ActiveSecret),
	}}
	if config.Cursor.PreviousKeyID != "" && config.Cursor.PreviousSecret != "" {
		cursorCodecConfig.Previous = &pagination.Key{
			ID: config.Cursor.PreviousKeyID, Secret: []byte(config.Cursor.PreviousSecret),
		}
	}
	if _, err := pagination.NewCodec(cursorCodecConfig); err != nil {
		problems = append(problems, errors.New("cursor key IDs and secrets are invalid"))
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(problems...))
	}
	return config, nil
}

func parseRatePolicy(lookup lookupFunc, prefix string, policy *ratelimit.Policy, problems *[]error) {
	if raw, ok := lookup(prefix + "_LIMIT"); ok && strings.TrimSpace(raw) != "" {
		parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || parsed < 1 || parsed > 10_000 {
			*problems = append(*problems, fmt.Errorf("%s_LIMIT must be between 1 and 10000", prefix))
		} else {
			policy.Limit = parsed
		}
	}
	if raw, ok := lookup(prefix + "_WINDOW"); ok && strings.TrimSpace(raw) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil || parsed < time.Second || parsed > time.Hour {
			*problems = append(*problems, fmt.Errorf("%s_WINDOW must be between 1s and 1h", prefix))
		} else {
			policy.Period = parsed
		}
	}
	if raw, ok := lookup(prefix + "_BURST"); ok && strings.TrimSpace(raw) != "" {
		parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || parsed < 1 || parsed > 10_000 {
			*problems = append(*problems, fmt.Errorf("%s_BURST must be between 1 and 10000", prefix))
		} else {
			policy.Burst = parsed
		}
	}
	if err := policy.Validate(); err != nil {
		*problems = append(*problems, fmt.Errorf("%s policy is invalid", prefix))
	}
}

func value(lookup lookupFunc, name string) string {
	value, _ := lookup(name)
	return strings.TrimSpace(value)
}

func valueOrDefault(lookup lookupFunc, name, fallback string) string {
	if result := value(lookup, name); result != "" {
		return result
	}
	return fallback
}
