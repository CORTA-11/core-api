package v1

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/CORTA-11/core-api/internal/apicontract"
	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/google/uuid"
)

const (
	editorIDHeader = "X-Synodus-Editor-ID"
)

func (handler *ResourceHandler) loadDocumentState(writer http.ResponseWriter, request *http.Request) {
	editorID, organizationID, teamID, documentID, ok := handler.privateDocumentScope(writer, request)
	if !ok {
		return
	}
	state, err := handler.documents.LoadState(request.Context(), editorID, organizationID, teamID, documentID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, state)
}

func (handler *ResourceHandler) storeDocumentState(writer http.ResponseWriter, request *http.Request) {
	editorID, organizationID, teamID, documentID, ok := handler.privateDocumentScope(writer, request)
	if !ok {
		return
	}
	var input service.DocumentStateWrite
	if err := httpx.DecodeJSON(request, &input, httpx.BodyLimitBytes(apicontract.BodyCollaborationJSON)); err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	state, err := handler.documents.StoreState(request.Context(), editorID, organizationID, teamID, documentID, input)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, state)
}

func (handler *ResourceHandler) privateDocumentScope(
	writer http.ResponseWriter, request *http.Request,
) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	if handler.documents == nil || !handler.validCollaborationCredential(request.Header.Get("Authorization")) {
		writeProblem(writer, request, httpx.ProblemUnauthenticated, nil)
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	editorID, editorErr := uuid.Parse(request.Header.Get(editorIDHeader))
	organizationID, validOrganization := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	documentID, validDocument := routeUUID(request, "document_id")
	if editorErr != nil || editorID == uuid.Nil {
		writeProblem(writer, request, httpx.ProblemInvalidRequest, service.ErrInvalidInput)
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	if !validOrganization || !validTeam || !validDocument {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return editorID, organizationID, teamID, documentID, true
}

func (handler *ResourceHandler) validCollaborationCredential(authorizationHeader string) bool {
	provided, found := strings.CutPrefix(authorizationHeader, "Bearer ")
	if !found || provided == "" || len(handler.collaborationServiceSecret) < 32 {
		return false
	}
	want := sha256.Sum256(handler.collaborationServiceSecret)
	got := sha256.Sum256([]byte(provided))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}
