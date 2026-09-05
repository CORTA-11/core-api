package v1

import (
	"net/http"
	"strconv"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/google/uuid"
)

type chatRequest struct {
	Message   string      `json:"message"`
	ReplyToID *uuid.UUID  `json:"reply_to_id"`
	Mentions  []uuid.UUID `json:"mentions"`
}

func (handler *ResourceHandler) listChatMessages(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, teamID, ok := handler.scoped(request, true)
	limit := int32(100)
	if raw := request.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil {
			handler.problem(writer, request, service.ErrInvalidInput)
			return
		}
		limit = int32(parsed)
	}
	before, err := parseOptionalTime(request.URL.Query().Get("before"))
	if !ok || err != nil || handler.chat == nil {
		handler.problem(writer, request, firstError(err, authorization.ErrResourceNotFound))
		return
	}
	messages, err := handler.chat.ListMessages(request.Context(), auth.Principal, orgID, teamID, limit, before)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Messages []service.ChatMessageView `json:"messages"`
	}{messages})
}

func (handler *ResourceHandler) createChatMessage(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, teamID, ok := handler.scoped(request, true)
	var input chatRequest
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || handler.chat == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	message, err := handler.chat.SendMessage(request.Context(), auth.Principal, orgID, teamID, input.Message, input.ReplyToID, input.Mentions)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusCreated, message)
}

func (handler *ResourceHandler) deleteChatMessage(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, teamID, ok := handler.scoped(request, true)
	messageID, validMessage := routeUUID(request, "message_id")
	if !ok || !validMessage || handler.chat == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	message, err := handler.chat.DeleteMessage(request.Context(), auth.Principal, orgID, teamID, messageID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, message)
}

func (handler *ResourceHandler) issueChatSocketTicket(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, teamID, ok := handler.scoped(request, true)
	if !ok || handler.chat == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	ticket, err := handler.chat.IssueSocketTicket(request.Context(), auth.Principal, orgID, teamID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Token string `json:"token"`
	}{ticket})
}

func parseOptionalTime(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, service.ErrInvalidInput
	}
	return &value, nil
}
