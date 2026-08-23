package service

import (
	"context"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type immediateOrganizationExecutor struct {
	queries *tenantdb.Queries
}

type immediateTeamExecutor struct {
	queries *tenantdb.Queries
}

func (executor immediateTeamExecutor) WithinTeam(
	ctx context.Context,
	_ tenancy.TeamContext,
	callback func(*tenantdb.Queries) error,
) error {
	return callback(executor.queries)
}

func (executor immediateOrganizationExecutor) WithinOrganization(
	ctx context.Context,
	_ tenancy.OrganizationContext,
	callback func(*tenantdb.Queries) error,
) error {
	return callback(executor.queries)
}

func TestOrgServiceGetOrgsUsesServerOwnedLimit(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	now := time.Now().UTC()
	mockPool.ExpectQuery("(?s)GetOrgs :many.*SELECT").
		WithArgs(int32(100)).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "public_id", "name", "schema_name", "created_at", "updated_at", "deleted_at",
			"lifecycle_state", "tenant_version", "tenant_checksum", "reconcile_attempts", "next_attempt_at",
			"last_error_code", "last_error_detail", "last_attempt_at", "provisioned_at",
		}).AddRow(
			int64(1), uuid.New(), "Example", "org_example", now, now, pgtype.Timestamptz{Valid: false},
			"provisioning", int64(0), "", int32(0), now, pgtype.Text{}, pgtype.Text{}, pgtype.Timestamptz{}, pgtype.Timestamptz{},
		))
	service := NewOrgService(mockPool, publicdb.New(mockPool))
	orgs, err := service.GetOrgs(context.Background())
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	require.NoError(t, mockPool.ExpectationsWereMet())
}

func TestOrgServiceCreateOrgOnlyRecordsProvisioningIntent(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()
	now := time.Now().UTC()
	mockPool.ExpectBegin()
	mockPool.ExpectQuery("(?s)CreateOrg :one.*INSERT").
		WithArgs("Example", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "public_id", "name", "schema_name", "created_at", "updated_at", "deleted_at",
			"lifecycle_state", "tenant_version", "tenant_checksum", "reconcile_attempts", "next_attempt_at",
			"last_error_code", "last_error_detail", "last_attempt_at", "provisioned_at",
		}).AddRow(
			int64(1), uuid.New(), "Example", "org_example", now, now, pgtype.Timestamptz{},
			"provisioning", int64(0), "", int32(0), now, pgtype.Text{}, pgtype.Text{}, pgtype.Timestamptz{}, pgtype.Timestamptz{},
		))
	mockPool.ExpectCommit()

	service := NewOrgService(mockPool, publicdb.New(mockPool))
	organization, err := service.CreateOrg(context.Background(), "Example")
	require.NoError(t, err)
	assert.Equal(t, "provisioning", organization.LifecycleState)
	require.NoError(t, mockPool.ExpectationsWereMet())
}

func TestTeamServiceGetTeamsUsesServerOwnedLimit(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	now := time.Now().UTC()
	mockPool.ExpectQuery("(?s)GetTeams :many.*SELECT").
		WithArgs(int32(100)).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "name", "slug", "created_at", "updated_at", "public_id", "is_quarantine",
		}).AddRow(int64(1), "Example", "example", now, now, uuid.New(), false))

	service := NewTeamService(immediateOrganizationExecutor{queries: tenantdb.New(mockPool)})
	teams, err := service.GetTeams(context.Background(), tenancy.OrganizationContext{})
	require.NoError(t, err)
	require.Len(t, teams, 1)
	require.NoError(t, mockPool.ExpectationsWereMet())
}

func TestTaskServiceGetTasksUsesServerOwnedLimit(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	now := time.Now().UTC()
	mockPool.ExpectQuery("(?s)GetTasks :many.*SELECT").
		WithArgs(int32(100)).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "team_id", "description", "status", "created_at", "updated_at", "public_id",
		}).AddRow(int64(1), int64(7), "Example", "todo", now, now, uuid.New()))

	service := NewTaskService(immediateTeamExecutor{queries: tenantdb.New(mockPool)})
	tasks, err := service.GetTasks(context.Background(), tenancy.TeamContext{})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.NoError(t, mockPool.ExpectationsWereMet())
}
