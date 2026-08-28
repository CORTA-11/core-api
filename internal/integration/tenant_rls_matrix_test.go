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

	alphaTeams, err := fixture.teamService.GetTeams(ctx, alphaSpecific)
	require.NoError(t, err)
	require.Len(t, alphaTeams, 2)
	assert.ElementsMatch(t, []uuid.UUID{alpha.teams[0].publicID, alpha.teams[1].publicID}, []uuid.UUID{
		alphaTeams[0].PublicID, alphaTeams[1].PublicID,
	})
	betaTeams, err := fixture.teamService.GetTeams(ctx, betaSpecific)
	require.NoError(t, err)
	require.Len(t, betaTeams, 2)
	assert.ElementsMatch(t, []uuid.UUID{beta.teams[0].publicID, beta.teams[1].publicID}, []uuid.UUID{
		betaTeams[0].PublicID, betaTeams[1].PublicID,
	})

	createdTeam, err := fixture.teamService.CreateTeam(ctx, alphaShared, "Alpha Runtime Team", "shared@tenant-boundary.example.test")
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

	createdTask, err := fixture.taskService.CreateTask(ctx, alphaTeamOne, "same-team runtime write", "in_progress")
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, createdTask.PublicID)
	updatedTask, err := fixture.taskService.UpdateTask(ctx, alphaTeamOne, createdTask.PublicID, "same-team runtime update", "done")
	require.NoError(t, err)
	assert.Equal(t, "same-team runtime update", updatedTask.Description)
	assert.Equal(t, "done", updatedTask.Status)
	deletedTask, err := fixture.taskService.DeleteTask(ctx, alphaTeamOne, createdTask.PublicID)
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
			_, updateErr := fixture.taskService.UpdateTask(ctx, alphaTeamOne, denied.target, "forbidden update", "done")
			require.True(t, errors.Is(updateErr, pgx.ErrNoRows), updateErr)
			_, deleteErr := fixture.taskService.DeleteTask(ctx, alphaTeamOne, denied.target)
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
	tasks, err := fixture.taskService.GetTasks(ctx, team)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, wantTask, tasks[0].PublicID)

	var generatedRows []tenantdb.Task
	require.NoError(t, fixture.executor.WithinTeam(ctx, team, func(queries *tenantdb.Queries) error {
		var queryErr error
		generatedRows, queryErr = queries.GetTasks(ctx, 100)
		return queryErr
	}))
	require.Len(t, generatedRows, 1)
	assert.Equal(t, wantTask, generatedRows[0].PublicID)
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
