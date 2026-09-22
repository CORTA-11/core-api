package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/push"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PushNotifier interface {
	SendMulticast(ctx context.Context, tokens []string, payload push.NotificationPayload) error
}

type DeviceTokenFinder interface {
	GetDeviceTokensForUsers(ctx context.Context, userIds []uuid.UUID) ([]publicdb.GetDeviceTokensForUsersRow, error)
}

type ChatApplication struct {
	authorizer   applicationAuthorizer
	publisher    ChatPublisher
	ticketSecret []byte
	deviceFinder DeviceTokenFinder
	pushNotifier PushNotifier
}

func NewChatApplication(authorizer applicationAuthorizer, publisher ChatPublisher, ticketSecret ...[]byte) *ChatApplication {
	app := &ChatApplication{authorizer: authorizer, publisher: publisher}
	if len(ticketSecret) > 0 {
		app.ticketSecret = append([]byte(nil), ticketSecret[0]...)
	}
	return app
}

func (application *ChatApplication) WithPushNotification(deviceFinder DeviceTokenFinder, pushNotifier PushNotifier) *ChatApplication {
	application.deviceFinder = deviceFinder
	application.pushNotifier = pushNotifier
	return application
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
	var senderName string
	var recipientUserIDs []uuid.UUID
	err = application.inTeam(ctx, principal, orgID, teamID, func(queries *tenantdb.Queries, roster rosterMap, _ string) error {
		if !roster.containsAll(mentions) {
			return ErrInvalidInput
		}
		row, err := queries.CreateChatMessage(ctx, tenantdb.CreateChatMessageParams{ReplyToPublicID: nullableUUID(replyTo), Mentions: nonNilUUIDs(mentions), Message: message})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidInput
		}
		if err != nil {
			return err
		}
		eventTeamID = row.TeamID
		result = chatView(row.PublicID, teamID, row.SenderUserPublicID, row.ReplyToPublicID, row.Mentions, row.Message, row.CreatedAt, row.DeletedAt, roster)

		sender := roster[principal.UserID]
		if sender.DisplayName != "" {
			senderName = sender.DisplayName
		} else if sender.Email != "" {
			senderName = sender.Email
		} else {
			senderName = "Team Member"
		}

		for memberID := range roster {
			if memberID != principal.UserID {
				recipientUserIDs = append(recipientUserIDs, memberID)
			}
		}
		return nil
	})
	if err == nil {
		application.publish(ctx, ChatEvent{TeamID: eventTeamID, Type: chatEventCreated, Data: result})
		if application.pushNotifier != nil && application.deviceFinder != nil && len(recipientUserIDs) > 0 {
			go application.dispatchPushNotification(senderName, message, teamID, orgID, recipientUserIDs)
		}
	}
	return result, err
}

func (application *ChatApplication) dispatchPushNotification(senderName, message string, teamID, orgID uuid.UUID, recipientIDs []uuid.UUID) {
	bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := application.deviceFinder.GetDeviceTokensForUsers(bgCtx, recipientIDs)
	if err != nil || len(rows) == 0 {
		return
	}

	tokens := make([]string, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, row.Token)
	}

	title := senderName
	body := message
	if len(body) > 100 {
		body = body[:97] + "..."
	}

	_ = application.pushNotifier.SendMulticast(bgCtx, tokens, push.NotificationPayload{
		Title: title,
		Body:  body,
		Data: map[string]string{
			"team_id": teamID.String(),
			"org_id":  orgID.String(),
			"type":    "chat",
		},
	})
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
