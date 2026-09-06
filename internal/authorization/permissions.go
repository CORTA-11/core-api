package authorization

// OrganizationAllows organizations allows.
func OrganizationAllows(role OrganizationRole, permission Permission) bool {
	if !ValidOrganizationRole(role) || !ValidPermission(permission) {
		return false
	}
	switch role {
	case OrganizationRoleOwner:
		switch permission {
		case PermissionOrgRead, PermissionOrgUpdate, PermissionOrgDelete, PermissionOrgRestore,
			PermissionOrgMembersRead, PermissionOrgMembersManage, PermissionOrgOwnersManage, PermissionTeamCreate,
			PermissionResourceRead, PermissionResourceManage, PermissionResourceDecide:
			return true
		}
	case OrganizationRoleAdministrator:
		switch permission {
		case PermissionOrgRead, PermissionOrgUpdate, PermissionOrgMembersRead,
			PermissionOrgMembersManage, PermissionTeamCreate,
			PermissionResourceRead, PermissionResourceManage, PermissionResourceDecide:
			return true
		}
	case OrganizationRoleMember:
		return permission == PermissionOrgRead || permission == PermissionResourceRead
	}
	return false
}

// TeamAllows teams allows.
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
			PermissionAuditRead, PermissionRealtimeConnect, PermissionResourceRequest,
			PermissionDocumentRead, PermissionDocumentCreate:
			return true
		}
	case TeamRoleResearchLead:
		switch permission {
		case PermissionTeamRead, PermissionTeamMembersRead,
			PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate, PermissionTaskMove, PermissionTaskDelete,
			PermissionFileRead, PermissionFileUpload, PermissionFileDelete,
			PermissionAuditRead, PermissionRealtimeConnect, PermissionDocumentRead, PermissionDocumentCreate:
			return true
		}
	case TeamRoleResearcher:
		switch permission {
		case PermissionTeamRead, PermissionTeamMembersRead,
			PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate, PermissionTaskMove,
			PermissionFileRead, PermissionFileUpload, PermissionRealtimeConnect, PermissionDocumentRead, PermissionDocumentCreate:
			return true
		}
	case TeamRoleContributor:
		switch permission {
		case PermissionTeamRead, PermissionTeamMembersRead, PermissionTaskRead, PermissionTaskCreate, PermissionTaskUpdate,
			PermissionFileRead, PermissionFileUpload, PermissionRealtimeConnect, PermissionDocumentRead, PermissionDocumentCreate:
			return true
		}
	case TeamRoleViewer:
		switch permission {
		case PermissionTeamRead, PermissionTeamMembersRead, PermissionTaskRead, PermissionFileRead, PermissionRealtimeConnect,
			PermissionDocumentRead, PermissionDocumentCreate:
			return true
		}
	}
	return false
}
