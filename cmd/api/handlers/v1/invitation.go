package v1

import (
	"net/http"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/service"
)

type invitationRequest struct {
	Email string `json:"email"`
}

func invitationToken(request *http.Request) (string, bool) {
	values := request.Header.Values("X-Invitation-Token")
	return first(values), len(values) == 1 && values[0] != ""
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (handler *ResourceHandler) listInvitations(writer http.ResponseWriter, request *http.Request) {
	auth, ok := authenticationFrom(request)
	orgID, valid := routeUUID(request, "org_id")
	if !ok || !valid || handler.invitations == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	items, err := handler.invitations.List(request.Context(), auth.Principal, orgID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Items []service.InvitationView `json:"items"`
	}{items})
}

func (handler *ResourceHandler) createInvitation(writer http.ResponseWriter, request *http.Request) {
	auth, ok := authenticationFrom(request)
	orgID, valid := routeUUID(request, "org_id")
	var input invitationRequest
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || !valid || handler.invitations == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		handler.problem(writer, request, service.ErrInvalidInput)
		return
	}
	created, err := handler.invitations.Create(request.Context(), auth.Principal, orgID, input.Email)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusCreated, created)
}

func (handler *ResourceHandler) revokeInvitation(writer http.ResponseWriter, request *http.Request) {
	auth, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	id, validID := routeUUID(request, "invitation_id")
	if !ok || !validOrg || !validID || handler.invitations == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err := handler.invitations.Revoke(request.Context(), auth.Principal, orgID, id); err != nil {
		handler.problem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *ResourceHandler) previewInvitation(writer http.ResponseWriter, request *http.Request) {
	token, ok := invitationToken(request)
	if !ok || handler.invitations == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	preview, err := handler.invitations.Preview(request.Context(), token)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, preview)
}

func (handler *ResourceHandler) acceptInvitation(writer http.ResponseWriter, request *http.Request) {
	handler.consumeInvitation(writer, request, true)
}

func (handler *ResourceHandler) declineInvitation(writer http.ResponseWriter, request *http.Request) {
	handler.consumeInvitation(writer, request, false)
}

func (handler *ResourceHandler) consumeInvitation(writer http.ResponseWriter, request *http.Request, accept bool) {
	auth, authenticated := authenticationFrom(request)
	token, valid := invitationToken(request)
	if !authenticated || !valid || handler.invitations == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err := handler.invitations.Consume(request.Context(), auth.Principal, token, accept); err != nil {
		handler.problem(writer, request, err)
		return
	}
	if accept {
		_ = httpx.WriteJSON(writer, http.StatusOK, struct {
			Accepted bool `json:"accepted"`
		}{true})
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
