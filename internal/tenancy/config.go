package tenancy

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultConcurrency = 4
	MaxConcurrency     = 16
	DefaultMaxAttempts = 5
)

type Config struct {
	DatabaseURL      string
	PollInterval     time.Duration
	RetryInitial     time.Duration
	RetryMaximum     time.Duration
	MaxAttempts      int
	Concurrency      int
	OperationTimeout time.Duration
	ShutdownTimeout  time.Duration
}

type LookupFunc func(string) (string, bool)

func LoadConfig(lookup LookupFunc) (Config, error) {
	cfg := Config{
		DatabaseURL: strings.TrimSpace(value(lookup, "DATABASE_URL")), PollInterval: 5 * time.Second,
		RetryInitial: 5 * time.Second, RetryMaximum: 5 * time.Minute, MaxAttempts: DefaultMaxAttempts,
		Concurrency: DefaultConcurrency, OperationTimeout: 2 * time.Minute, ShutdownTimeout: 10 * time.Second,
	}
	var problems []error
	if cfg.DatabaseURL == "" {
		problems = append(problems, errors.New("DATABASE_URL is required"))
	}
	for _, item := range []struct {
		name string
		dst  *time.Duration
		max  time.Duration
	}{
		{"PROVISIONER_POLL_INTERVAL", &cfg.PollInterval, time.Minute},
		{"PROVISIONER_RETRY_INITIAL", &cfg.RetryInitial, 5 * time.Minute},
		{"PROVISIONER_RETRY_MAXIMUM", &cfg.RetryMaximum, time.Hour},
		{"PROVISIONER_OPERATION_TIMEOUT", &cfg.OperationTimeout, time.Hour},
		{"PROVISIONER_SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout, 5 * time.Minute},
	} {
		if raw := strings.TrimSpace(value(lookup, item.name)); raw != "" {
			duration, err := time.ParseDuration(raw)
			if err != nil || duration <= 0 || duration > item.max {
				problems = append(problems, fmt.Errorf("%s must be a positive duration no greater than %s", item.name, item.max))
			} else {
				*item.dst = duration
			}
		}
	}
	for _, item := range []struct {
		name     string
		dst      *int
		min, max int
	}{
		{"PROVISIONER_CONCURRENCY", &cfg.Concurrency, 1, MaxConcurrency},
		{"PROVISIONER_MAX_ATTEMPTS", &cfg.MaxAttempts, 1, 100},
	} {
		if raw := strings.TrimSpace(value(lookup, item.name)); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < item.min || n > item.max {
				problems = append(problems, fmt.Errorf("%s must be between %d and %d", item.name, item.min, item.max))
			} else {
				*item.dst = n
			}
		}
	}
	if cfg.RetryMaximum < cfg.RetryInitial {
		problems = append(problems, errors.New("PROVISIONER_RETRY_MAXIMUM must not be less than PROVISIONER_RETRY_INITIAL"))
	}
	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid provisioner configuration: %w", errors.Join(problems...))
	}
	return cfg, nil
}

func value(lookup LookupFunc, name string) string {
	result, _ := lookup(name)
	return result
}

func ValidateConcurrency(value int) error {
	if value < 1 || value > MaxConcurrency {
		return fmt.Errorf("concurrency must be between 1 and %d", MaxConcurrency)
	}
	return nil
}

func retryDelay(initial, maximum time.Duration, attempts int) time.Duration {
	delay := initial
	for i := 1; i < attempts && delay < maximum; i++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}
