//go:build integration

package integration_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func provisionerFixture(t *testing.T) (*pgxpool.Pool, *tenancy.Reconciler, tenancy.MigrationSet) {
	t.Helper()
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	source, err := tenancy.EmbeddedMigrations()
	require.NoError(t, err)
	cfg := tenancy.Config{
		RetryInitial: time.Millisecond, RetryMaximum: 10 * time.Millisecond, MaxAttempts: 5,
		Concurrency: 4, OperationTimeout: time.Minute, PollInterval: time.Second, ShutdownTimeout: time.Second,
	}
	return pool, tenancy.NewReconciler(pool, source, cfg), source
}

func TestLifecycleMigrationFailsSafeForExistingNoncanonicalSchema(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	databaseURL := testsupport.RequiredEnv(t, "TEST_DATABASE_URL")
	migrator, err := migrate.New("file://"+filepath.Join(testsupport.RepositoryRoot(), "db/migrations/public"), databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = migrator.Close() })
	require.NoError(t, migrator.Steps(3))
	id := uuid.New()
	legacySchema := "org_" + id.String()
	_, err = pool.Exec(context.Background(), `INSERT INTO public.orgs (public_id, name, schema_name) VALUES ($1, 'Legacy', $2)`, id, legacySchema)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `CREATE SCHEMA `+`"`+legacySchema+`"`)
	require.NoError(t, err)

	err = migrator.Steps(1)
	require.Error(t, err)
	assert.False(t, errors.Is(err, migrate.ErrNoChange))
	var storedSchema string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT schema_name FROM public.orgs WHERE public_id = $1`, id).Scan(&storedSchema))
	assert.Equal(t, legacySchema, storedSchema)
}

func TestLifecycleConstraintsRejectInvalidRegistryStates(t *testing.T) {
	pool, _, _ := provisionerFixture(t)
	id := insertOrganization(t, pool)
	_, err := pool.Exec(context.Background(), `UPDATE public.orgs SET lifecycle_state = 'active' WHERE public_id = $1`, id)
	assert.Error(t, err)
	_, err = pool.Exec(context.Background(), `UPDATE public.orgs SET schema_name = 'org_wrong' WHERE public_id = $1`, id)
	assert.Error(t, err)
	_, err = pool.Exec(context.Background(), `UPDATE public.orgs SET lifecycle_state = 'failed' WHERE public_id = $1`, id)
	assert.Error(t, err)
}

func insertOrganization(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `
        INSERT INTO public.orgs (public_id, name, schema_name)
        VALUES ($1, 'Provisioning test', $2)`, id, tenancy.CanonicalSchema(id.String()))
	require.NoError(t, err)
	return id
}

func TestTenantProvisioningBootstrapsAndIsIdempotent(t *testing.T) {
	pool, reconciler, source := provisionerFixture(t)
	id := insertOrganization(t, pool)

	first := reconciler.Reconcile(context.Background(), id)
	require.Equal(t, tenancy.StateActive, first.State, first.ErrorDetail)
	assert.Equal(t, source.Version, first.Version)

	schema := tenancy.CanonicalSchema(id.String())
	var ledgerCount int
	var firstApplied time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM `+schema+`.schema_migrations`).Scan(&ledgerCount))
	require.Equal(t, len(source.Migrations), ledgerCount)
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT applied_at FROM `+schema+`.schema_migrations WHERE version = 1`).Scan(&firstApplied))

	second := reconciler.Reconcile(context.Background(), id)
	require.Equal(t, tenancy.StateActive, second.State, second.ErrorDetail)
	var secondApplied time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT applied_at FROM `+schema+`.schema_migrations WHERE version = 1`).Scan(&secondApplied))
	assert.Equal(t, firstApplied, secondApplied)
}

func TestTenantProvisioningAdoptsVerifiedM01LedgerWithoutDataLoss(t *testing.T) {
	pool, reconciler, source := provisionerFixture(t)
	id := insertOrganization(t, pool)
	schema := tenancy.CanonicalSchema(id.String())
	testsupport.CreateSchema(t, pool, schema)
	legacyMigrator, err := migrate.New(
		"file://"+filepath.Join(testsupport.RepositoryRoot(), "db/migrations/tenant"),
		testsupport.DatabaseURLForSchema(t, schema),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = legacyMigrator.Close() })
	require.NoError(t, legacyMigrator.Steps(2))
	_, err = pool.Exec(context.Background(), `INSERT INTO `+schema+`.teams (name, slug) VALUES ('Preserved', 'preserved')`)
	require.NoError(t, err)

	result := reconciler.Reconcile(context.Background(), id)
	require.Equal(t, tenancy.StateActive, result.State, result.ErrorDetail)
	var teamCount, ledgerCount int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM `+schema+`.teams WHERE slug = 'preserved'`).Scan(&teamCount))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM `+schema+`.schema_migrations`).Scan(&ledgerCount))
	assert.Equal(t, 1, teamCount)
	assert.Equal(t, len(source.Migrations), ledgerCount)
}

func TestTenantProvisioningRejectsDirtyLegacyAndChecksumDivergence(t *testing.T) {
	t.Run("dirty legacy ledger", func(t *testing.T) {
		pool, reconciler, _ := provisionerFixture(t)
		id := insertOrganization(t, pool)
		schema := tenancy.CanonicalSchema(id.String())
		testsupport.CreateSchema(t, pool, schema)
		_, err := pool.Exec(context.Background(), `CREATE TABLE `+schema+`.schema_migrations (version bigint NOT NULL, dirty boolean NOT NULL); INSERT INTO `+schema+`.schema_migrations VALUES (1, true)`)
		require.NoError(t, err)
		result := reconciler.Reconcile(context.Background(), id)
		assert.Equal(t, tenancy.StateFailed, result.State)
		assert.Equal(t, "legacy_migration_dirty", result.ErrorCode)
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		pool, reconciler, _ := provisionerFixture(t)
		id := insertOrganization(t, pool)
		require.Equal(t, tenancy.StateActive, reconciler.Reconcile(context.Background(), id).State)
		schema := tenancy.CanonicalSchema(id.String())
		_, err := pool.Exec(context.Background(), `UPDATE `+schema+`.schema_migrations SET checksum = $1 WHERE version = 1`, strings.Repeat("f", 64))
		require.NoError(t, err)
		result := reconciler.Reconcile(context.Background(), id)
		assert.Equal(t, tenancy.StateFailed, result.State)
		assert.Equal(t, "migration_checksum_divergence", result.ErrorCode)
	})
}

func TestProvisionerAutomaticallyMigratesStaleActiveTenant(t *testing.T) {
	pool, reconciler, source := provisionerFixture(t)
	id := insertOrganization(t, pool)
	schema := tenancy.CanonicalSchema(id.String())
	_, err := pool.Exec(context.Background(), `CREATE SCHEMA `+schema)
	require.NoError(t, err)
	tx, err := pool.Begin(context.Background())
	require.NoError(t, err)
	require.NoError(t, func() error {
		if _, err := tx.Exec(context.Background(), `SELECT set_config('search_path', $1, true)`, schema); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `CREATE TABLE schema_migrations (version BIGINT PRIMARY KEY, checksum CHAR(64) NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), source.Migrations[0].SQL); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `INSERT INTO schema_migrations (version, checksum) VALUES (1, $1)`, source.Migrations[0].Checksum); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `UPDATE public.orgs SET lifecycle_state = 'active', tenant_version = 1, tenant_checksum = $2, provisioned_at = now() WHERE public_id = $1`, id, strings.Repeat("a", 64))
		return err
	}())
	require.NoError(t, tx.Commit(context.Background()))

	claimed, err := reconciler.ClaimDue(context.Background(), 4)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{id}, claimed)
	result := reconciler.Reconcile(context.Background(), id)
	require.Equal(t, tenancy.StateActive, result.State, result.ErrorDetail)
	assert.Equal(t, source.Version, result.Version)
}

func TestProvisionerPersistsRetriesAndTerminalFailure(t *testing.T) {
	pool, _, _ := provisionerFixture(t)
	id := insertOrganization(t, pool)
	badSource := tenancy.MigrationSet{
		Migrations: []tenancy.Migration{{Version: 1, Name: "000001_invalid.up.sql", SQL: "THIS IS NOT SQL", Checksum: strings.Repeat("a", 64)}},
		Version:    1, Checksum: strings.Repeat("b", 64),
	}
	cfg := tenancy.Config{RetryInitial: time.Millisecond, RetryMaximum: 10 * time.Millisecond, MaxAttempts: 5, Concurrency: 4, OperationTimeout: time.Minute}
	reconciler := tenancy.NewReconciler(pool, badSource, cfg)
	for attempt := 1; attempt <= 5; attempt++ {
		result := reconciler.Reconcile(context.Background(), id)
		assert.Equal(t, attempt, result.Attempts)
		if attempt < 5 {
			assert.Equal(t, tenancy.StateProvisioning, result.State)
		} else {
			assert.Equal(t, tenancy.StateFailed, result.State)
		}
		assert.Equal(t, "reconciliation_failed", result.ErrorCode)
		assert.NotContains(t, result.ErrorDetail, "THIS IS NOT SQL")
	}
	count, err := reconciler.Retry(context.Background(), &id)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	status, err := reconciler.Status(context.Background(), &id)
	require.NoError(t, err)
	require.Len(t, status, 1)
	assert.Equal(t, 0, status[0].Attempts)
	assert.Equal(t, tenancy.StateProvisioning, status[0].State)
}

func TestConcurrentProvisioningSerializesOneTenant(t *testing.T) {
	pool, reconciler, source := provisionerFixture(t)
	id := insertOrganization(t, pool)
	results := reconciler.ReconcileMany(context.Background(), []uuid.UUID{id, id}, 2)
	for result := range results {
		require.Equal(t, tenancy.StateActive, result.State, result.ErrorDetail)
	}
	var count int
	schema := tenancy.CanonicalSchema(id.String())
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM `+schema+`.schema_migrations`).Scan(&count))
	assert.Equal(t, len(source.Migrations), count)
}
