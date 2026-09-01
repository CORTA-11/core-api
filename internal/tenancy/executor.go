package tenancy

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
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

type GeneratedQueriesCallback func(*publicdb.Queries, *tenantdb.Queries) error

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
	return e.within(ctx, organization, 0, false, func(_ *publicdb.Queries, tenantQueries *tenantdb.Queries) error {
		return callback(tenantQueries)
	})
}

// WithinOrganizationQueries exposes only generated public and tenant queries
// in one organization-scoped transaction. Raw transactions, internal IDs, and
// schema names remain executor-owned.
func (e *Executor) WithinOrganizationQueries(
	ctx context.Context,
	organization OrganizationContext,
	callback GeneratedQueriesCallback,
) error {
	if err := organization.validate(); err != nil || e == nil || e.pool == nil {
		return ErrInvalidContext
	}
	if callback == nil {
		return ErrInvalidCallback
	}
	return e.within(ctx, organization, 0, false, callback)
}

// WithinTeam executes callback in a team-scoped transaction with the resolved
// internal team identity installed transaction-locally for later RLS policies.
func (e *Executor) WithinTeam(
	ctx context.Context,
	team TeamContext,
	callback func(*tenantdb.Queries) error,
) error {
	if err := team.validate(); err != nil || e == nil || e.pool == nil {
		return ErrInvalidContext
	}
	if callback == nil {
		return ErrInvalidCallback
	}
	return e.within(ctx, team.organization, team.teamID, true, func(_ *publicdb.Queries, tenantQueries *tenantdb.Queries) error {
		return callback(tenantQueries)
	})
}

// WithinTeamQueries is the team-scoped generated-query variant used when an
// authorization decision must be re-read in the mutating transaction.
func (e *Executor) WithinTeamQueries(
	ctx context.Context,
	team TeamContext,
	callback GeneratedQueriesCallback,
) error {
	if err := team.validate(); err != nil || e == nil || e.pool == nil {
		return ErrInvalidContext
	}
	if callback == nil {
		return ErrInvalidCallback
	}
	return e.within(ctx, team.organization, team.teamID, true, callback)
}

// within handles the within operation.
func (e *Executor) within(
	ctx context.Context,
	organization OrganizationContext,
	teamID int64,
	teamScoped bool,
	callback GeneratedQueriesCallback,
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
	publicQueries := publicdb.New(tx)
	if err := revalidateOrganization(ctx, publicQueries, organization); err != nil {
		return err
	}
	if err := callback(publicQueries, tenantdb.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tenant transaction: %w", err)
	}
	finished = true
	return nil
}

// revalidateOrganization revalidates organization.
func revalidateOrganization(ctx context.Context, queries *publicdb.Queries, organization OrganizationContext) error {
	row, err := queries.ResolveOrganizationContext(ctx, publicdb.ResolveOrganizationContextParams{
		UserPublicID:         organization.userPublicID,
		OrganizationPublicID: organization.organizationPublicID,
	})
	if err != nil {
		return ErrOrganizationUnavailable
	}
	if row.OrganizationID != organization.organizationID || row.SchemaName != organization.schemaName ||
		row.LifecycleState != string(StateActive) || row.TenantVersion != organization.tenantVersion ||
		strings.TrimSpace(row.TenantChecksum) != organization.tenantChecksum || !row.SchemaExists {
		return ErrOrganizationUnavailable
	}
	return nil
}

// installTenantScope installs tenant scope.
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
		searchPath, organization.userPublicID.String(), teamSetting); err != nil {
		return fmt.Errorf("install tenant transaction scope: %w", err)
	}
	return nil
}

// rollbackDetached rollbacks detached.
func rollbackDetached(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), rollbackTimeout)
	defer cancel()
	// pgx closes the underlying connection when a real transaction rollback
	// fails. pgxpool therefore discards it instead of returning uncertain session
	// or transaction state to a subsequent borrower.
	_ = tx.Rollback(ctx)
}
