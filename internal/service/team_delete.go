package service

import (
	"context"
	"errors"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func (application *TeamTaskApplication) DeleteTeam(ctx context.Context, principal session.Principal, organizationID, teamID uuid.UUID) error {
	if application == nil || application.authorizer == nil || !validPrincipal(principal) || organizationID == uuid.Nil || teamID == uuid.Nil {
		return authorization.ErrResourceNotFound
	}
	remove := func(queries *tenantdb.Queries) error {
		_, err := queries.SoftDeleteTeam(ctx, teamID)
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) {
			switch databaseError.Code {
			case "P0002":
				return authorization.ErrResourceNotFound
			case "42501":
				return authorization.ErrOperationDenied
			}
		}
		return err
	}
	// Organization administration grants deletion, never access to team content.
	err := application.authorizer.WithinOrganization(ctx, principal, organizationID, authorization.PermissionTeamDelete, remove)
	if errors.Is(err, authorization.ErrOperationDenied) {
		return application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionTeamDelete, remove)
	}
	return err
}
