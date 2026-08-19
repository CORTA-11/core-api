package service

import (
	"context"

	"github.com/google/uuid"
)

// OrgUserService defines methods for managing user memberships in organizations.
type OrgUserService interface {
	AddUserToOrg(ctx context.Context, orgPublicID uuid.UUID, userPublicID uuid.UUID) error
	RemoveUserFromOrg(ctx context.Context, orgPublicID uuid.UUID, userPublicID uuid.UUID) error
	GetOrgsForUser(ctx context.Context, userPublicID uuid.UUID) ([]Organization, error)
	GetUsersInOrg(ctx context.Context, orgPublicID uuid.UUID) ([]User, error)
	GetNumberOfUsersInOrg(ctx context.Context, orgPublicID uuid.UUID) (int64, error)
}
