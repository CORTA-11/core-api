package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ChatApplication struct {
	authorizer applicationAuthorizer
	publisher  ChatPublisher
}

func NewChatApplication(authorizer applicationAuthorizer, publisher ChatPublisher) *ChatApplication {
	return &ChatApplication{authorizer: authorizer, publisher: publisher}
}

func (application *ChatApplication) ListMessages(ctx context.Context, principal session.Principal, orgID, teamID uuid.UUID, limit int32, before *time.Time) ([]ChatMessageView, error) {
	var result []ChatMessageView
	err := application.inTeam(ctx, principal, orgID, teamID, func(queries *tenantdb.Queries, roster rosterMap, _ string) error {
		rows, err := queries.ListChatMessages(ctx, tenantdb.ListChatMessagesParams{BeforeSet: before != nil, BeforeCreatedAt: valueTime(before), Limit: boundedChatLimit(limit)})
		if err != nil {
			return err
		}
		slices.Reverse(rows)
		result = make([]ChatMessageView, 0, len(rows))
		for _, row := range rows {
			result = append(result, chatView(row.PublicID, teamID, row.SenderUserPublicID, row.ReplyToPublicID, row.Mentions, row.Message, row.CreatedAt, row.DeletedAt, roster))
		}
		return nil
	})
	return result, err
}

func (application *ChatApplication) SendMessage(ctx context.Context, principal session.Principal, orgID, teamID uuid.UUID, message string, replyTo *uuid.UUID, mentions []uuid.UUID) (ChatMessageView, error) {
	message, err := validateChatMessage(message)
	if err != nil {
		return ChatMessageView{}, err
	}
	var result ChatMessageView
	var eventTeamID int64
	err = application.inTeam(ctx, principal, orgID, teamID, func(queries *tenantdb.Queries, roster rosterMap, _ string) error {
		if !roster.containsAll(mentions) {
			return ErrInvalidInput
		}
		row, err := queries.CreateChatMessage(ctx, tenantdb.CreateChatMessageParams{ReplyToPublicID: nullableUUID(replyTo), Mentions: mentions, Message: message})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidInput
		}
		if err != nil {
			return err
		}
		eventTeamID = row.TeamID
		result = chatView(row.PublicID, teamID, row.SenderUserPublicID, row.ReplyToPublicID, row.Mentions, row.Message, row.CreatedAt, row.DeletedAt, roster)
		return nil
	})
	if err == nil {
		application.publish(ctx, ChatEvent{TeamID: eventTeamID, Type: chatEventCreated, Data: result})
	}
	return result, err
}

func (application *ChatApplication) DeleteMessage(ctx context.Context, principal session.Principal, orgID, teamID, messageID uuid.UUID) (ChatMessageView, error) {
	var result ChatMessageView
	var eventTeamID int64
	err := application.inTeam(ctx, principal, orgID, teamID, func(queries *tenantdb.Queries, roster rosterMap, role string) error {
		current, err := queries.GetChatMessage(ctx, messageID)
		if errors.Is(err, pgx.ErrNoRows) {
			return authorization.ErrResourceNotFound
		}
		if err != nil {
			return err
		}
		if current.SenderUserPublicID != principal.UserID && role != string(authorization.TeamRoleAdmin) && role != string(authorization.TeamRoleResearchLead) {
			return authorization.ErrOperationDenied
		}
		row, err := queries.SoftDeleteChatMessage(ctx, messageID)
		if errors.Is(err, pgx.ErrNoRows) {
			return authorization.ErrResourceNotFound
		}
		if err != nil {
			return err
		}
		eventTeamID = row.TeamID
		result = chatView(row.PublicID, teamID, row.SenderUserPublicID, row.ReplyToPublicID, row.Mentions, row.Message, row.CreatedAt, row.DeletedAt, roster)
		return nil
	})
	if err == nil {
		application.publish(ctx, ChatEvent{TeamID: eventTeamID, Type: chatEventDeleted, Data: result})
	}
	return result, err
}

func (application *ChatApplication) inTeam(ctx context.Context, principal session.Principal, orgID, teamID uuid.UUID, callback func(*tenantdb.Queries, rosterMap, string) error) error {
	if application == nil || application.authorizer == nil {
		return authorization.ErrResourceNotFound
	}
	return application.authorizer.WithinTeam(ctx, principal, orgID, teamID, authorization.PermissionRealtimeConnect, func(queries *tenantdb.Queries) error {
		members, err := queries.ListBoundTeamMembers(ctx, 10000)
		if err != nil {
			return err
		}
		roster := newRosterMap(members)
		return callback(queries, roster, roster.role(principal.UserID))
	})
}

func (application *ChatApplication) publish(ctx context.Context, event ChatEvent) {
	if application.publisher != nil {
		_ = application.publisher.PublishChatEvent(ctx, event)
	}
}
