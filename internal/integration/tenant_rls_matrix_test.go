//go:build isolation

package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenantRLSMatrix(t *testing.T) {
	fixture := newTenantBoundaryFixture(t)
	ctx := context.Background()
	alpha := fixture.orgs[0]
	beta := fixture.orgs[1]

	alphaShared := fixture.resolveOrganization(t, fixture.users.shared, alpha)
	betaShared := fixture.resolveOrganization(t, fixture.users.shared, beta)
	alphaSpecific := fixture.resolveOrganization(t, fixture.users.alpha, alpha)
	betaSpecific := fixture.resolveOrganization(t, fixture.users.beta, beta)

	for _, denied := range []struct {
		name         string
		user         uuid.UUID
		organization uuid.UUID
	}{
		{"outsider alpha", fixture.users.outsider, alpha.publicID},
		{"outsider beta", fixture.users.outsider, beta.publicID},
		{"alpha user in beta", fixture.users.alpha, beta.publicID},
		{"beta user in alpha", fixture.users.beta, alpha.publicID},
		{"unknown organization", fixture.users.shared, uuid.MustParse("10000000-0000-4000-8000-000000000099")},
	} {
		t.Run("organization resolution rejects "+denied.name, func(t *testing.T) {
			_, err := fixture.resolver.ResolveOrganization(ctx, denied.user, denied.organization)
			require.ErrorIs(t, err, tenancy.ErrOrganizationUnavailable)
		})
	}

	alphaTeamOne := fixture.resolveTeam(t, fixture.users.shared, alpha, alpha.teams[0])
	alphaTeamTwo := fixture.resolveTeam(t, fixture.users.shared, alpha, alpha.teams[1])
	betaTeamOne := fixture.resolveTeam(t, fixture.users.shared, beta, beta.teams[0])
	betaTeamTwo := fixture.resolveTeam(t, fixture.users.shared, beta, beta.teams[1])

	for _, denied := range []struct {
		name         string
		organization tenancy.OrganizationContext
		team         uuid.UUID
	}{
		{"beta team through alpha context", alphaShared, beta.teams[0].publicID},
		{"alpha team through beta context", betaShared, alpha.teams[0].publicID},
		{"task UUID used as team UUID", alphaShared, alpha.teams[0].taskID},
		{"unknown team", alphaShared, uuid.MustParse("30000000-0000-4000-8000-000000000099")},
	} {
		t.Run("team resolution rejects "+denied.name, func(t *testing.T) {
			_, err := fixture.resolver.ResolveTeam(ctx, denied.organization, denied.team)
			require.ErrorIs(t, err, tenancy.ErrTeamUnavailable)
		})
	}

	var alphaTeams []tenantdb.Team
	err := fixture.executor.WithinOrganization(ctx, alphaSpecific, func(queries *tenantdb.Queries) error {
		var err error
		alphaTeams, err = queries.GetTeams(ctx, 100)
		return err
	})
	require.NoError(t, err)
	require.Len(t, alphaTeams, 2)
	assert.ElementsMatch(t, []uuid.UUID{alpha.teams[0].publicID, alpha.teams[1].publicID}, []uuid.UUID{
		alphaTeams[0].PublicID, alphaTeams[1].PublicID,
	})
	var betaTeams []tenantdb.Team
	err = fixture.executor.WithinOrganization(ctx, betaSpecific, func(queries *tenantdb.Queries) error {
		var err error
		betaTeams, err = queries.GetTeams(ctx, 100)
		return err
	})
	require.NoError(t, err)
	require.Len(t, betaTeams, 2)
	assert.ElementsMatch(t, []uuid.UUID{beta.teams[0].publicID, beta.teams[1].publicID}, []uuid.UUID{
		betaTeams[0].PublicID, betaTeams[1].PublicID,
	})

	var createdTeam tenantdb.Team
	err = fixture.executor.WithinOrganization(ctx, alphaShared, func(queries *tenantdb.Queries) error {
		var err error
		createdTeam, err = queries.CreateTeamWithCreator(ctx, tenantdb.CreateTeamWithCreatorParams{
			Name: "Alpha Runtime Team", Slug: "alpha-runtime-team", LeaderEmail: "shared@tenant-boundary.example.test",
		})
		return err
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, createdTeam.PublicID)
	_, err = fixture.resolver.ResolveTeam(ctx, alphaShared, createdTeam.PublicID)
	require.NoError(t, err)
	_, err = fixture.resolver.ResolveTeam(ctx, betaShared, createdTeam.PublicID)
	require.ErrorIs(t, err, tenancy.ErrTeamUnavailable)

	assertTaskScope(t, fixture, alphaTeamOne, alpha.teams[0].taskID)
	assertTaskScope(t, fixture, alphaTeamTwo, alpha.teams[1].taskID)
	assertTaskScope(t, fixture, betaTeamOne, beta.teams[0].taskID)
	assertTaskScope(t, fixture, betaTeamTwo, beta.teams[1].taskID)

	var createdTask tenantdb.Task
	err = fixture.executor.WithinTeam(ctx, alphaTeamOne, func(queries *tenantdb.Queries) error {
		var err error
		createdTask, err = queries.CreateTask(ctx, tenantdb.CreateTaskParams{Description: "same-team runtime write", Status: "in_progress"})
		return err
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, createdTask.PublicID)
	var updatedTask tenantdb.Task
	err = fixture.executor.WithinTeam(ctx, alphaTeamOne, func(queries *tenantdb.Queries) error {
		var err error
		updatedTask, err = queries.UpdateTask(ctx, tenantdb.UpdateTaskParams{PublicID: createdTask.PublicID, Description: "same-team runtime update", Status: "done"})
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, "same-team runtime update", updatedTask.Description)
	assert.Equal(t, "done", updatedTask.Status)
	var deletedTask tenantdb.Task
	err = fixture.executor.WithinTeam(ctx, alphaTeamOne, func(queries *tenantdb.Queries) error {
		var err error
		deletedTask, err = queries.DeleteTask(ctx, createdTask.PublicID)
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, createdTask.PublicID, deletedTask.PublicID)

	for _, denied := range []struct {
		name   string
		target uuid.UUID
	}{
		{"cross-team", alpha.teams[1].taskID},
		{"cross-organization", beta.teams[0].taskID},
		{"unknown", uuid.MustParse("40000000-0000-4000-8000-000000000099")},
	} {
		t.Run(denied.name+" task mutations are absent", func(t *testing.T) {
			updateErr := fixture.executor.WithinTeam(ctx, alphaTeamOne, func(queries *tenantdb.Queries) error {
				_, err := queries.UpdateTask(ctx, tenantdb.UpdateTaskParams{PublicID: denied.target, Description: "forbidden update", Status: "done"})
				return err
			})
			require.True(t, errors.Is(updateErr, pgx.ErrNoRows), updateErr)
			deleteErr := fixture.executor.WithinTeam(ctx, alphaTeamOne, func(queries *tenantdb.Queries) error {
				_, err := queries.DeleteTask(ctx, denied.target)
				return err
			})
			require.True(t, errors.Is(deleteErr, pgx.ErrNoRows), deleteErr)
		})
	}

	assertPrivilegedTask(t, fixture, alpha, alpha.teams[1].taskID, alpha.teams[1].slug+" initial task", "todo")
	assertPrivilegedTask(t, fixture, beta, beta.teams[0].taskID, beta.teams[0].slug+" initial task", "todo")
	assertRuntimePoolClean(t, fixture.runtimePool)
}

func assertTaskScope(t *testing.T, fixture *tenantBoundaryFixture, team tenancy.TeamContext, wantTask uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	tasks, err := fixture.readTasks(ctx, team)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, wantTask, tasks[0].PublicID)
}

func assertPrivilegedTask(
	t *testing.T,
	fixture *tenantBoundaryFixture,
	organization tenantBoundaryOrganization,
	publicID uuid.UUID,
	wantDescription string,
	wantStatus string,
) {
	t.Helper()
	var description, status string
	err := fixture.adminPool.QueryRow(context.Background(), `SELECT description, status FROM `+
		pgx.Identifier{organization.schema, "tasks"}.Sanitize()+` WHERE public_id = $1`, publicID).Scan(&description, &status)
	require.NoError(t, err)
	assert.Equal(t, wantDescription, description)
	assert.Equal(t, wantStatus, status)
}
