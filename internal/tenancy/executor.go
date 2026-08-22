package tenancy

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rollbackTimeout = 2 * time.Second

var ErrInvalidCallback = errors.New("tenant callback is invalid")

// Executor is the sole owner of tenant transaction setup and cleanup. Callback
// code receives generated tenant queries, never a transaction or connection.
type Executor struct {
	pool *pgxpool.Pool
}

// NewExecutor constructs a tenant executor over pool.
func NewExecutor(pool *pgxpool.Pool) *Executor {
	return &Executor{pool: pool}
}

// WithinOrganization executes callback in an organization-scoped transaction.
// Organization scope always clears app.team_id explicitly.
func (e *Executor) WithinOrganization(
	ctx context.Context,
	organization OrganizationContext,
	callback func(*tenantdb.Queries) error,
) error {
	if err := organization.validate(); err != nil || e == nil || e.pool == nil {
		return ErrInvalidContext
	}
	if callback == nil {
		return ErrInvalidCallback
	}
	return e.within(ctx, organization, 0, false, callback)
}

func (e *Executor) within(
	ctx context.Context,
	organization OrganizationContext,
	teamID int64,
	teamScoped bool,
	callback func(*tenantdb.Queries) error,
) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tenant transaction: %w", err)
	}
	finished := false
	defer func() {
		if !finished {
			// This defer also runs during panic unwinding. It intentionally does
			// not recover, so the original panic value is preserved after cleanup.
			rollbackDetached(tx)
		}
	}()

	if err := installTenantScope(ctx, tx, organization, teamID, teamScoped); err != nil {
		return err
	}
	queries := tenantdb.New(tx)
	if err := callback(queries); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tenant transaction: %w", err)
	}
	finished = true
	return nil
}

func installTenantScope(
	ctx context.Context,
	tx pgx.Tx,
	organization OrganizationContext,
	teamID int64,
	teamScoped bool,
) error {
	var schemaExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_catalog.pg_namespace
			WHERE nspname = $1
		)`, organization.schemaName).Scan(&schemaExists); err != nil {
		return fmt.Errorf("verify tenant namespace: %w", err)
	}
	if !schemaExists {
		return ErrOrganizationUnavailable
	}

	searchPath := pgx.Identifier{"pg_catalog"}.Sanitize() + ", " + pgx.Identifier{organization.schemaName}.Sanitize()
	teamSetting := ""
	if teamScoped {
		teamSetting = strconv.FormatInt(teamID, 10)
	}
	if _, err := tx.Exec(ctx, `
		SELECT set_config('search_path', $1, true),
		       set_config('app.user_id', $2, true),
		       set_config('app.team_id', $3, true)`,
		searchPath, strconv.FormatInt(organization.userID, 10), teamSetting); err != nil {
		return fmt.Errorf("install tenant transaction scope: %w", err)
	}
	return nil
}

func rollbackDetached(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), rollbackTimeout)
	defer cancel()
	// pgx closes the underlying connection when a real transaction rollback
	// fails. pgxpool therefore discards it instead of returning uncertain session
	// or transaction state to a subsequent borrower.
	_ = tx.Rollback(ctx)
}
