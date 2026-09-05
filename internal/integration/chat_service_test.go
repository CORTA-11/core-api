//go:build isolation

package integration_test

import (
	"context"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chatEventSink struct {
	events []service.ChatEvent
}

func (sink *chatEventSink) PublishChatEvent(_ context.Context, event service.ChatEvent) error {
	sink.events = append(sink.events, event)
	return nil
}

func TestChatServicePersistsAndPublishesTeamMessages(t *testing.T) {
	fixture := newTenantBoundaryFixture(t)
	ctx := context.Background()
	alpha := fixture.orgs[0]
	team := alpha.teams[0]
	principal := session.Principal{UserID: fixture.users.shared, SessionID: uuid.New()}
	events := &chatEventSink{}
	chat := service.NewChatApplication(
		authorization.NewAuthorizer(fixture.resolver, fixture.executor),
		events,
		[]byte("integration-socket-ticket-secret-value"),
	)

	created, err := chat.SendMessage(ctx, principal, alpha.publicID, team.publicID, " hello chat ", nil, []uuid.UUID{fixture.users.alpha})
	require.NoError(t, err)
	assert.Equal(t, "hello chat", created.Message)
	assert.Equal(t, team.publicID, created.ChannelID)
	require.Len(t, events.events, 1)
	assert.Equal(t, int64(team.id), events.events[0].TeamID)
	assert.Equal(t, "message.created", events.events[0].Type)

	messages, err := chat.ListMessages(ctx, principal, alpha.publicID, team.publicID, 100, nil)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, created.ID, messages[0].ID)

	deleted, err := chat.DeleteMessage(ctx, principal, alpha.publicID, team.publicID, created.ID)
	require.NoError(t, err)
	assert.NotNil(t, deleted.DeletedAt)
	require.Len(t, events.events, 2)
	assert.Equal(t, "message.deleted", events.events[1].Type)
}
