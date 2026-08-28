package v1

import (
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
	organizations OrganizationService
	teamTasks     TeamTaskService
}

type nameRequest struct {
	Name string `json:"name"`
}

type taskRequest struct {
	Description string `json:"description"`
	Status      string `json:"status"`
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
	var input nameRequest
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || handler.teamTasks == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	team, err := handler.teamTasks.CreateTeam(request.Context(), authentication.Principal, organizationID, input.Name)
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
		organizationID, teamID, input.Description, input.Status)
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
		organizationID, teamID, taskID, input.Description, input.Status)
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
