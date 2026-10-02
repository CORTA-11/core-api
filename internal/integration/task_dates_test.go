//go:build isolation

package integration_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskDatesPersistAcrossWritesAndReload(t *testing.T) {
	fixture := newTenantBoundaryFixture(t)
	ctx := context.Background()
	org := fixture.orgs[0]
	team := org.teams[0]
	principal := session.Principal{UserID: fixture.users.shared, SessionID: uuid.New()}
	_, err := fixture.adminPool.Exec(ctx, `UPDATE `+org.schema+`.team_members SET role = 'researcher' WHERE user_public_id = $1`, fixture.users.shared)
	require.NoError(t, err)
	codec, err := pagination.NewCodec(pagination.CodecConfig{Active: pagination.Key{
		ID: "task-date-test", Secret: bytes.Repeat([]byte{0x71}, 32),
	}})
	require.NoError(t, err)
	application := service.NewTeamTaskApplication(authorization.NewAuthorizer(fixture.resolver, fixture.executor), codec)
	start := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	due := start.AddDate(0, 0, 2)
	details := "Detailed instructions\nwith a second line"
	created, err := application.CreateTask(ctx, principal, org.publicID, team.publicID, "Dated task", "todo", &fixture.users.shared,
		service.TaskDates{StartDate: &start, DueDate: &due}, &details)
	require.NoError(t, err)
	assert.Equal(t, &start, created.StartDate)
	assert.Equal(t, &due, created.DueDate)
	assert.Equal(t, details, created.Details)

	// Reload from the database, not just the write response.
	page, err := application.ListTasks(ctx, principal, org.publicID, team.publicID, pagination.Parameters{PageSize: 100})
	require.NoError(t, err)
	found := false
	for _, item := range page.Items {
		if item.ID == created.ID {
			found = true
			assert.Equal(t, &start, item.StartDate)
			assert.Equal(t, &due, item.DueDate)
			assert.Equal(t, details, item.Details)
		}
	}
	require.True(t, found)

	moved, err := application.UpdateTask(ctx, principal, org.publicID, team.publicID, created.ID, "Dated task", "done", nil, false, service.TaskDates{}, nil)
	require.NoError(t, err)
	assert.Equal(t, &start, moved.StartDate)
	assert.Equal(t, &due, moved.DueDate)
	assert.Equal(t, details, moved.Details)
	assert.Equal(t, &fixture.users.shared, moved.AssigneeID)

	details = "Edited instructions"
	newDue := due.AddDate(0, 0, 3)
	updated, err := application.UpdateTask(ctx, principal, org.publicID, team.publicID, created.ID, "Edited and unassigned", "in_progress", nil, true,
		service.TaskDates{SetStartDate: true, SetDueDate: true, DueDate: &newDue}, &details)
	require.NoError(t, err)
	assert.Nil(t, updated.StartDate)
	assert.Equal(t, &newDue, updated.DueDate)
	assert.Nil(t, updated.AssigneeID)
	assert.Equal(t, "Edited and unassigned", updated.Description)
	assert.Equal(t, "in_progress", updated.Status)
	assert.Equal(t, details, updated.Details)

	details = ""
	cleared, err := application.UpdateTask(ctx, principal, org.publicID, team.publicID, created.ID, "Dated task", "todo", nil, false,
		service.TaskDates{SetDueDate: true}, &details)
	require.NoError(t, err)
	assert.Nil(t, cleared.StartDate)
	assert.Nil(t, cleared.DueDate)
	page, err = application.ListTasks(ctx, principal, org.publicID, team.publicID, pagination.Parameters{PageSize: 100})
	require.NoError(t, err)
	for _, item := range page.Items {
		if item.ID == created.ID {
			assert.Empty(t, item.Details)
			assert.Equal(t, "Dated task", item.Description)
		}
	}

	_, err = application.UpdateTask(ctx, principal, fixture.orgs[1].publicID, fixture.orgs[1].teams[0].publicID,
		created.ID, "Forbidden", "todo", nil, false, service.TaskDates{SetDueDate: true, DueDate: &due}, &details)
	require.Error(t, err)
}
