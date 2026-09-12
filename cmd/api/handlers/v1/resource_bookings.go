package v1

import (
	"net/http"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/google/uuid"
)

type resourceRequestInput struct {
	TeamPublicID uuid.UUID `json:"team_public_id"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Purpose      string    `json:"purpose"`
}

type decisionInput struct {
	Status string `json:"status"`
}

func (handler *ResourceHandler) listResources(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, _, ok := handler.scoped(request, false)
	if !ok || handler.resourceBookings == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	items, err := handler.resourceBookings.List(request.Context(), auth.Principal, orgID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Items []service.ResourceView `json:"items"`
	}{items})
}

func (handler *ResourceHandler) createResource(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, _, ok := handler.scoped(request, false)
	var input service.ResourceWrite
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || handler.resourceBookings == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		handler.problem(writer, request, service.ErrInvalidInput)
		return
	}
	view, err := handler.resourceBookings.Create(request.Context(), auth.Principal, orgID, input)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusCreated, view)
}

func (handler *ResourceHandler) updateResource(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, _, ok := handler.scoped(request, false)
	id, valid := routeUUID(request, "resource_id")
	var input service.ResourcePatch
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || !valid || handler.resourceBookings == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		handler.problem(writer, request, service.ErrInvalidInput)
		return
	}
	view, err := handler.resourceBookings.Update(request.Context(), auth.Principal, orgID, id, input)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, view)
}

func (handler *ResourceHandler) deleteResource(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, _, ok := handler.scoped(request, false)
	id, valid := routeUUID(request, "resource_id")
	if !ok || !valid || handler.resourceBookings == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err := handler.resourceBookings.Delete(request.Context(), auth.Principal, orgID, id); err != nil {
		handler.problem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *ResourceHandler) listBookings(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, _, ok := handler.scoped(request, false)
	if !ok || handler.resourceBookings == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	items, err := handler.resourceBookings.ListBookings(request.Context(), auth.Principal, orgID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Items []service.BookingView `json:"items"`
	}{items})
}

func (handler *ResourceHandler) createResourceRequest(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, _, ok := handler.scoped(request, false)
	resourceID, valid := routeUUID(request, "resource_id")
	var input resourceRequestInput
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || !valid || handler.resourceBookings == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		handler.problem(writer, request, service.ErrInvalidInput)
		return
	}
	view, err := handler.resourceBookings.Request(request.Context(), auth.Principal, orgID, resourceID, input.TeamPublicID, input.StartTime, input.EndTime, input.Purpose)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusCreated, view)
}

func (handler *ResourceHandler) listResourceRequests(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, _, ok := handler.scoped(request, false)
	if !ok || handler.resourceBookings == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	items, err := handler.resourceBookings.ListRequests(request.Context(), auth.Principal, orgID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Items []service.ResourceRequestView `json:"items"`
	}{items})
}

func (handler *ResourceHandler) decideResourceRequest(writer http.ResponseWriter, request *http.Request) {
	auth, orgID, _, ok := handler.scoped(request, false)
	id, valid := routeUUID(request, "request_id")
	var input decisionInput
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || !valid || handler.resourceBookings == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		handler.problem(writer, request, service.ErrInvalidInput)
		return
	}
	view, err := handler.resourceBookings.Decide(request.Context(), auth.Principal, orgID, id, input.Status)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, view)
}
