//go:build isolation

package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceRequestsArePrivateAndConcurrentApprovalHasOneWinner(t *testing.T) {
	fixture := newTenantBoundaryFixture(t)
	ctx := context.Background()
	organization := fixture.orgs[0]
	owner := session.Principal{UserID: fixture.users.shared, SessionID: uuid.New()}
	member := session.Principal{UserID: fixture.users.alpha, SessionID: uuid.New()}

	_, err := fixture.adminPool.Exec(ctx, `UPDATE public.org_user SET role = CASE
		WHEN user_id = (SELECT id FROM public.users WHERE user_id = $2) THEN 'owner' ELSE 'member' END
		WHERE org_id = $1`, organization.id, fixture.users.shared)
	require.NoError(t, err)
	members := pgx.Identifier{organization.schema, "team_members"}.Sanitize()
	_, err = fixture.adminPool.Exec(ctx, `DELETE FROM `+members+` WHERE
		(team_id = $1 AND user_public_id = $4) OR (team_id = $2 AND user_public_id = $3)`,
		organization.teams[0].id, organization.teams[1].id, fixture.users.shared, fixture.users.alpha)
	require.NoError(t, err)
	_, err = fixture.adminPool.Exec(ctx, `UPDATE `+members+` SET role = 'team_admin' WHERE
		(team_id = $1 AND user_public_id = $3) OR (team_id = $2 AND user_public_id = $4)`,
		organization.teams[0].id, organization.teams[1].id, fixture.users.shared, fixture.users.alpha)
	require.NoError(t, err)

	application := service.NewResourceApplication(authorization.NewAuthorizer(fixture.resolver, fixture.executor))
	day := time.Now().UTC().Add(8 * 24 * time.Hour)
	start := time.Date(day.Year(), day.Month(), day.Day(), 10, 0, 0, 0, time.UTC)
	resource, err := application.Create(ctx, owner, organization.publicID, service.ResourceWrite{
		Name: "Shared Microscope", Code: "MIC-01", Kind: "instrument", Enabled: true,
		Availability: []service.AvailabilityWindow{{Weekday: int(start.Weekday()), Start: "09:00", End: "17:00"}},
	})
	require.NoError(t, err)

	first, err := application.Request(ctx, owner, organization.publicID, resource.ID,
		organization.teams[0].publicID, start, start.Add(2*time.Hour), "First request")
	require.NoError(t, err)
	second, err := application.Request(ctx, owner, organization.publicID, resource.ID,
		organization.teams[0].publicID, start.Add(time.Hour), start.Add(3*time.Hour), "Second request")
	require.NoError(t, err)

	errorsSeen := make(chan error, 2)
	var group sync.WaitGroup
	for _, requestID := range []uuid.UUID{first.ID, second.ID} {
		group.Add(1)
		go func() {
			defer group.Done()
			_, decideErr := application.Decide(ctx, owner, organization.publicID, requestID, "approved")
			errorsSeen <- decideErr
		}()
	}
	group.Wait()
	close(errorsSeen)
	successes, conflicts := 0, 0
	for decideErr := range errorsSeen {
		if decideErr == nil {
			successes++
		} else if errors.Is(decideErr, service.ErrConflict) {
			conflicts++
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)

	teamRequest, err := application.Request(ctx, member, organization.publicID, resource.ID,
		organization.teams[1].publicID, start.Add(4*time.Hour), start.Add(5*time.Hour), "Team two request")
	require.NoError(t, err)
	_, err = application.Decide(ctx, owner, organization.publicID, teamRequest.ID, "approved")
	require.NoError(t, err)
	bookings, err := application.ListBookings(ctx, member, organization.publicID)
	require.NoError(t, err)
	require.Len(t, bookings, 2)
	visible, redacted := 0, 0
	for _, booking := range bookings {
		if booking.DetailsVisible {
			visible++
			assert.NotNil(t, booking.Purpose)
		} else {
			redacted++
			assert.Nil(t, booking.Purpose)
		}
	}
	assert.Equal(t, 1, visible)
	assert.Equal(t, 1, redacted)
	assert.ErrorIs(t, application.Delete(ctx, owner, organization.publicID, resource.ID), service.ErrConflict)
}
