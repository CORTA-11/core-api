package v1

import (
	"net/http"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/service"
)

type nameRequest struct {
	Name string `json:"name"`
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
