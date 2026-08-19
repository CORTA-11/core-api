//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/testsupport"
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
