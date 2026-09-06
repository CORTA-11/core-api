package service

import (
	"context"
	"errors"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type DocumentSocketTicketClaims struct {
	UserID     uuid.UUID `json:"user_id"`
	OrgID      uuid.UUID `json:"org_id"`
	TeamID     uuid.UUID `json:"team_id"`
	DocumentID uuid.UUID `json:"document_id"`
	Purpose    string    `json:"purpose"`
	ExpiresAt  int64     `json:"exp"`
}

func (application *DocumentApplication) IssueSocketTicket(
	ctx context.Context, principal session.Principal, organizationID, teamID, documentID uuid.UUID,
) (string, error) {
	if application == nil || application.authorizer == nil || len(application.ticketSecret) < 32 {
		return "", errors.New("document socket ticket is not configured")
	}
	if !validPrincipal(principal) || organizationID == uuid.Nil || teamID == uuid.Nil || documentID == uuid.Nil {
		return "", authorization.ErrResourceNotFound
	}
	err := application.authorizer.WithinTeam(
		ctx, principal, organizationID, teamID, authorization.PermissionRealtimeConnect,
		func(queries *tenantdb.Queries) error {
			team, err := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{
				PublicID: teamID, UserPublicID: principal.UserID,
			})
			if err != nil {
				return err
			}
			_, err = queries.GetDocumentForTeam(ctx, tenantdb.GetDocumentForTeamParams{
				TeamID: team.ID, PublicID: documentID,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				return authorization.ErrResourceNotFound
			}
			return err
		},
	)
	if err != nil {
		return "", err
	}
	return signSocketTicket(DocumentSocketTicketClaims{
		UserID: principal.UserID, OrgID: organizationID, TeamID: teamID,
		DocumentID: documentID, Purpose: "document", ExpiresAt: time.Now().Add(socketTicketTTL).Unix(),
	}, application.ticketSecret)
}
