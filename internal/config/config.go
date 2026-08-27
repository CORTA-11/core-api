package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DevelopmentJWTSecret  = "development-only-jwt-secret-change-me"
	DevelopmentCSRFSecret = "development-only-csrf-secret-change-me"
)

type Config struct {
	Environment       string
	HTTPAddr          string
	HTTPReadTimeout   time.Duration
	HTTPWriteTimeout  time.Duration
	HTTPIdleTimeout   time.Duration
	ShutdownTimeout   time.Duration
	DependencyTimeout time.Duration
	DatabaseURL       string
	RedisURL          string
	MinIO             MinIO
	JWTSecret         string
	CSRFSecret        string
	PprofEnabled      bool
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
	return LoadFrom(os.LookupEnv)
}

func LoadFrom(lookup lookupFunc) (Config, error) {
	config := Config{
		Environment:       valueOrDefault(lookup, "APP_ENV", "development"),
		HTTPAddr:          valueOrDefault(lookup, "HTTP_ADDR", ":8080"),
		JWTSecret:         valueOrDefault(lookup, "JWT_SECRET", DevelopmentJWTSecret),
		CSRFSecret:        valueOrDefault(lookup, "CSRF_SECRET", DevelopmentCSRFSecret),
		DatabaseURL:       value(lookup, "DATABASE_URL"),
		RedisURL:          value(lookup, "REDIS_URL"),
		HTTPReadTimeout:   15 * time.Second,
		HTTPWriteTimeout:  15 * time.Second,
		HTTPIdleTimeout:   60 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		DependencyTimeout: 3 * time.Second,
		MinIO: MinIO{
			Endpoint:  value(lookup, "MINIO_ENDPOINT"),
			AccessKey: value(lookup, "MINIO_ACCESS_KEY"),
			SecretKey: value(lookup, "MINIO_SECRET_KEY"),
			Bucket:    value(lookup, "MINIO_BUCKET_NAME"),
		},
	}

	var problems []error
	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{{"HTTP_READ_TIMEOUT", &config.HTTPReadTimeout}, {"HTTP_WRITE_TIMEOUT", &config.HTTPWriteTimeout}, {"HTTP_IDLE_TIMEOUT", &config.HTTPIdleTimeout}, {"SHUTDOWN_TIMEOUT", &config.ShutdownTimeout}, {"DEPENDENCY_TIMEOUT", &config.DependencyTimeout}} {
		if raw, ok := lookup(setting.name); ok && strings.TrimSpace(raw) != "" {
			duration, err := time.ParseDuration(raw)
			if err != nil || duration <= 0 || duration > 5*time.Minute {
				problems = append(problems, fmt.Errorf("%s must be a duration between 0 and 5m", setting.name))
				continue
			}
			*setting.target = duration
		}
	}

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
	if config.Environment == "production" {
		if len(config.JWTSecret) < 32 || isDevelopmentSecret(config.JWTSecret) {
			problems = append(problems, errors.New("JWT_SECRET must be a non-development value of at least 32 characters in production"))
		}
		if len([]byte(config.CSRFSecret)) < 32 || isDevelopmentSecret(config.CSRFSecret) ||
			config.CSRFSecret == config.JWTSecret || config.CSRFSecret == databasePassword(config.DatabaseURL) {
			problems = append(problems, errors.New("CSRF_SECRET must be a distinct non-development value of at least 32 bytes in production"))
		}
		if config.PprofEnabled {
			problems = append(problems, errors.New("PPROF_ENABLED cannot be enabled in production"))
		}
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(problems...))
	}
	return config, nil
}

func databasePassword(databaseURL string) string {
	parsed, err := url.Parse(databaseURL)
	if err != nil || parsed.User == nil {
		return ""
	}
	password, _ := parsed.User.Password()
	return password
}

func isDevelopmentSecret(secret string) bool {
	normalized := strings.ToLower(secret)
	return secret == DevelopmentJWTSecret ||
		secret == "your-super-secret-key-change-in-production" ||
		strings.Contains(normalized, "change-me")
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
