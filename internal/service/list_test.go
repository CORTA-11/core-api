package service

import (
	"context"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrgServiceGetOrgsUsesServerOwnedLimit(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	now := time.Now().UTC()
	mockPool.ExpectBegin()
	mockPool.ExpectExec("^SET LOCAL search_path TO public$").
		WillReturnResult(pgxmock.NewResult("SET", 0))
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
	mockPool.ExpectCommit()

	service := NewOrgService(mockPool, publicdb.New(mockPool), "postgres://unused")
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
	mockPool.ExpectExec("^SET LOCAL search_path TO public$").WillReturnResult(pgxmock.NewResult("SET", 0))
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

	service := NewOrgService(mockPool, publicdb.New(mockPool), "postgres://unused")
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
	mockPool.ExpectBegin()
	mockPool.ExpectExec("^SET LOCAL search_path TO org_example$").
		WillReturnResult(pgxmock.NewResult("SET", 0))
	mockPool.ExpectQuery("(?s)GetTeams :many.*SELECT").
		WithArgs(int32(100)).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "name", "slug", "created_at", "updated_at", "public_id", "is_quarantine",
		}).AddRow(int64(1), "Example", "example", now, now, uuid.New(), false))
	mockPool.ExpectCommit()

	service := NewTeamService(mockPool, tenantdb.New(mockPool))
	teams, err := service.GetTeams(context.Background(), "org_example")
	require.NoError(t, err)
	require.Len(t, teams, 1)
	require.NoError(t, mockPool.ExpectationsWereMet())
}

func TestTaskServiceGetTasksUsesServerOwnedLimit(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	now := time.Now().UTC()
	mockPool.ExpectBegin()
	mockPool.ExpectExec("^SET LOCAL search_path TO org_example$").
		WillReturnResult(pgxmock.NewResult("SET", 0))
	mockPool.ExpectQuery("(?s)GetTasks :many.*SELECT").
		WithArgs(int64(7), int32(100)).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "team_id", "description", "status", "created_at", "updated_at", "public_id",
		}).AddRow(int64(1), int64(7), "Example", "todo", now, now, uuid.New()))
	mockPool.ExpectCommit()

	service := NewTaskService(mockPool, tenantdb.New(mockPool))
	tasks, err := service.GetTasks(context.Background(), "org_example", 7)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.NoError(t, mockPool.ExpectationsWereMet())
}
