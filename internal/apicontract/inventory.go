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
	BodyJSON     BodyLimitClass = "resource-json-64k"
	BodyFile     BodyLimitClass = "file-multipart-10m"
)

type RateLimitClass string

const (
	RateNone           RateLimitClass = "none"
	RateLogin          RateLimitClass = "login"
	RateRegistration   RateLimitClass = "registration"
	RateAdministrative RateLimitClass = "administrative"
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
	{http.MethodPost, "/api/v1/auth/register", "register", AuthenticationPublic, CSRFNone, "", BodyAuthJSON, RateRegistration},
	{http.MethodPost, "/api/v1/auth/login", "login", AuthenticationPublic, CSRFNone, "", BodyAuthJSON, RateLogin},
	{http.MethodGet, "/api/v1/auth/session", "getCurrentSession", AuthenticationRequired, CSRFNone, "", BodyNone, RateNone},
	{http.MethodDelete, "/api/v1/auth/session", "logout", AuthenticationLogout, CSRFRequired, "", BodyNone, RateNone},
	{http.MethodGet, "/api/v1/auth/sessions", "listSessions", AuthenticationRequired, CSRFNone, "", BodyNone, RateNone},
	{http.MethodDelete, "/api/v1/auth/sessions", "revokeAllSessions", AuthenticationRequired, CSRFRequired, "", BodyNone, RateNone},
	{http.MethodDelete, "/api/v1/auth/sessions/{session_id}", "revokeSession", AuthenticationRequired, CSRFRequired, "", BodyNone, RateNone},
	{http.MethodPut, "/api/v1/auth/password", "changePassword", AuthenticationRequired, CSRFRequired, "", BodyAuthJSON, RateNone},
	{http.MethodGet, "/api/v1/orgs", "listOrganizations", AuthenticationRequired, CSRFNone, "", BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs", "createOrganization", AuthenticationRequired, CSRFRequired, "", BodyJSON, RateAdministrative},
	{http.MethodGet, "/api/v1/orgs/{org_id}", "getOrganization", AuthenticationRequired, CSRFNone, authorization.PermissionOrgRead, BodyNone, RateNone},
	{http.MethodPatch, "/api/v1/orgs/{org_id}", "updateOrganization", AuthenticationRequired, CSRFRequired, authorization.PermissionOrgUpdate, BodyJSON, RateAdministrative},
	{http.MethodDelete, "/api/v1/orgs/{org_id}", "deleteOrganization", AuthenticationRequired, CSRFRequired, authorization.PermissionOrgDelete, BodyNone, RateAdministrative},
	{http.MethodPost, "/api/v1/orgs/{org_id}/restore", "restoreOrganization", AuthenticationRequired, CSRFRequired, authorization.PermissionOrgRestore, BodyNone, RateAdministrative},
	{http.MethodGet, "/api/v1/orgs/{org_id}/members", "listOrganizationMembers", AuthenticationRequired, CSRFNone, authorization.PermissionOrgMembersRead, BodyNone, RateNone},
	{http.MethodGet, "/api/v1/orgs/{org_id}/invitations", "listOrganizationInvitations", AuthenticationRequired, CSRFNone, authorization.PermissionOrgMembersManage, BodyNone, RateAdministrative},
	{http.MethodPost, "/api/v1/orgs/{org_id}/invitations", "createOrganizationInvitation", AuthenticationRequired, CSRFRequired, authorization.PermissionOrgMembersManage, BodyJSON, RateAdministrative},
	{http.MethodDelete, "/api/v1/orgs/{org_id}/invitations/{invitation_id}", "revokeOrganizationInvitation", AuthenticationRequired, CSRFRequired, authorization.PermissionOrgMembersManage, BodyNone, RateAdministrative},
	{http.MethodGet, "/api/v1/organization-invitations/current", "getCurrentOrganizationInvitation", AuthenticationPublic, CSRFNone, "", BodyNone, RateNone},
	{http.MethodPost, "/api/v1/organization-invitations/current/accept", "acceptCurrentOrganizationInvitation", AuthenticationRequired, CSRFRequired, "", BodyNone, RateAdministrative},
	{http.MethodDelete, "/api/v1/organization-invitations/current", "declineCurrentOrganizationInvitation", AuthenticationRequired, CSRFRequired, "", BodyNone, RateAdministrative},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams", "listTeams", AuthenticationRequired, CSRFNone, authorization.PermissionOrgRead, BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs/{org_id}/teams", "createTeam", AuthenticationRequired, CSRFRequired, authorization.PermissionTeamCreate, BodyJSON, RateAdministrative},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams/{team_id}/members", "listTeamMembers", AuthenticationRequired, CSRFNone, authorization.PermissionTeamMembersRead, BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs/{org_id}/teams/{team_id}/members", "addTeamMember", AuthenticationRequired, CSRFRequired, authorization.PermissionTeamMembersManage, BodyJSON, RateAdministrative},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams/{team_id}/tasks", "listTasks", AuthenticationRequired, CSRFNone, authorization.PermissionTaskRead, BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs/{org_id}/teams/{team_id}/tasks", "createTask", AuthenticationRequired, CSRFRequired, authorization.PermissionTaskCreate, BodyJSON, RateNone},
	{http.MethodPatch, "/api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}", "updateTask", AuthenticationRequired, CSRFRequired, authorization.PermissionTaskUpdate, BodyJSON, RateNone},
	{http.MethodDelete, "/api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}", "deleteTask", AuthenticationRequired, CSRFRequired, authorization.PermissionTaskDelete, BodyNone, RateNone},
	{http.MethodGet, "/api/v1/orgs/{org_id}/resources", "listResources", AuthenticationRequired, CSRFNone, authorization.PermissionResourceRead, BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs/{org_id}/resources", "createResource", AuthenticationRequired, CSRFRequired, authorization.PermissionResourceManage, BodyJSON, RateAdministrative},
	{http.MethodPatch, "/api/v1/orgs/{org_id}/resources/{resource_id}", "updateResource", AuthenticationRequired, CSRFRequired, authorization.PermissionResourceManage, BodyJSON, RateAdministrative},
	{http.MethodDelete, "/api/v1/orgs/{org_id}/resources/{resource_id}", "deleteResource", AuthenticationRequired, CSRFRequired, authorization.PermissionResourceManage, BodyNone, RateAdministrative},
	{http.MethodGet, "/api/v1/orgs/{org_id}/bookings", "listBookings", AuthenticationRequired, CSRFNone, authorization.PermissionResourceRead, BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs/{org_id}/resources/{resource_id}/requests", "createResourceRequest", AuthenticationRequired, CSRFRequired, authorization.PermissionResourceRequest, BodyJSON, RateNone},
	{http.MethodGet, "/api/v1/orgs/{org_id}/resource-requests", "listResourceRequests", AuthenticationRequired, CSRFNone, authorization.PermissionResourceRead, BodyNone, RateNone},
	{http.MethodPatch, "/api/v1/orgs/{org_id}/resource-requests/{request_id}", "decideResourceRequest", AuthenticationRequired, CSRFRequired, authorization.PermissionResourceDecide, BodyJSON, RateAdministrative},
	{http.MethodPut, "/api/v1/auth/user-keys", "upsertUserKeys", AuthenticationRequired, CSRFRequired, "", BodyJSON, RateNone},
	{http.MethodGet, "/api/v1/auth/user-keys", "getUserKeys", AuthenticationRequired, CSRFNone, "", BodyNone, RateNone},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams/{team_id}/members/public-keys", "getPublicKeysForTeam", AuthenticationRequired, CSRFNone, authorization.PermissionTeamRead, BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs/{org_id}/teams/{team_id}/keys", "createTeamKey", AuthenticationRequired, CSRFRequired, authorization.PermissionFileUpload, BodyJSON, RateNone},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams/{team_id}/keys", "listTeamKeys", AuthenticationRequired, CSRFNone, authorization.PermissionTeamRead, BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs/{org_id}/teams/{team_id}/files", "uploadFile", AuthenticationRequired, CSRFRequired, authorization.PermissionFileUpload, BodyFile, RateNone},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams/{team_id}/files", "listFiles", AuthenticationRequired, CSRFNone, authorization.PermissionFileRead, BodyNone, RateNone},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams/{team_id}/files/{file_id}", "downloadFile", AuthenticationRequired, CSRFNone, authorization.PermissionFileRead, BodyNone, RateNone},
	{http.MethodDelete, "/api/v1/orgs/{org_id}/teams/{team_id}/files/{file_id}", "deleteFile", AuthenticationRequired, CSRFRequired, authorization.PermissionFileDelete, BodyNone, RateNone},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams/{team_id}/chat/messages", "listChatMessages", AuthenticationRequired, CSRFNone, authorization.PermissionRealtimeConnect, BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs/{org_id}/teams/{team_id}/chat/messages", "createChatMessage", AuthenticationRequired, CSRFRequired, authorization.PermissionRealtimeConnect, BodyJSON, RateNone},
	{http.MethodDelete, "/api/v1/orgs/{org_id}/teams/{team_id}/chat/messages/{message_id}", "deleteChatMessage", AuthenticationRequired, CSRFRequired, authorization.PermissionRealtimeConnect, BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs/{org_id}/teams/{team_id}/chat/socket-ticket", "issueChatSocketTicket", AuthenticationRequired, CSRFRequired, authorization.PermissionRealtimeConnect, BodyNone, RateNone},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams/{team_id}/documents", "listDocuments", AuthenticationRequired, CSRFNone, authorization.PermissionDocumentRead, BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs/{org_id}/teams/{team_id}/documents", "createDocument", AuthenticationRequired, CSRFRequired, authorization.PermissionDocumentCreate, BodyJSON, RateNone},
	{http.MethodGet, "/api/v1/orgs/{org_id}/teams/{team_id}/documents/{document_id}", "getDocument", AuthenticationRequired, CSRFNone, authorization.PermissionDocumentRead, BodyNone, RateNone},
	{http.MethodPatch, "/api/v1/orgs/{org_id}/teams/{team_id}/documents/{document_id}", "updateDocument", AuthenticationRequired, CSRFRequired, authorization.PermissionDocumentUpdate, BodyJSON, RateNone},
	{http.MethodDelete, "/api/v1/orgs/{org_id}/teams/{team_id}/documents/{document_id}", "deleteDocument", AuthenticationRequired, CSRFRequired, authorization.PermissionDocumentDelete, BodyNone, RateNone},
	{http.MethodPost, "/api/v1/orgs/{org_id}/teams/{team_id}/documents/{document_id}/socket-ticket", "issueDocumentSocketTicket", AuthenticationRequired, CSRFRequired, authorization.PermissionRealtimeConnect, BodyNone, RateNone},
}
