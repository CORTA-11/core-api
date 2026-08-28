package authorization

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRolePermissionMappingsAreClosedAndSeparated(t *testing.T) {
	t.Parallel()
	orgExpected := map[OrganizationRole]map[Permission]bool{
		OrganizationRoleOwner: set(PermissionOrgRead, PermissionOrgUpdate, PermissionOrgDelete, PermissionOrgRestore,
			PermissionOrgMembersRead, PermissionOrgMembersManage, PermissionOrgOwnersManage, PermissionTeamCreate),
		OrganizationRoleAdministrator: set(PermissionOrgRead, PermissionOrgUpdate, PermissionOrgMembersRead,
			PermissionOrgMembersManage, PermissionTeamCreate),
		OrganizationRoleMember: set(PermissionOrgRead),
	}
	teamExpected := map[TeamRole]map[Permission]bool{
		TeamRoleAdmin: set(PermissionTeamRead, PermissionTeamUpdate, PermissionTeamDelete,
			PermissionTeamMembersRead, PermissionTeamMembersManage,
			PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate, PermissionTaskMove, PermissionTaskDelete,
			PermissionFileRead, PermissionFileUpload, PermissionFileDelete, PermissionAuditRead, PermissionRealtimeConnect),
		TeamRoleResearchLead: set(PermissionTeamRead, PermissionTeamMembersRead,
			PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate, PermissionTaskMove, PermissionTaskDelete,
			PermissionFileRead, PermissionFileUpload, PermissionFileDelete, PermissionAuditRead, PermissionRealtimeConnect),
		TeamRoleResearcher: set(PermissionTeamRead, PermissionTeamMembersRead, PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate,
			PermissionTaskMove, PermissionFileRead, PermissionFileUpload, PermissionRealtimeConnect),
		TeamRoleContributor: set(PermissionTeamRead, PermissionTeamMembersRead, PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate,
			PermissionFileRead, PermissionFileUpload, PermissionRealtimeConnect),
		TeamRoleViewer: set(PermissionTeamRead, PermissionTeamMembersRead, PermissionTaskRead, PermissionFileRead, PermissionRealtimeConnect),
	}
	for _, role := range organizationRoles {
		for _, permission := range permissions {
			assert.Equal(t, orgExpected[role][permission], OrganizationAllows(role, permission), "%s %s", role, permission)
		}
	}
	for _, role := range teamRoles {
		for _, permission := range permissions {
			assert.Equal(t, teamExpected[role][permission], TeamAllows(role, permission), "%s %s", role, permission)
		}
	}

	assert.False(t, OrganizationAllows("", PermissionOrgRead))
	assert.False(t, OrganizationAllows("future_role", PermissionOrgRead))
	assert.False(t, OrganizationAllows(OrganizationRoleOwner, "future.permission"))
	assert.False(t, TeamAllows("", PermissionTaskRead))
	assert.False(t, TeamAllows("future_role", PermissionTaskRead))
	assert.False(t, TeamAllows(TeamRoleAdmin, "future.permission"))
	assert.False(t, OrganizationAllows(OrganizationRoleOwner, PermissionTeamRead), "organization administration must not grant team access")
}

func set(values ...Permission) map[Permission]bool {
	result := make(map[Permission]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
