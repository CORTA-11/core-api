//go:build isolation

package integration_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestM02D05PredicateFreeQueryAndUnsafeSettingsDefaultToDeny(t *testing.T) {
	fixture := newD05Fixture(t)
	ctx := context.Background()

	for _, scope := range []struct {
		name         string
		organization d05Organization
		team         d05Team
	}{
		{"alpha one", fixture.orgs[0], fixture.orgs[0].teams[0]},
		{"alpha two", fixture.orgs[0], fixture.orgs[0].teams[1]},
		{"beta one", fixture.orgs[1], fixture.orgs[1].teams[0]},
		{"beta two", fixture.orgs[1], fixture.orgs[1].teams[1]},
	} {
		t.Run(scope.name+" is contained by RLS", func(t *testing.T) {
			team := fixture.resolveTeam(t, fixture.users.shared, scope.organization, scope.team)
			var tasks []tenantdb.Task
			require.NoError(t, fixture.executor.WithinTeam(ctx, team, func(queries *tenantdb.Queries) error {
				var err error
				tasks, err = queries.IsolationProbeTasks(ctx, 8)
				return err
			}))
			require.Len(t, tasks, 1)
			assert.Equal(t, scope.team.taskID, tasks[0].PublicID)
			assert.Equal(t, scope.team.id, tasks[0].TeamID)
		})
	}

	alpha := fixture.orgs[0]
	validUser := fixture.users.shared.String()
	validTeam := strconv.FormatInt(alpha.teams[0].id, 10)
	for _, test := range []struct {
		name        string
		userSetting *string
		teamSetting *string
		wantError   bool
	}{
		{"both settings missing", nil, nil, false},
		{"user setting missing", nil, &validTeam, false},
		{"team setting missing", &validUser, nil, false},
		{"malformed user setting", stringPointer("not-a-uuid"), &validTeam, true},
		{"malformed team setting", &validUser, stringPointer("not-a-team-id"), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			tasks, err := fixture.unsafeProbeWithSettings(ctx, alpha.schema, test.userSetting, test.teamSetting)
			if test.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Empty(t, tasks)
			}
			assertD05PoolClean(t, fixture.runtimePool)
		})
	}
}

func TestM02D05RejectsForgedRegistryAndRechecksRevokedMembership(t *testing.T) {
	fixture := newD05Fixture(t)
	ctx := context.Background()
	alpha := fixture.orgs[0]
	team := alpha.teams[0]

	_, err := fixture.adminPool.Exec(ctx, `ALTER TABLE public.orgs DROP CONSTRAINT orgs_canonical_schema_check`)
	require.NoError(t, err)
	_, err = fixture.adminPool.Exec(ctx, `UPDATE public.orgs SET schema_name = 'public' WHERE public_id = $1`, alpha.publicID)
	require.NoError(t, err)
	_, err = fixture.resolver.ResolveOrganization(ctx, fixture.users.shared, alpha.publicID)
	require.ErrorIs(t, err, tenancy.ErrRegistryIntegrity)
	_, err = fixture.adminPool.Exec(ctx, `UPDATE public.orgs SET schema_name = $1 WHERE public_id = $2`, alpha.schema, alpha.publicID)
	require.NoError(t, err)

	resolvedOrganization := fixture.resolveOrganization(t, fixture.users.alpha, alpha)
	resolvedTeam, err := fixture.resolver.ResolveTeam(ctx, resolvedOrganization, team.publicID)
	require.NoError(t, err)
	membersTable := pgx.Identifier{alpha.schema, "team_members"}.Sanitize()
	_, err = fixture.adminPool.Exec(ctx, `DELETE FROM `+membersTable+` WHERE team_id = $1 AND user_public_id = $2`,
		team.id, fixture.users.alpha)
	require.NoError(t, err)

	tasks, err := fixture.taskService.GetTasks(ctx, resolvedTeam)
	require.NoError(t, err)
	assert.Empty(t, tasks)
	_, err = fixture.taskService.CreateTask(ctx, resolvedTeam, "revoked write", "todo")
	require.Error(t, err)
	_, err = fixture.resolver.ResolveTeam(ctx, resolvedOrganization, team.publicID)
	require.ErrorIs(t, err, tenancy.ErrTeamUnavailable)
	assertTaskDescriptionAbsent(t, fixture, alpha, "revoked write")

	_, err = fixture.adminPool.Exec(ctx, `INSERT INTO `+membersTable+`
		(team_id, user_public_id, role) VALUES ($1, $2, 'viewer')`, team.id, fixture.users.alpha)
	require.NoError(t, err)
	tasks, err = fixture.taskService.GetTasks(ctx, resolvedTeam)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, team.taskID, tasks[0].PublicID)
	assertD05PoolClean(t, fixture.runtimePool)
}

func TestM02D05ExecutorFaultsRollbackAndLeavePoolReusable(t *testing.T) {
	fixture := newD05Fixture(t)
	ctx := context.Background()
	alpha := fixture.orgs[0]
	beta := fixture.orgs[1]
	alphaTeam := fixture.resolveTeam(t, fixture.users.shared, alpha, alpha.teams[0])
	betaTeam := fixture.resolveTeam(t, fixture.users.shared, beta, beta.teams[1])

	callbackErr := errors.New("d05 callback rollback")
	err := fixture.executor.WithinTeam(ctx, alphaTeam, func(queries *tenantdb.Queries) error {
		_, createErr := queries.CreateTask(ctx, tenantdb.CreateTaskParams{Description: "callback rollback", Status: "todo"})
		require.NoError(t, createErr)
		return callbackErr
	})
	require.ErrorIs(t, err, callbackErr)
	assertTaskDescriptionAbsent(t, fixture, alpha, "callback rollback")
	assertD05PoolClean(t, fixture.runtimePool)

	panicValue := &struct{ operation string }{operation: "d05 panic rollback"}
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = fixture.executor.WithinTeam(ctx, alphaTeam, func(queries *tenantdb.Queries) error {
			_, createErr := queries.CreateTask(ctx, tenantdb.CreateTaskParams{Description: "panic rollback", Status: "todo"})
			require.NoError(t, createErr)
			panic(panicValue)
		})
	}()
	assert.Same(t, panicValue, recovered)
	assertTaskDescriptionAbsent(t, fixture, alpha, "panic rollback")
	assertD05PoolClean(t, fixture.runtimePool)

	canceledContext, cancel := context.WithCancel(ctx)
	err = fixture.executor.WithinTeam(canceledContext, alphaTeam, func(queries *tenantdb.Queries) error {
		_, createErr := queries.CreateTask(canceledContext, tenantdb.CreateTaskParams{Description: "cancellation rollback", Status: "todo"})
		require.NoError(t, createErr)
		cancel()
		return nil
	})
	require.True(t, errors.Is(err, context.Canceled), err)
	assertTaskDescriptionAbsent(t, fixture, alpha, "cancellation rollback")
	assertD05PoolClean(t, fixture.runtimePool)

	alphaTasks, err := fixture.taskService.GetTasks(ctx, alphaTeam)
	require.NoError(t, err)
	require.Len(t, alphaTasks, 1)
	assert.Equal(t, alpha.teams[0].taskID, alphaTasks[0].PublicID)
	betaTasks, err := fixture.taskService.GetTasks(ctx, betaTeam)
	require.NoError(t, err)
	require.Len(t, betaTasks, 1)
	assert.Equal(t, beta.teams[1].taskID, betaTasks[0].PublicID)
	assertD05PoolClean(t, fixture.runtimePool)
}

func (fixture *d05Fixture) unsafeProbeWithSettings(
	ctx context.Context,
	schema string,
	userSetting *string,
	teamSetting *string,
) ([]tenantdb.Task, error) {
	tx, err := fixture.runtimePool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	searchPath := pgx.Identifier{"pg_catalog"}.Sanitize() + ", " + pgx.Identifier{schema}.Sanitize()
	if _, err = tx.Exec(ctx, `SELECT set_config('search_path', $1, true)`, searchPath); err != nil {
		return nil, err
	}
	if userSetting != nil {
		if _, err = tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, *userSetting); err != nil {
			return nil, err
		}
	}
	if teamSetting != nil {
		if _, err = tx.Exec(ctx, `SELECT set_config('app.team_id', $1, true)`, *teamSetting); err != nil {
			return nil, err
		}
	}
	return tenantdb.New(tx).IsolationProbeTasks(ctx, 8)
}

func assertTaskDescriptionAbsent(
	t *testing.T,
	fixture *d05Fixture,
	organization d05Organization,
	description string,
) {
	t.Helper()
	var count int
	err := fixture.adminPool.QueryRow(context.Background(), `SELECT count(*) FROM `+
		pgx.Identifier{organization.schema, "tasks"}.Sanitize()+` WHERE description = $1`, description).Scan(&count)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func stringPointer(value string) *string {
	return &value
}
