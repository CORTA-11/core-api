// Package apicontract owns the reviewed v1 route metadata and OpenAPI
// conformance helpers. The inventory describes dark routes as well as mounted
// routes; D06 performs the public cutover.
package apicontract

import (
	"net/http"

	"github.com/CORTA-11/core-api/internal/authorization"
)

type AuthenticationPolicy string

const (
	AuthenticationPublic   AuthenticationPolicy = "public"
	AuthenticationRequired AuthenticationPolicy = "required"
	AuthenticationLogout   AuthenticationPolicy = "logout-cookie"
)

type CSRFPolicy string

const (
	CSRFNone     CSRFPolicy = "none"
	CSRFRequired CSRFPolicy = "required"
)

type BodyLimitClass string

const (
	BodyNone     BodyLimitClass = "none"
	BodyAuthJSON BodyLimitClass = "auth-json-4k"
	BodyJSON     BodyLimitClass = "resource-json-16k"
)

type RateLimitClass string

const (
	RateLogin RateLimitClass = "login"
	RateRead  RateLimitClass = "authenticated-read"
	RateWrite RateLimitClass = "authenticated-write"
)

type Route struct {
	Method         string
	Pattern        string
	OperationID    string
	Authentication AuthenticationPolicy
	CSRF           CSRFPolicy
	Permission     authorization.Permission
	BodyLimit      BodyLimitClass
	RateLimit      RateLimitClass
}

var Routes = [...]Route{
	{http.MethodPost, "/api/v1/auth/login", "login", AuthenticationPublic, CSRFNone, "", BodyAuthJSON, RateLogin},
	{http.MethodGet, "/api/v1/auth/session", "getCurrentSession", AuthenticationRequired, CSRFNone, "", BodyNone, RateRead},
	{http.MethodDelete, "/api/v1/auth/session", "logout", AuthenticationLogout, CSRFRequired, "", BodyNone, RateWrite},
	{http.MethodGet, "/api/v1/auth/sessions", "listSessions", AuthenticationRequired, CSRFNone, "", BodyNone, RateRead},
	{http.MethodDelete, "/api/v1/auth/sessions", "revokeAllSessions", AuthenticationRequired, CSRFRequired, "", BodyNone, RateWrite},
	{http.MethodDelete, "/api/v1/auth/sessions/{session_id}", "revokeSession", AuthenticationRequired, CSRFRequired, "", BodyNone, RateWrite},
	{http.MethodPut, "/api/v1/auth/password", "changePassword", AuthenticationRequired, CSRFRequired, "", BodyAuthJSON, RateWrite},
	{http.MethodGet, "/api/v1/orgs", "listOrganizations", AuthenticationRequired, CSRFNone, "", BodyNone, RateRead},
	{http.MethodPost, "/api/v1/orgs", "createOrganization", AuthenticationRequired, CSRFRequired, "", BodyJSON, RateWrite},
	{http.MethodGet, "/api/v1/orgs/{org_id}", "getOrganization", AuthenticationRequired, CSRFNone, authorization.PermissionOrgRead, BodyNone, RateRead},
	{http.MethodPatch, "/api/v1/orgs/{org_id}", "updateOrganization", AuthenticationRequired, CSRFRequired, authorization.PermissionOrgUpdate, BodyJSON, RateWrite},
	{http.MethodDelete, "/api/v1/orgs/{org_id}", "deleteOrganization", AuthenticationRequired, CSRFRequired, authorization.PermissionOrgDelete, BodyNone, RateWrite},
	{http.MethodPost, "/api/v1/orgs/{org_id}/restore", "restoreOrganization", AuthenticationRequired, CSRFRequired, authorization.PermissionOrgRestore, BodyNone, RateWrite},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams", "listTeams", AuthenticationRequired, CSRFNone, authorization.PermissionOrgRead, BodyNone, RateRead},
	{http.MethodPost, "/api/v1/orgs/{org_id}/teams", "createTeam", AuthenticationRequired, CSRFRequired, authorization.PermissionTeamCreate, BodyJSON, RateWrite},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams/{team_id}/tasks", "listTasks", AuthenticationRequired, CSRFNone, authorization.PermissionTaskRead, BodyNone, RateRead},
	{http.MethodPost, "/api/v1/orgs/{org_id}/teams/{team_id}/tasks", "createTask", AuthenticationRequired, CSRFRequired, authorization.PermissionTaskCreate, BodyJSON, RateWrite},
	{http.MethodPatch, "/api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}", "updateTask", AuthenticationRequired, CSRFRequired, authorization.PermissionTaskUpdate, BodyJSON, RateWrite},
	{http.MethodDelete, "/api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}", "deleteTask", AuthenticationRequired, CSRFRequired, authorization.PermissionTaskDelete, BodyNone, RateWrite},
}
