//go:build integration

package integration_test

import (
	"context"
	"sync"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatedOrganizationCreationIsAtomicAndCreatesOnlyCreatorOwner(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	ctx := context.Background()
	userID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO public.users (user_id, email, password_hash, display_name)
		VALUES ($1, 'creator@example.test', 'hash', 'Creator')`, userID)
	require.NoError(t, err)
	creator := authorization.NewOrganizationCreator(pool)
	created, err := creator.Create(ctx, session.Principal{UserID: userID, SessionID: uuid.New()}, "Created atomically")
	require.NoError(t, err)
	assert.Equal(t, tenancy.StateProvisioning, created.State)

	var memberships, owners int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE membership.role = 'owner')
		FROM public.org_user AS membership
		JOIN public.orgs AS organization ON organization.id = membership.org_id
		WHERE organization.public_id = $1`, created.PublicID).Scan(&memberships, &owners))
	assert.Equal(t, 1, memberships)
	assert.Equal(t, 1, owners)
	var tenantSchemas int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname = $1`,
		tenancy.CanonicalSchema(created.PublicID.String())).Scan(&tenantSchemas))
	assert.Zero(t, tenantSchemas, "request path must not create tenant state or team membership")

	var before int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM public.orgs`).Scan(&before))
	_, err = creator.Create(ctx, session.Principal{UserID: uuid.New(), SessionID: uuid.New()}, "Must roll back")
	assert.ErrorIs(t, err, authorization.ErrUnauthenticated)
	var after int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM public.orgs`).Scan(&after))
	assert.Equal(t, before, after)
}

func TestConcurrentOwnerRemovalPreservesAnOwner(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	ctx := context.Background()
	source, err := tenancy.EmbeddedMigrations()
	require.NoError(t, err)
	organizationID := uuid.New()
	var organizationInternalID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO public.orgs (public_id, name, schema_name, lifecycle_state, tenant_version, tenant_checksum, provisioned_at)
		VALUES ($1, 'Owner concurrency', $2, 'active', $3, $4, now()) RETURNING id`,
		organizationID, tenancy.CanonicalSchema(organizationID.String()), source.Version, source.Checksum).Scan(&organizationInternalID))
	users := []uuid.UUID{uuid.New(), uuid.New()}
	for index, userID := range users {
		var internalID int64
		require.NoError(t, pool.QueryRow(ctx, `
			INSERT INTO public.users (user_id, email, password_hash, display_name)
			VALUES ($1, $2::text, 'hash', 'Owner') RETURNING id`, userID,
			userID.String()+"@example.test").Scan(&internalID))
		_, err := pool.Exec(ctx, `INSERT INTO public.org_user (org_id, user_id, role) VALUES ($1, $2, 'owner')`,
			organizationInternalID, internalID)
		require.NoError(t, err, index)
	}

	manager := authorization.NewMembershipManager(pool)
	results := make(chan error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range users {
		wait.Add(1)
		go func(actorIndex int) {
			defer wait.Done()
			<-start
			results <- manager.Remove(ctx,
				session.Principal{UserID: users[actorIndex], SessionID: uuid.New()},
				organizationID, users[1-actorIndex])
		}(index)
	}
	close(start)
	wait.Wait()
	close(results)
	var successes int
	for result := range results {
		if result == nil {
			successes++
		}
	}
	assert.Equal(t, 1, successes)
	var owners int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM public.org_user WHERE org_id = $1 AND role = 'owner'`,
		organizationInternalID).Scan(&owners))
	assert.Equal(t, 1, owners)
}
