package tenancy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Reconciler advances organization schemas toward an embedded MigrationSet and
// persists enough state for retries to resume after process failure.
type Reconciler struct {
	pool   *pgxpool.Pool
	source MigrationSet
	config Config
}

// NewReconciler constructs a reconciler over the public registry and tenant
// schemas available through pool.
func NewReconciler(pool *pgxpool.Pool, source MigrationSet, config Config) *Reconciler {
	return &Reconciler{pool: pool, source: source, config: config}
}

// Reconcile brings one organization to the current migration set. Expected
// operational and permanent failures are returned as a sanitized Result rather
// than an error so fleet callers can continue processing other organizations.
func (r *Reconciler) Reconcile(ctx context.Context, publicID uuid.UUID) Result {
	if r.config.OperationTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.config.OperationTimeout)
		defer cancel()
	}
	result := Result{OrganizationID: publicID}
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return r.failureResult(ctx, result, 0, err)
	}
	defer conn.Release()

	org, err := loadOrganization(ctx, conn, publicID)
	if err != nil {
		return r.failureResult(ctx, result, 0, err)
	}
	result.Attempts = org.ReconcileAttempts
	if org.LifecycleState == StateDeleting {
		result.State, result.ErrorCode, result.ErrorDetail = StateDeleting, "organization_deleting", "organization is deleting"
		return result
	}
	if org.SchemaName != CanonicalSchema(org.PublicID.String()) {
		return r.failureResult(ctx, result, org.ReconcileAttempts, permanent("noncanonical_schema", "organization registry has a noncanonical schema"))
	}

	// The session-level advisory lock is held on this dedicated connection. It
	// serializes manual commands and competing provisioner processes for one org.
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, org.ID); err != nil {
		return r.failureResult(ctx, result, org.ReconcileAttempts, err)
	}
	defer func() {
		// Unlock must not inherit a canceled operation context or leave a pooled
		// session holding the lock indefinitely.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, org.ID)
	}()

	// The lock may have blocked behind another reconciler, so decisions must use
	// registry state reloaded after ownership is acquired.
	org, err = loadOrganization(ctx, conn, publicID)
	if err != nil {
		return r.failureResult(ctx, result, org.ReconcileAttempts, err)
	}
	if org.LifecycleState == StateDeleting {
		result.State, result.ErrorCode, result.ErrorDetail = StateDeleting, "organization_deleting", "organization is deleting"
		return result
	}
	alreadyCurrent := org.LifecycleState == StateActive && org.TenantVersion == r.source.Version && org.TenantChecksum == r.source.Checksum
	if alreadyCurrent {
		// Registry metadata alone cannot prove that the tenant ledger or verified
		// base catalog was not altered out of band.
		if err := r.prepareAndValidateLedger(ctx, conn, org); err != nil {
			return r.failureResult(ctx, result, org.ReconcileAttempts, err)
		}
		result.State, result.Version, result.Checksum = StateActive, r.source.Version, r.source.Checksum
		return result
	}

	attempts, err := beginAttempt(ctx, conn, org.ID, r.config.OperationTimeout)
	if err != nil {
		return r.failureResult(ctx, result, org.ReconcileAttempts, err)
	}
	result.Attempts = attempts

	if err := r.prepareAndValidateLedger(ctx, conn, org); err != nil {
		return r.failureResult(ctx, result, attempts, err)
	}
	if err := r.applyMissing(ctx, conn, org); err != nil {
		return r.failureResult(ctx, result, attempts, err)
	}
	command, err := conn.Exec(ctx, `
        UPDATE public.orgs
        SET lifecycle_state = 'active', tenant_version = $2, tenant_checksum = $3,
            last_error_code = NULL, last_error_detail = NULL, next_attempt_at = now(),
            provisioned_at = COALESCE(provisioned_at, now()), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL AND lifecycle_state = 'provisioning'`, org.ID, r.source.Version, r.source.Checksum)
	if err != nil {
		return r.failureResult(ctx, result, attempts, err)
	}
	if command.RowsAffected() != 1 {
		result.State, result.ErrorCode, result.ErrorDetail = StateDeleting, "organization_unavailable", "organization became unavailable during reconciliation"
		return result
	}
	result.State, result.Version, result.Checksum = StateActive, r.source.Version, r.source.Checksum
	return result
}

// loadOrganization loads organization.
func loadOrganization(ctx context.Context, conn *pgxpool.Conn, publicID uuid.UUID) (Organization, error) {
	var org Organization
	err := conn.QueryRow(ctx, `
        SELECT id, public_id, schema_name, lifecycle_state, tenant_version,
               tenant_checksum, reconcile_attempts, next_attempt_at
        FROM public.orgs WHERE public_id = $1`, publicID).Scan(
		&org.ID, &org.PublicID, &org.SchemaName, &org.LifecycleState, &org.TenantVersion,
		&org.TenantChecksum, &org.ReconcileAttempts, &org.NextAttemptAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, permanent("organization_not_found", "organization was not found")
	}
	if err != nil {
		return Organization{}, fmt.Errorf("load organization registry record: %w", err)
	}
	return org, nil
}

// beginAttempt begins attempt.
func beginAttempt(ctx context.Context, conn *pgxpool.Conn, id int64, timeout time.Duration) (int, error) {
	var attempts int
	// next_attempt_at doubles as a bounded claim lease. A crashed process becomes
	// eligible again after the maximum duration of one reconciliation attempt.
	err := conn.QueryRow(ctx, `
        UPDATE public.orgs
        SET lifecycle_state = 'provisioning', reconcile_attempts = reconcile_attempts + 1,
            last_attempt_at = now(), next_attempt_at = now() + $2::interval, updated_at = now()
        WHERE id = $1 AND deleted_at IS NULL AND lifecycle_state <> 'deleting'
        RETURNING reconcile_attempts`, id, timeout.String()).Scan(&attempts)
	if err != nil {
		return 0, fmt.Errorf("record reconciliation attempt: %w", err)
	}
	return attempts, nil
}

// prepareAndValidateLedger prepares and validate ledger.
func (r *Reconciler) prepareAndValidateLedger(ctx context.Context, conn *pgxpool.Conn, org Organization) error {
	// Identifier.Sanitize quotes the server-derived name before it is used in DDL;
	// query parameters cannot represent PostgreSQL identifiers.
	identifier := pgx.Identifier{org.SchemaName}.Sanitize()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("start tenant preparation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+identifier); err != nil {
		return fmt.Errorf("allocate tenant schema: %w", err)
	}
	if _, err := tx.Exec(ctx, `REVOKE ALL ON SCHEMA `+identifier+` FROM PUBLIC`); err != nil {
		return fmt.Errorf("restrict tenant schema: %w", err)
	}
	if err := setTenantSearchPath(ctx, tx, org.SchemaName); err != nil {
		return err
	}

	columns, err := ledgerColumns(ctx, tx, org.SchemaName)
	if err != nil {
		return err
	}
	adoptedLegacy := false
	// Only an empty schema, the exact legacy golang-migrate ledger, or the current
	// checksum ledger is recognized. Unknown layouts fail closed rather than
	// guessing which migrations are safe to replay.
	switch {
	case len(columns) == 0:
		var tableCount int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_type = 'BASE TABLE'`, org.SchemaName).Scan(&tableCount); err != nil {
			return fmt.Errorf("inspect tenant schema: %w", err)
		}
		if tableCount != 0 {
			return permanent("catalog_divergence", "tenant schema has objects but no recognized migration ledger")
		}
		if err := createLedger(ctx, tx); err != nil {
			return err
		}
	case columns["dirty"] && columns["version"] && len(columns) == 2:
		if err := r.adoptLegacy(ctx, tx, org.SchemaName); err != nil {
			return err
		}
		adoptedLegacy = true
	case columns["version"] && columns["checksum"] && columns["applied_at"] && len(columns) == 3:
		if err := r.validateLedger(ctx, tx); err != nil {
			return err
		}
	default:
		return permanent("ledger_divergence", "tenant migration ledger has an unknown format")
	}
	var ledgerVersion int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version), 0) FROM schema_migrations`).Scan(&ledgerVersion); err != nil {
		return fmt.Errorf("read tenant ledger version: %w", err)
	}
	if adoptedLegacy {
		command, err := tx.Exec(ctx, `
			UPDATE public.orgs SET tenant_version = $2, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL AND lifecycle_state = 'provisioning'`, org.ID, ledgerVersion)
		if err != nil {
			return fmt.Errorf("record adopted tenant registry version: %w", err)
		}
		if command.RowsAffected() != 1 {
			return permanent("organization_unavailable", "organization became unavailable during reconciliation")
		}
	} else if ledgerVersion != org.TenantVersion {
		return permanent("registry_ledger_divergence", "tenant registry version differs from its migration ledger")
	}
	if err := validateBaseCatalog(ctx, tx, org.SchemaName, ledgerVersion); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tenant preparation: %w", err)
	}
	return nil
}

// setTenantSearchPath sets tenant search path.
func setTenantSearchPath(ctx context.Context, tx pgx.Tx, schema string) error {
	// The transaction-local setting prevents a pooled connection from leaking one
	// organization's schema into a later borrower.
	if _, err := tx.Exec(ctx, `SELECT set_config('search_path', $1, true)`, schema); err != nil {
		return fmt.Errorf("set tenant migration scope: %w", err)
	}
	return nil
}

// ledgerColumns returns the migration ledger columns.
func ledgerColumns(ctx context.Context, tx pgx.Tx, schema string) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `
        SELECT column_name FROM information_schema.columns
        WHERE table_schema = $1 AND table_name = 'schema_migrations'`, schema)
	if err != nil {
		return nil, fmt.Errorf("inspect tenant migration ledger: %w", err)
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("read tenant migration ledger catalog: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tenant migration ledger catalog: %w", err)
	}
	return columns, nil
}

// createLedger creates ledger.
func createLedger(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `CREATE TABLE schema_migrations (
        version BIGINT PRIMARY KEY,
        checksum CHAR(64) NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
        applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
    )`)
	if err != nil {
		return fmt.Errorf("create tenant migration ledger: %w", err)
	}
	return nil
}

// adoptLegacy adopts legacy.
func (r *Reconciler) adoptLegacy(ctx context.Context, tx pgx.Tx, schema string) error {
	var version int64
	var dirty bool
	if err := tx.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		return permanent("legacy_ledger_invalid", "legacy tenant migration ledger is empty or invalid")
	}
	if dirty {
		return permanent("legacy_migration_dirty", "legacy tenant migration ledger is dirty")
	}
	const baseCatalogVersion = int64(2)
	if version != baseCatalogVersion || r.source.Version < baseCatalogVersion {
		return permanent("legacy_version_divergence", "legacy tenant schema is partial, unknown, or ahead of source")
	}
	// The old ledger has no checksums, so adoption is permitted only for the one
	// known base version after its concrete catalog shape has been verified.
	if err := validateBaseCatalog(ctx, tx, schema, version); err != nil {
		return permanent("legacy_catalog_divergence", "legacy tenant catalog does not match the verified base layout")
	}
	if _, err := tx.Exec(ctx, `DROP TABLE schema_migrations`); err != nil {
		return fmt.Errorf("replace legacy migration ledger: %w", err)
	}
	if err := createLedger(ctx, tx); err != nil {
		return err
	}
	for _, migration := range r.source.Migrations {
		if migration.Version > version {
			break
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`, migration.Version, migration.Checksum); err != nil {
			return fmt.Errorf("record adopted tenant migration: %w", err)
		}
	}
	return nil
}

// validateBaseCatalog validates base catalog.
func validateBaseCatalog(ctx context.Context, tx pgx.Tx, schema string, version int64) error {
	// This is deliberately a compatibility check for the fixed base catalog. For
	// later migrations, the ledger proves migration-source identity; it does not
	// claim to detect arbitrary out-of-band DDL changes.
	if version < 0 {
		return permanent("catalog_divergence", "tenant catalog version is outside the verified migration set")
	}
	wantTables := int(version)
	if version > 2 {
		wantTables = 2
	}
	var tableCount int
	if err := tx.QueryRow(ctx, `
        SELECT count(*) FROM information_schema.tables
        WHERE table_schema = $1 AND table_type = 'BASE TABLE' AND table_name <> 'schema_migrations'`, schema).Scan(&tableCount); err != nil {
		return fmt.Errorf("inspect tenant catalog tables: %w", err)
	}
	if (version <= 2 && tableCount != wantTables) || (version > 2 && tableCount < wantTables) {
		return permanent("catalog_divergence", "tenant catalog tables differ from the migration ledger")
	}
	if version == 0 {
		return nil
	}
	wantColumns := 5
	if version >= 2 {
		wantColumns = 11
	}
	var matchingColumns int
	if err := tx.QueryRow(ctx, `
        SELECT count(*)
        FROM information_schema.columns
        WHERE table_schema = $1
          AND (table_name, column_name, data_type) IN (
            ('teams', 'id', 'bigint'),
            ('teams', 'name', 'character varying'),
            ('teams', 'slug', 'character varying'),
            ('teams', 'created_at', 'timestamp with time zone'),
            ('teams', 'updated_at', 'timestamp with time zone'),
            ('tasks', 'id', 'bigint'),
            ('tasks', 'team_id', 'bigint'),
            ('tasks', 'description', 'text'),
            ('tasks', 'status', 'text'),
            ('tasks', 'created_at', 'timestamp with time zone'),
            ('tasks', 'updated_at', 'timestamp with time zone')
          )
          AND ($2::bigint >= 2 OR table_name = 'teams')`, schema, version).Scan(&matchingColumns); err != nil {
		return fmt.Errorf("inspect tenant catalog columns: %w", err)
	}
	if matchingColumns != wantColumns {
		return permanent("catalog_divergence", "tenant catalog columns differ from the migration ledger")
	}
	return nil
}

// validateLedger validates ledger.
func (r *Reconciler) validateLedger(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT version, trim(checksum) FROM schema_migrations ORDER BY version`)
	if err != nil {
		return fmt.Errorf("read tenant migration ledger: %w", err)
	}
	defer rows.Close()
	// A tenant ledger must be an exact prefix of the contiguous embedded source;
	// gaps, unknown versions, and rewritten migrations are permanent divergence.
	index := 0
	for rows.Next() {
		var version int64
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return fmt.Errorf("read tenant migration entry: %w", err)
		}
		if index >= len(r.source.Migrations) || r.source.Migrations[index].Version != version {
			return permanent("migration_version_divergence", "tenant migration ledger is unknown, has gaps, or is ahead of source")
		}
		if r.source.Migrations[index].Checksum != checksum {
			return permanent("migration_checksum_divergence", "tenant migration checksum differs from embedded source")
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read tenant migration entries: %w", err)
	}
	return nil
}

// applyMissing applies missing.
func (r *Reconciler) applyMissing(ctx context.Context, conn *pgxpool.Conn, org Organization) error {
	var current int64
	if err := conn.QueryRow(ctx, `SELECT tenant_version FROM public.orgs WHERE id = $1`, org.ID).Scan(&current); err != nil {
		return fmt.Errorf("read tenant registry version: %w", err)
	}
	for _, migration := range r.source.Migrations {
		if migration.Version <= current {
			continue
		}
		// Commit the schema change, tenant ledger entry, and public registry version
		// together. A retry therefore starts after the last fully committed version.
		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("start tenant migration: %w", err)
		}
		if err := setTenantSearchPath(ctx, tx, org.SchemaName); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply tenant migration version %d: %w", migration.Version, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`, migration.Version, migration.Checksum); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record tenant migration version %d: %w", migration.Version, err)
		}
		command, err := tx.Exec(ctx, `
			UPDATE public.orgs SET tenant_version = $2, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL AND lifecycle_state = 'provisioning'`, org.ID, migration.Version)
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record tenant registry version: %w", err)
		}
		if command.RowsAffected() != 1 {
			_ = tx.Rollback(ctx)
			return permanent("organization_unavailable", "organization became unavailable during reconciliation")
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit tenant migration version %d: %w", migration.Version, err)
		}
		current = migration.Version
	}
	return nil
}

// failureResult failures result.
func (r *Reconciler) failureResult(ctx context.Context, result Result, attempts int, reconcileErr error) Result {
	code, detail, isPermanent := errorFields(reconcileErr)
	result.State, result.Attempts, result.ErrorCode, result.ErrorDetail = StateProvisioning, attempts, code, detail
	if errors.Is(reconcileErr, context.Canceled) || result.OrganizationID == uuid.Nil {
		return result
	}
	state := StateProvisioning
	if isPermanent || attempts >= r.config.MaxAttempts {
		state = StateFailed
	}
	delay := retryDelay(r.config.RetryInitial, r.config.RetryMaximum, attempts)
	// Failure state must survive an expired operation context. This detached,
	// bounded write is best-effort and never overrides a concurrent delete.
	markCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = r.pool.Exec(markCtx, `
        UPDATE public.orgs
        SET lifecycle_state = $2, last_error_code = $3, last_error_detail = $4,
            next_attempt_at = now() + $5::interval, updated_at = now()
        WHERE public_id = $1 AND deleted_at IS NULL AND lifecycle_state <> 'deleting'`,
		result.OrganizationID, state, code, detail, delay.String())
	result.State = state
	return result
}

// trimChecksum trims checksum.
func trimChecksum(value string) string { return strings.TrimSpace(value) }
