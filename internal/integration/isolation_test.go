//go:build isolation

package integration_test

import (
	"context"
	"testing"

	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTwoSchemasDoNotShareRows(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	for _, schema := range []string{"tenant_alpha", "tenant_beta"} {
		testsupport.CreateSchema(t, pool, schema)
		testsupport.ApplyMigrations(t, "db/migrations/tenant", testsupport.DatabaseURLForSchema(t, schema))
	}

	_, err := pool.Exec(context.Background(), `INSERT INTO tenant_alpha.teams (name, slug) VALUES ('Alpha', 'alpha')`)
	require.NoError(t, err)
	var alphaCount, betaCount int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM tenant_alpha.teams`).Scan(&alphaCount))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM tenant_beta.teams`).Scan(&betaCount))
	assert.Equal(t, 1, alphaCount)
	assert.Zero(t, betaCount)
}
