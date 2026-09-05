package service

import (
	"strings"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateChatMessageTrimsAndBoundsText(t *testing.T) {
	t.Parallel()
	message, err := validateChatMessage("  ship it  ")
	require.NoError(t, err)
	assert.Equal(t, "ship it", message)

	for _, input := range []string{"", "  \t\n ", strings.Repeat("a", maxChatMessageRunes+1)} {
		_, err := validateChatMessage(input)
		assert.ErrorIs(t, err, ErrInvalidInput)
	}
}

func TestChatViewMapsUUIDsToFrontendShape(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 5, 8, 30, 0, 0, time.UTC)
	id := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	teamID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	sender := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	reply := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	mention := uuid.MustParse("55555555-5555-4555-8555-555555555555")

	view := chatView(id, teamID, sender, pgtype.UUID{Bytes: reply, Valid: true}, []uuid.UUID{mention},
		"hello", now, pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true}, rosterMap{
			sender:  {UserPublicID: sender, DisplayName: "Ada", Role: "team_admin"},
			mention: tenantdb.BoundTeamMember{UserPublicID: mention, DisplayName: "Grace"},
		})

	assert.Equal(t, id, view.ID)
	assert.Equal(t, teamID, view.ChannelID)
	assert.Equal(t, "Ada", view.Sender.Name)
	assert.Equal(t, numericUUID(sender), view.Sender.ID)
	require.NotNil(t, view.ReplyToID)
	assert.Equal(t, reply, *view.ReplyToID)
	assert.Equal(t, []int64{numericUUID(mention)}, view.Mentions)
	require.NotNil(t, view.DeletedAt)
	assert.Equal(t, now.Add(time.Minute), *view.DeletedAt)
}

func TestNonNilUUIDsNormalizesOmittedMentions(t *testing.T) {
	t.Parallel()
	require.Empty(t, nonNilUUIDs(nil))
	assert.NotNil(t, nonNilUUIDs(nil))

	mention := uuid.New()
	assert.Equal(t, []uuid.UUID{mention}, nonNilUUIDs([]uuid.UUID{mention}))
}
