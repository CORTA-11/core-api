// Command provisioner reconciles organization schemas with the tenant
// migrations embedded in the running binary.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

var errFleetNotCurrent = errors.New("requested tenant fleet is not current")

func main() { os.Exit(realMain()) }

func realMain() int {
	_ = godotenv.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	result := make(chan error, 1)
	go func() { result <- execute(ctx, os.Args[1:], os.LookupEnv, os.Stdout) }()
	var err error
	// Cancellation first asks database work to stop, then bounds how long the
	// process waits for advisory-lock cleanup and failure-state persistence.
	select {
	case err = <-result:
	case <-ctx.Done():
		shutdownTimeout := 10 * time.Second
		if cfg, configErr := tenancy.LoadConfig(os.LookupEnv); configErr == nil {
			shutdownTimeout = cfg.ShutdownTimeout
		}
		timer := time.NewTimer(shutdownTimeout)
		defer timer.Stop()
		select {
		case err = <-result:
		case <-timer.C:
			slog.Error("provisioner shutdown timed out")
			return 1
		}
	}
	if err != nil {
		// Command output contains bounded, sanitized detail; logs deliberately do
		// not render driver errors or connection strings.
		slog.Error("provisioner command failed")
		return 1
	}
	return 0
}

func execute(ctx context.Context, args []string, lookup tenancy.LookupFunc, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: provisioner run|reconcile|status|retry")
	}
	cfg, err := tenancy.LoadConfig(lookup)
	if err != nil {
		return err
	}
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return errors.New("DATABASE_URL is invalid")
	}
	// Workers may each hold a connection-scoped advisory lock. Two spare
	// connections leave room for fleet coordination and detached failure writes.
	// #nosec G115 -- configuration validation bounds concurrency to 1..16.
	poolConfig.MaxConns = int32(cfg.Concurrency + 2)
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return errors.New("could not open the provisioner database")
	}
	defer pool.Close()
	source, err := tenancy.EmbeddedMigrations()
	if err != nil {
		return err
	}
	reconciler := tenancy.NewReconciler(pool, source, cfg)
	encoder := json.NewEncoder(output)
	// One JSON object per line lets operators retain every tenant outcome even
	// when a later result makes the overall command exit nonzero.
	emit := func(result tenancy.Result) { _ = encoder.Encode(result) }

	switch args[0] {
	case "run":
		if len(args) != 1 {
			return errors.New("usage: provisioner run")
		}
		return reconciler.Run(ctx, emit)
	case "reconcile":
		id, all, concurrency, err := parseSelection("reconcile", args[1:], cfg.Concurrency, true)
		if err != nil {
			return err
		}
		var ids []uuid.UUID
		if all {
			// Fleet reconciliation requires the explicit --all flag; an omitted
			// selector must not accidentally migrate every organization.
			ids, err = reconciler.AllOrganizationIDs(ctx)
		} else {
			ids = []uuid.UUID{*id}
		}
		if err != nil {
			return err
		}
		failed := false
		for result := range reconciler.ReconcileMany(ctx, ids, concurrency) {
			emit(result)
			failed = failed || result.State != tenancy.StateActive
		}
		if failed {
			return errFleetNotCurrent
		}
		return nil
	case "status":
		id, _, _, err := parseSelection("status", args[1:], cfg.Concurrency, false)
		if err != nil {
			return err
		}
		statuses, err := reconciler.Status(ctx, id)
		if err != nil {
			return err
		}
		if id != nil && len(statuses) == 0 {
			return errors.New("organization was not found")
		}
		current := true
		for _, status := range statuses {
			_ = encoder.Encode(status)
			current = current && status.Current
		}
		if !current {
			return errFleetNotCurrent
		}
		return nil
	case "retry":
		id, _, _, err := parseSelection("retry", args[1:], cfg.Concurrency, false)
		if err != nil {
			return err
		}
		count, err := reconciler.Retry(ctx, id)
		if err != nil {
			return err
		}
		if id != nil && count == 0 {
			return errors.New("organization is not in terminal failure")
		}
		statuses, err := reconciler.Status(ctx, id)
		if err != nil {
			return err
		}
		for _, status := range statuses {
			_ = encoder.Encode(status)
		}
		return nil
	default:
		return fmt.Errorf("unknown provisioner command %q", args[0])
	}
}

func parseSelection(command string, args []string, defaultConcurrency int, allowConcurrency bool) (*uuid.UUID, bool, int, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	organization := flags.String("organization", "", "public organization UUID")
	all := flags.Bool("all", false, "select all organizations")
	concurrency := flags.Int("concurrency", defaultConcurrency, "bounded worker concurrency")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return nil, false, 0, fmt.Errorf("invalid %s arguments", command)
	}
	if !allowConcurrency && *concurrency != defaultConcurrency {
		return nil, false, 0, fmt.Errorf("%s does not accept --concurrency", command)
	}
	if err := tenancy.ValidateConcurrency(*concurrency); err != nil {
		return nil, false, 0, err
	}
	if *organization != "" && *all {
		return nil, false, 0, errors.New("--organization and --all are mutually exclusive")
	}
	if command == "reconcile" && *organization == "" && !*all {
		return nil, false, 0, errors.New("reconcile requires --organization or --all")
	}
	if *organization == "" {
		return nil, true, *concurrency, nil
	}
	// Commands accept only the public identity. Raw schema names would turn an
	// operational interface into an avoidable tenant-boundary trust input.
	publicID, err := uuid.Parse(*organization)
	if err != nil {
		return nil, false, 0, errors.New("--organization must be a public UUID")
	}
	return &publicID, false, *concurrency, nil
}
