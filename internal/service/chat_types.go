package service

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	chatEventCreated    = "message.created"
	chatEventDeleted    = "message.deleted"
	maxChatMessageRunes = 4096
)

type ChatSenderView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type ChatMessageView struct {
	ID        uuid.UUID      `json:"id"`
	ChannelID uuid.UUID      `json:"channel_id"`
	Sender    ChatSenderView `json:"sender"`
	ReplyToID *uuid.UUID     `json:"reply_to_id,omitempty"`
	Mentions  []int64        `json:"mentions,omitempty"`
	Message   string         `json:"message"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt *time.Time     `json:"deleted_at,omitempty"`
}

type ChatEvent struct {
	TeamID int64           `json:"team_id"`
	Type   string          `json:"type"`
	Data   ChatMessageView `json:"data"`
}

type ChatPublisher interface {
	PublishChatEvent(context.Context, ChatEvent) error
}

type rosterMap map[uuid.UUID]tenantdb.BoundTeamMember

func newRosterMap(members []tenantdb.BoundTeamMember) rosterMap {
	roster := make(rosterMap, len(members))
	for _, member := range members {
		roster[member.UserPublicID] = member
	}
	return roster
}

func (roster rosterMap) containsAll(ids []uuid.UUID) bool {
	for _, id := range ids {
		if _, ok := roster[id]; !ok {
			return false
		}
	}
	return true
}

func (roster rosterMap) role(id uuid.UUID) string { return roster[id].Role }

func validateChatMessage(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" || utf8.RuneCountInString(message) > maxChatMessageRunes {
		return "", ErrInvalidInput
	}
	return message, nil
}

func boundedChatLimit(limit int32) int32 {
	if limit < 1 || limit > 100 {
		return 100
	}
	return limit
}

func nullableUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func nonNilUUIDs(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

func valueTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func uuidPointer(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func chatView(id, teamID, sender uuid.UUID, reply pgtype.UUID, mentions []uuid.UUID, message string, created time.Time, deleted pgtype.Timestamptz, roster rosterMap) ChatMessageView {
	var deletedAt *time.Time
	if deleted.Valid {
		deletedAt = &deleted.Time
	}
	return ChatMessageView{ID: id, ChannelID: teamID, Sender: ChatSenderView{ID: numericUUID(sender), Name: roster[sender].DisplayName}, ReplyToID: uuidPointer(reply), Mentions: numericUUIDs(mentions), Message: message, CreatedAt: created, DeletedAt: deletedAt}
}

func numericUUIDs(ids []uuid.UUID) []int64 {
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		result = append(result, numericUUID(id))
	}
	return result
}

func numericUUID(id uuid.UUID) int64 {
	value, _ := strconv.ParseInt(strings.ReplaceAll(id.String(), "-", "")[:15], 16, 64)
	return value
}
