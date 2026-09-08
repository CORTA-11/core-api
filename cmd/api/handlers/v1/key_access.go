// Key access requests let a member who joined after a team-key version was
// created ask the leader for permission to read files sealed under it.
package v1

import (
	"net/http"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/service"
)

// createKeyAccessRequest creates the caller's pending key access request.
func (handler *ResourceHandler) createKeyAccessRequest(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	if !ok || !validOrg || !validTeam || handler.keyAccess == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}

	view, err := handler.keyAccess.CreateKeyAccessRequest(request.Context(), authentication.Principal, orgID, teamID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}

	_ = httpx.WriteJSON(writer, http.StatusCreated, view)
}

// listKeyAccessRequests lists a team's key access requests.
func (handler *ResourceHandler) listKeyAccessRequests(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	if !ok || !validOrg || !validTeam || handler.keyAccess == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}

	views, err := handler.keyAccess.ListKeyAccessRequests(request.Context(), authentication.Principal, orgID, teamID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}

	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Items []service.KeyAccessRequestView `json:"items"`
	}{views})
}

// approveKeyAccessRequest approves a pending key access request.
func (handler *ResourceHandler) approveKeyAccessRequest(writer http.ResponseWriter, request *http.Request) {
	handler.decideKeyAccessRequest(writer, request, "granted")
}

// denyKeyAccessRequest denies a pending key access request.
func (handler *ResourceHandler) denyKeyAccessRequest(writer http.ResponseWriter, request *http.Request) {
	handler.decideKeyAccessRequest(writer, request, "denied")
}

func (handler *ResourceHandler) decideKeyAccessRequest(writer http.ResponseWriter, request *http.Request, status string) {
	authentication, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	requestID, validRequest := routeUUID(request, "request_id")
	if !ok || !validOrg || !validTeam || !validRequest || handler.keyAccess == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}

	view, err := handler.keyAccess.DecideKeyAccessRequest(request.Context(), authentication.Principal, orgID, teamID, requestID, status)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}

	_ = httpx.WriteJSON(writer, http.StatusOK, view)
}
