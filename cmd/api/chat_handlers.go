package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CORTA-11/core-api/internal/auth"
	"github.com/CORTA-11/core-api/internal/realtime"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type chatSender struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

type chatMessageResponse struct {
	ID        string     `json:"id"`
	ChannelID string     `json:"channel_id"`
	Sender    chatSender `json:"sender"`
	ReplyToID *string    `json:"reply_to_id,omitempty"`
	Message   string     `json:"message"`
	CreatedAt string     `json:"created_at"`
	DeletedAt *string    `json:"deleted_at,omitempty"`
}

type createChatMessageRequest struct {
	Message   string  `json:"message"`
	ReplyToID *string `json:"reply_to_id"`
}

func (router *Router) listChatMessages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _, channel, ok := router.requireTeamChatAccess(w, r)
		if !ok {
			return
		}

		limit := int32(50)
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := parseOrgIDParam(raw)
			if err != nil || parsed < 1 || parsed > 100 {
				writeError(w, http.StatusBadRequest, "limit must be between 1 and 100")
				return
			}
			limit = int32(parsed)
		}

		before := pgtype.UUID{}
		if raw := r.URL.Query().Get("before"); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid before cursor")
				return
			}
			before = pgtype.UUID{Bytes: id, Valid: true}
		}

		rows, err := router.queries.ListChatMessages(r.Context(), repository.ListChatMessagesParams{
			ChannelID:    channel.ID,
			Before:       before,
			MessageLimit: limit,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load messages")
			return
		}

		out := make([]chatMessageResponse, 0, len(rows))
		for i := len(rows) - 1; i >= 0; i-- {
			out = append(out, mapListMessage(rows[i]))
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"messages": out,
		})
	}
}

func (router *Router) createChatMessage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, membership, channel, ok := router.requireTeamChatAccess(w, r)
		if !ok {
			return
		}

		var req createChatMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request payload")
			return
		}

		req.Message = strings.TrimSpace(req.Message)
		if req.Message == "" {
			writeError(w, http.StatusBadRequest, "message is required")
			return
		}
		if len(req.Message) > 4000 {
			writeError(w, http.StatusBadRequest, "message is too long")
			return
		}

		replyTo := pgtype.UUID{}
		if req.ReplyToID != nil && strings.TrimSpace(*req.ReplyToID) != "" {
			id, err := uuid.Parse(strings.TrimSpace(*req.ReplyToID))
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid reply_to_id")
				return
			}
			parent, err := router.queries.GetChatMessageByID(r.Context(), id)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeError(w, http.StatusBadRequest, "reply target not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to validate reply target")
				return
			}
			if parent.ChannelID != channel.ID {
				writeError(w, http.StatusBadRequest, "reply target is in another channel")
				return
			}
			if parent.DeletedAt.Valid {
				writeError(w, http.StatusBadRequest, "cannot reply to a deleted message")
				return
			}
			replyTo = pgtype.UUID{Bytes: id, Valid: true}
		}

		msg, err := router.queries.CreateChatMessage(r.Context(), repository.CreateChatMessageParams{
			ChannelID: channel.ID,
			SenderID:  claims.UserID,
			ReplyToID: replyTo,
			Message:   req.Message,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to send message")
			return
		}

		user, err := router.queries.GetUserByID(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load sender")
			return
		}

		response := chatMessageResponse{
			ID:        msg.ID.String(),
			ChannelID: msg.ChannelID.String(),
			Sender: chatSender{
				ID:        user.ID,
				Name:      user.Name,
				AvatarURL: textPtr(user.AvatarUrl),
			},
			ReplyToID: uuidPtr(msg.ReplyToID),
			Message:   msg.Message,
			CreatedAt: msg.CreatedAt.UTC().Format(time.RFC3339Nano),
			DeletedAt: nil,
		}

		router.publisher.Publish(r.Context(), realtime.Event{
			TeamID: membership.TeamID,
			Type:   "message.created",
			Data:   response,
		})

		writeJSON(w, http.StatusCreated, response)
	}
}

func (router *Router) deleteChatMessage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, membership, channel, ok := router.requireTeamChatAccess(w, r)
		if !ok {
			return
		}

		messageID, err := uuid.Parse(chi.URLParam(r, "messageId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid message id")
			return
		}

		msg, err := router.queries.GetChatMessageByID(r.Context(), messageID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "message not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to load message")
			return
		}

		if msg.ChannelID != channel.ID {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}

		if msg.DeletedAt.Valid {
			writeError(w, http.StatusConflict, "message already deleted")
			return
		}

		isLeader := membership.Role == repository.TeamRoleTEAMLEADER
		if msg.SenderID != claims.UserID && !isLeader {
			writeError(w, http.StatusForbidden, "forbidden: cannot delete this message")
			return
		}

		deleted, err := router.queries.SoftDeleteChatMessage(r.Context(), messageID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusConflict, "message already deleted")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to delete message")
			return
		}

		payload := map[string]any{
			"id":         deleted.ID.String(),
			"channel_id": deleted.ChannelID.String(),
			"deleted_at": timestamptzPtr(deleted.DeletedAt),
		}

		router.publisher.Publish(r.Context(), realtime.Event{
			TeamID: membership.TeamID,
			Type:   "message.deleted",
			Data:   payload,
		})

		writeJSON(w, http.StatusOK, payload)
	}
}

func (router *Router) requireTeamChatAccess(w http.ResponseWriter, r *http.Request) (
	*auth.CustomClaims,
	*auth.TeamMembership,
	repository.Channel,
	bool,
) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return nil, nil, repository.Channel{}, false
	}

	// Org admins manage teams/members only — team chat is for allocated members.
	if claims.OrgRole == "ORG_ADMIN" {
		writeError(w, http.StatusForbidden, "forbidden: organization admins cannot access team chat")
		return nil, nil, repository.Channel{}, false
	}

	teamPublicID, err := uuid.Parse(chi.URLParam(r, "teamPublicId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid team id")
		return nil, nil, repository.Channel{}, false
	}

	membership, err := loadTeamMembership(r, router.queries, teamPublicID, claims.UserID)
	if err != nil {
		if errors.Is(err, errTeamNotFound) {
			writeError(w, http.StatusNotFound, "team not found")
			return nil, nil, repository.Channel{}, false
		}
		if errors.Is(err, errNotTeamMember) {
			writeError(w, http.StatusForbidden, "forbidden: not a team member")
			return nil, nil, repository.Channel{}, false
		}
		writeError(w, http.StatusInternalServerError, "failed to load team membership")
		return nil, nil, repository.Channel{}, false
	}

	if membership.OrgID != claims.OrgID {
		writeError(w, http.StatusForbidden, "forbidden: wrong organization")
		return nil, nil, repository.Channel{}, false
	}

	channel, err := router.queries.GetDefaultChannelByTeamID(r.Context(), membership.TeamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "team chat channel not found")
			return nil, nil, repository.Channel{}, false
		}
		writeError(w, http.StatusInternalServerError, "failed to load chat channel")
		return nil, nil, repository.Channel{}, false
	}

	return claims, membership, channel, true
}

func mapListMessage(row repository.ListChatMessagesRow) chatMessageResponse {
	return chatMessageResponse{
		ID:        row.ID.String(),
		ChannelID: row.ChannelID.String(),
		Sender: chatSender{
			ID:        row.SenderID,
			Name:      row.SenderName,
			AvatarURL: textPtr(row.SenderAvatarUrl),
		},
		ReplyToID: uuidPtr(row.ReplyToID),
		Message:   row.Message,
		CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339Nano),
		DeletedAt: timestamptzPtr(row.DeletedAt),
	}
}
