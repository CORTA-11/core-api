package v1

import (
	"errors"
	"net/http"
	"strconv"

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
	organizations              service.OrganizationService
	organizationMembers        service.OrganizationMemberService
	teamTasks                  service.TeamTaskService
	invitations                service.InvitationService
	resourceBookings           service.ResourceBookingService
	keys                       service.KeyService
	keyAccess                  service.KeyAccessService
	files                      service.FileService
	chat                       service.ChatService
	documents                  service.DocumentService
	collaborationServiceSecret []byte
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

func routeInt32(request *http.Request, name string) (int32, bool) {
	value, err := strconv.ParseInt(chi.URLParam(request, name), 10, 32)
	return int32(value), err == nil // #nosec G115 -- ParseInt enforces the int32 range.
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
