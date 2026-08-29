package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maximumResourceBodyBytes = 64 << 10

type ResourceHandler struct {
	organizations       OrganizationService
	organizationMembers OrganizationMemberService
	teamTasks           TeamTaskService
	invitations         InvitationService
	resourceBookings    ResourceBookingService
	keys                KeyService
	files               FileService
}

type nameRequest struct {
	Name string `json:"name"`
}

type createTeamRequest struct {
	Name        string `json:"name"`
	LeaderEmail string `json:"leader_email"`
}

type taskRequest struct {
	Description string     `json:"description"`
	Status      string     `json:"status"`
	AssigneeID  *uuid.UUID `json:"assignee_id"`
	// assigneeSet separates an omitted field (keep current) from an explicit
	// null (unassign); encoding/json cannot express that on a plain pointer.
	assigneeSet bool
}

func (task *taskRequest) UnmarshalJSON(data []byte) error {
	var raw struct {
		Description string          `json:"description"`
		Status      string          `json:"status"`
		AssigneeID  json.RawMessage `json:"assignee_id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	task.Description = raw.Description
	task.Status = raw.Status
	if raw.AssigneeID == nil {
		return nil
	}
	task.assigneeSet = true
	if bytes.Equal(bytes.TrimSpace(raw.AssigneeID), []byte("null")) {
		return nil
	}
	var value uuid.UUID
	if err := json.Unmarshal(raw.AssigneeID, &value); err != nil {
		return err
	}
	task.AssigneeID = &value
	return nil
}

type invitationRequest struct {
	Email string `json:"email"`
}

func (handler *ResourceHandler) listOrganizationMembers(writer http.ResponseWriter, request *http.Request) {
	auth, ok := authenticationFrom(request)
	orgID, valid := routeUUID(request, "org_id")
	if !ok || !valid || handler.organizationMembers == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	items, err := handler.organizationMembers.ListMembers(request.Context(), auth.Principal, orgID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Items []service.OrganizationMemberView `json:"items"`
	}{items})
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

func (handler *ResourceHandler) listOrganizations(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	parameters, err := pagination.Parse(request.URL.Query())
	if !ok || err != nil || handler.organizations == nil {
		handler.problem(writer, request, firstError(err, service.ErrInvalidInput))
		return
	}
	page, err := handler.organizations.List(request.Context(), authentication.Principal, parameters)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, page)
}

func (handler *ResourceHandler) createOrganization(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	var input nameRequest
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || err != nil || handler.organizations == nil {
		handler.problem(writer, request, firstError(err, service.ErrInvalidInput))
		return
	}
	organization, err := handler.organizations.Create(request.Context(), authentication.Principal, input.Name)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusCreated, organization)
}

func (handler *ResourceHandler) getOrganization(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	organizationID, validID := routeUUID(request, "org_id")
	if !ok || !validID || handler.organizations == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	organization, err := handler.organizations.Get(request.Context(), authentication.Principal, organizationID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, organization)
}

func (handler *ResourceHandler) updateOrganization(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	organizationID, validID := routeUUID(request, "org_id")
	var input nameRequest
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || !validID || handler.organizations == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	organization, err := handler.organizations.Update(request.Context(), authentication.Principal, organizationID, input.Name)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, organization)
}

func (handler *ResourceHandler) deleteOrganization(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	organizationID, validID := routeUUID(request, "org_id")
	if !ok || !validID || handler.organizations == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err := handler.organizations.Delete(request.Context(), authentication.Principal, organizationID); err != nil {
		handler.problem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *ResourceHandler) restoreOrganization(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	organizationID, validID := routeUUID(request, "org_id")
	if !ok || !validID || handler.organizations == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	organization, err := handler.organizations.Restore(request.Context(), authentication.Principal, organizationID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, organization)
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

func (handler *ResourceHandler) listTasks(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	parameters, err := pagination.Parse(request.URL.Query())
	if !ok || err != nil || handler.teamTasks == nil {
		handler.problem(writer, request, firstError(err, authorization.ErrResourceNotFound))
		return
	}
	page, err := handler.teamTasks.ListTasks(request.Context(), authentication.Principal, organizationID, teamID, parameters)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, page)
}

func (handler *ResourceHandler) createTask(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	input, err := decodeTask(request)
	if !ok || handler.teamTasks == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	task, err := handler.teamTasks.CreateTask(request.Context(), authentication.Principal,
		organizationID, teamID, input.Description, input.Status, input.AssigneeID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusCreated, task)
}

func (handler *ResourceHandler) updateTask(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	taskID, validTask := routeUUID(request, "task_id")
	input, err := decodeTask(request)
	if !ok || !validTask || handler.teamTasks == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	task, err := handler.teamTasks.UpdateTask(request.Context(), authentication.Principal,
		organizationID, teamID, taskID, input.Description, input.Status, input.AssigneeID, input.assigneeSet)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, task)
}

func (handler *ResourceHandler) deleteTask(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	taskID, validTask := routeUUID(request, "task_id")
	if !ok || !validTask || handler.teamTasks == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err := handler.teamTasks.DeleteTask(request.Context(), authentication.Principal,
		organizationID, teamID, taskID); err != nil {
		handler.problem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *ResourceHandler) scoped(request *http.Request, requireTeam bool) (session.Authentication, uuid.UUID, uuid.UUID, bool) {
	authentication, ok := authenticationFrom(request)
	organizationID, validOrganization := routeUUID(request, "org_id")
	if !ok || !validOrganization {
		return session.Authentication{}, uuid.Nil, uuid.Nil, false
	}
	if !requireTeam {
		return authentication, organizationID, uuid.Nil, true
	}
	teamID, validTeam := routeUUID(request, "team_id")
	return authentication, organizationID, teamID, validTeam
}

func routeUUID(request *http.Request, name string) (uuid.UUID, bool) {
	value, err := uuid.Parse(chi.URLParam(request, name))
	return value, err == nil && value != uuid.Nil
}

func decodeTask(request *http.Request) (taskRequest, error) {
	var input taskRequest
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	return input, err
}

func (handler *ResourceHandler) problem(writer http.ResponseWriter, request *http.Request, err error) {
	kind := httpx.ProblemDependencyUnavailable
	switch {
	case errors.Is(err, service.ErrInvalidInput), errors.Is(err, pagination.ErrInvalidParameters):
		kind = httpx.ProblemInvalidRequest
	case errors.Is(err, authorization.ErrUnauthenticated):
		kind = httpx.ProblemUnauthenticated
	case errors.Is(err, authorization.ErrOperationDenied):
		kind = httpx.ProblemForbidden
	case errors.Is(err, authorization.ErrResourceNotFound):
		kind = httpx.ProblemNotFound
	case errors.Is(err, service.ErrConflict):
		kind = httpx.ProblemConflict
	}
	writeProblem(writer, request, kind, err)
}

func firstError(preferred, fallback error) error {
	if preferred != nil {
		return preferred
	}
	return fallback
}
