package authorization

func OrganizationAllows(role OrganizationRole, permission Permission) bool {
	if !ValidOrganizationRole(role) || !ValidPermission(permission) {
		return false
	}
	switch role {
	case OrganizationRoleOwner:
		switch permission {
		case PermissionOrgRead, PermissionOrgUpdate, PermissionOrgDelete, PermissionOrgRestore,
			PermissionOrgMembersRead, PermissionOrgMembersManage, PermissionOrgOwnersManage, PermissionTeamCreate:
			return true
		}
	case OrganizationRoleAdministrator:
		switch permission {
		case PermissionOrgRead, PermissionOrgUpdate, PermissionOrgMembersRead,
			PermissionOrgMembersManage, PermissionTeamCreate:
			return true
		}
	case OrganizationRoleMember:
		return permission == PermissionOrgRead
	}
	return false
}

func TeamAllows(role TeamRole, permission Permission) bool {
	if !ValidTeamRole(role) || !ValidPermission(permission) {
		return false
	}
	switch role {
	case TeamRoleAdmin:
		switch permission {
		case PermissionTeamRead, PermissionTeamUpdate, PermissionTeamDelete,
			PermissionTeamMembersRead, PermissionTeamMembersManage,
			PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate, PermissionTaskMove, PermissionTaskDelete,
			PermissionFileRead, PermissionFileUpload, PermissionFileDelete,
			PermissionAuditRead, PermissionRealtimeConnect:
			return true
		}
	case TeamRoleResearchLead:
		switch permission {
		case PermissionTeamRead, PermissionTeamMembersRead,
			PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate, PermissionTaskMove, PermissionTaskDelete,
			PermissionFileRead, PermissionFileUpload, PermissionFileDelete,
			PermissionAuditRead, PermissionRealtimeConnect:
			return true
		}
	case TeamRoleResearcher:
		switch permission {
		case PermissionTeamRead, PermissionTeamMembersRead,
			PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate, PermissionTaskMove,
			PermissionFileRead, PermissionFileUpload, PermissionRealtimeConnect:
			return true
		}
	case TeamRoleContributor:
		switch permission {
		case PermissionTeamRead, PermissionTeamMembersRead, PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate,
			PermissionFileRead, PermissionFileUpload, PermissionRealtimeConnect:
			return true
		}
	case TeamRoleViewer:
		switch permission {
		case PermissionTeamRead, PermissionTeamMembersRead, PermissionTaskRead, PermissionFileRead, PermissionRealtimeConnect:
			return true
		}
	}
	return false
}
