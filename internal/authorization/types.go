// Package authorization defines the closed application permission vocabulary
// and composes it with trusted database-backed tenant execution.
package authorization

import "errors"

var (
	ErrUnauthenticated  = errors.New("authentication is required")
	ErrOperationDenied  = errors.New("operation is denied")
	ErrResourceNotFound = errors.New("resource is not found")
)

type Permission string

const (
	PermissionOrgRead           Permission = "org.read"
	PermissionOrgUpdate         Permission = "org.update"
	PermissionOrgDelete         Permission = "org.delete"
	PermissionOrgRestore        Permission = "org.restore"
	PermissionOrgMembersRead    Permission = "org.members.read"
	PermissionOrgMembersManage  Permission = "org.members.manage"
	PermissionOrgOwnersManage   Permission = "org.owners.manage"
	PermissionTeamCreate        Permission = "team.create"
	PermissionTeamRead          Permission = "team.read"
	PermissionTeamUpdate        Permission = "team.update"
	PermissionTeamDelete        Permission = "team.delete"
	PermissionTeamMembersRead   Permission = "team.members.read"
	PermissionTeamMembersManage Permission = "team.members.manage"
	PermissionTaskRead          Permission = "task.read"
	PermissionTaskCreate        Permission = "task.create"
	PermissionTaskUpdate        Permission = "task.update"
	PermissionTaskMove          Permission = "task.move"
	PermissionTaskDelete        Permission = "task.delete"
	PermissionFileRead          Permission = "file.read"
	PermissionFileUpload        Permission = "file.upload"
	PermissionFileDelete        Permission = "file.delete"
	PermissionAuditRead         Permission = "audit.read"
	PermissionRealtimeConnect   Permission = "realtime.connect"
	PermissionDocumentRead      Permission = "document.read"
	PermissionDocumentCreate    Permission = "document.create"
	PermissionResourceRead      Permission = "resource.read"
	PermissionResourceManage    Permission = "resource.manage"
	PermissionResourceRequest   Permission = "resource.request"
	PermissionResourceDecide    Permission = "resource.decide"
)

var permissions = [...]Permission{
	PermissionOrgRead, PermissionOrgUpdate, PermissionOrgDelete, PermissionOrgRestore,
	PermissionOrgMembersRead, PermissionOrgMembersManage, PermissionOrgOwnersManage,
	PermissionTeamCreate, PermissionTeamRead, PermissionTeamUpdate, PermissionTeamDelete,
	PermissionTeamMembersRead, PermissionTeamMembersManage,
	PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate, PermissionTaskMove, PermissionTaskDelete,
	PermissionFileRead, PermissionFileUpload, PermissionFileDelete,
	PermissionAuditRead, PermissionRealtimeConnect,
	PermissionDocumentRead, PermissionDocumentCreate,
	PermissionResourceRead, PermissionResourceManage, PermissionResourceRequest, PermissionResourceDecide,
}

type OrganizationRole string

const (
	OrganizationRoleOwner         OrganizationRole = "owner"
	OrganizationRoleAdministrator OrganizationRole = "administrator"
	OrganizationRoleMember        OrganizationRole = "member"
)

var organizationRoles = [...]OrganizationRole{
	OrganizationRoleOwner, OrganizationRoleAdministrator, OrganizationRoleMember,
}

type TeamRole string

const (
	TeamRoleAdmin        TeamRole = "team_admin"
	TeamRoleResearchLead TeamRole = "research_lead"
	TeamRoleResearcher   TeamRole = "researcher"
	TeamRoleContributor  TeamRole = "contributor"
	TeamRoleViewer       TeamRole = "viewer"
)

var teamRoles = [...]TeamRole{
	TeamRoleAdmin, TeamRoleResearchLead, TeamRoleResearcher, TeamRoleContributor, TeamRoleViewer,
}

// ValidPermission checks whether permission is valid.
func ValidPermission(permission Permission) bool {
	for _, known := range permissions {
		if permission == known {
			return true
		}
	}
	return false
}

// ValidOrganizationRole checks whether organization role is valid.
func ValidOrganizationRole(role OrganizationRole) bool {
	for _, known := range organizationRoles {
		if role == known {
			return true
		}
	}
	return false
}

// ValidTeamRole checks whether team role is valid.
func ValidTeamRole(role TeamRole) bool {
	for _, known := range teamRoles {
		if role == known {
			return true
		}
	}
	return false
}
