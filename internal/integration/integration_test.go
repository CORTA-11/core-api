//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/dbroles"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicAndTenantMigrations(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))

	var publicTable string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT to_regclass('public.orgs')::text`).Scan(&publicTable))
	assert.Equal(t, "orgs", publicTable)

	testsupport.CreateSchema(t, pool, "tenant_migration_test")
	testsupport.ApplyMigrations(t, "db/migrations/tenant", testsupport.DatabaseURLForSchema(t, "tenant_migration_test"))
	var tenantTable string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT to_regclass('tenant_migration_test.tasks')::text`).Scan(&tenantTable))
	assert.Equal(t, "tenant_migration_test.tasks", tenantTable)
}

func TestDatabaseRolesAreSeparatedAndRuntimeCannotMutateLedger(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	ctx := context.Background()

	type roleAttributes struct {
		login, superuser, createRole, createDB, replication, bypassRLS bool
	}
	for _, role := range []struct {
		name  string
		login bool
	}{
		{"synodus_owner", false},
		{"synodus_migrator", true},
		{"synodus_provisioner", true},
		{"synodus_runtime", true},
	} {
		var got roleAttributes
		err := pool.QueryRow(ctx, `
			SELECT rolcanlogin, rolsuper, rolcreaterole, rolcreatedb, rolreplication, rolbypassrls
			FROM pg_roles WHERE rolname = $1`, role.name).Scan(
			&got.login, &got.superuser, &got.createRole, &got.createDB, &got.replication, &got.bypassRLS,
		)
		require.NoError(t, err)
		assert.Equal(t, role.login, got.login)
		assert.False(t, got.superuser)
		assert.False(t, got.createRole)
		assert.False(t, got.createDB)
		assert.False(t, got.replication)
		assert.False(t, got.bypassRLS)
	}

	for _, member := range []string{"synodus_migrator", "synodus_provisioner"} {
		var inheritOption, setOption bool
		require.NoError(t, pool.QueryRow(ctx, `
			SELECT inherit_option, set_option
			FROM pg_auth_members membership
			JOIN pg_roles owner_role ON owner_role.oid = membership.roleid
			JOIN pg_roles member_role ON member_role.oid = membership.member
			WHERE owner_role.rolname = 'synodus_owner' AND member_role.rolname = $1`, member).Scan(
			&inheritOption, &setOption,
		))
		assert.False(t, inheritOption)
		assert.True(t, setOption)
	}

	var ledgerOwner string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT tableowner
		FROM pg_tables
		WHERE schemaname = 'public' AND tablename = 'schema_migrations'`).Scan(&ledgerOwner))
	assert.Equal(t, "synodus_owner", ledgerOwner)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	_, err = tx.Exec(ctx, `SET LOCAL ROLE synodus_runtime`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `CREATE SCHEMA runtime_must_not_create`)
	assert.Error(t, err)
	require.NoError(t, tx.Rollback(ctx))

	tx, err = pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	_, err = tx.Exec(ctx, `SET LOCAL ROLE synodus_runtime`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `UPDATE public.schema_migrations SET dirty = dirty`)
	assert.Error(t, err)
}

func TestDatabaseRoleBootstrapConfiguresOperationalLogins(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	databaseURL := testsupport.RequiredEnv(t, "TEST_DATABASE_URL")
	testsupport.ApplyMigrations(t, "db/migrations/public", databaseURL)

	passwords := map[string]string{
		"synodus_runtime":     "runtime-integration-password",
		"synodus_migrator":    "migrator-integration-password",
		"synodus_provisioner": "provisioner-integration-password",
	}
	require.NoError(t, dbroles.Configure(context.Background(), dbroles.Config{
		BootstrapDatabaseURL: databaseURL,
		RuntimePassword:      passwords["synodus_runtime"],
		MigratorPassword:     passwords["synodus_migrator"],
		ProvisionerPassword:  passwords["synodus_provisioner"],
	}))

	for _, test := range []struct {
		role        string
		currentUser string
	}{
		{role: "synodus_runtime", currentUser: "synodus_runtime"},
		{role: "synodus_migrator", currentUser: "synodus_owner"},
		{role: "synodus_provisioner", currentUser: "synodus_owner"},
	} {
		t.Run(test.role, func(t *testing.T) {
			config, err := pgxpool.ParseConfig(databaseURL)
			require.NoError(t, err)
			config.ConnConfig.User = test.role
			config.ConnConfig.Password = passwords[test.role]
			rolePool, err := pgxpool.NewWithConfig(context.Background(), config)
			require.NoError(t, err)
			t.Cleanup(rolePool.Close)

			var sessionUser, currentUser string
			require.NoError(t, rolePool.QueryRow(context.Background(),
				`SELECT session_user, current_user`).Scan(&sessionUser, &currentUser))
			assert.Equal(t, test.role, sessionUser)
			assert.Equal(t, test.currentUser, currentUser)
		})
	}
}

func TestPostgresQueryAndCleanup(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	_, err := pool.Exec(context.Background(), `CREATE TABLE cleanup_probe (value text); INSERT INTO cleanup_probe VALUES ('first')`)
	require.NoError(t, err)
	testsupport.ResetPostgres(t, pool)
	err = pool.QueryRow(context.Background(), `SELECT value FROM cleanup_probe`).Scan(new(string))
	assert.Error(t, err)
}

func TestRedisRoundTripAndMiss(t *testing.T) {
	client := testsupport.OpenRedis(t)
	testsupport.FlushRedis(t, client)
	ctx := context.Background()
	require.NoError(t, client.Set(ctx, "probe", "value", 0).Err())
	value, err := client.Get(ctx, "probe").Result()
	require.NoError(t, err)
	assert.Equal(t, "value", value)
	_, err = client.Get(ctx, "missing").Result()
	assert.Error(t, err)
}

func TestMinIORoundTrip(t *testing.T) {
	client := testsupport.OpenMinIO(t)
	ctx := context.Background()
	bucket := "integration-probe"
	exists, err := client.BucketExists(ctx, bucket)
	require.NoError(t, err)
	if !exists {
		require.NoError(t, client.MakeBucket(ctx, bucket, miniogo.MakeBucketOptions{}))
	}
	t.Cleanup(func() {
		testsupport.EmptyBucket(t, client, bucket)
		_ = client.RemoveBucket(context.Background(), bucket)
	})

	_, err = client.PutObject(ctx, bucket, "probe.txt", strings.NewReader("payload"), 7, miniogo.PutObjectOptions{})
	require.NoError(t, err)
	object, err := client.GetObject(ctx, bucket, "probe.txt", miniogo.GetObjectOptions{})
	require.NoError(t, err)
	defer object.Close()
	var output bytes.Buffer
	_, err = output.ReadFrom(object)
	require.NoError(t, err)
	assert.Equal(t, "payload", output.String())
}
