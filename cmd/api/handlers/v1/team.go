package v1

import (
	"net/http"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/service"
)

type createTeamRequest struct {
	Name        string `json:"name"`
	LeaderEmail string `json:"leader_email"`
}

func (handler *ResourceHandler) listTeams(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, _, ok := handler.scoped(request, false)
	parameters, err := pagination.Parse(request.URL.Query())
	if !ok || err != nil || handler.teamTasks == nil {
		handler.problem(writer, request, firstError(err, authorization.ErrResourceNotFound))
		return
	}
	page, err := handler.teamTasks.ListTeams(request.Context(), authentication.Principal, organizationID, parameters)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, page)
}

func (handler *ResourceHandler) createTeam(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, _, ok := handler.scoped(request, false)
	var input createTeamRequest
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || handler.teamTasks == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	team, err := handler.teamTasks.CreateTeam(request.Context(), authentication.Principal, organizationID, input.Name, input.LeaderEmail)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusCreated, team)
}

func (handler *ResourceHandler) listTeamMembers(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, teamID, ok := handler.scoped(request, true)
	if !ok || handler.teamTasks == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	items, err := handler.teamTasks.ListTeamMembers(request.Context(), auth.Principal, orgID, teamID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Items []service.TeamMemberView `json:"items"`
	}{items})
}

func (handler *ResourceHandler) addTeamMember(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, teamID, ok := handler.scoped(request, true)
	var input invitationRequest
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || handler.teamTasks == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		handler.problem(writer, request, service.ErrInvalidInput)
		return
	}
	member, err := handler.teamTasks.AddTeamMember(request.Context(), auth.Principal, orgID, teamID, input.Email)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusCreated, member)
}
