package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
)

const maximumDocumentTitleLength = 255

type DocumentView struct {
	ID        uuid.UUID `json:"id"`
	TeamID    uuid.UUID `json:"team_id"`
	Title     string    `json:"title"`
	UpdatedBy uuid.UUID `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DocumentApplication struct{ authorizer applicationAuthorizer }

func NewDocumentApplication(authorizer applicationAuthorizer) *DocumentApplication {
	return &DocumentApplication{authorizer: authorizer}
}

func (application *DocumentApplication) Create(
	ctx context.Context, principal session.Principal, organizationID, teamID uuid.UUID, title string,
) (DocumentView, error) {
	title, err := normalizeDocumentTitle(title)
	if err != nil {
		return DocumentView{}, err
	}
	if application == nil || application.authorizer == nil || !validPrincipal(principal) || organizationID == uuid.Nil || teamID == uuid.Nil {
		return DocumentView{}, authorization.ErrResourceNotFound
	}
	var row tenantdb.Document
	err = application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionDocumentCreate,
		func(queries *tenantdb.Queries) error {
			team, queryErr := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{PublicID: teamID, UserPublicID: principal.UserID})
			if queryErr != nil {
				return queryErr
			}
			row, queryErr = queries.CreateDocument(ctx, tenantdb.CreateDocumentParams{TeamID: team.ID, Title: title, LastUpdatedBy: principal.UserID})
			return queryErr
		})
	if err != nil {
		return DocumentView{}, err
	}
	view := documentView(row)
	view.TeamID = teamID
	return view, nil
}

func (application *DocumentApplication) List(
	ctx context.Context, principal session.Principal, organizationID, teamID uuid.UUID,
) ([]DocumentView, error) {
	if application == nil || application.authorizer == nil || !validPrincipal(principal) || organizationID == uuid.Nil || teamID == uuid.Nil {
		return nil, authorization.ErrResourceNotFound
	}
	var rows []tenantdb.ListDocumentsForTeamRow
	err := application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionDocumentRead,
		func(queries *tenantdb.Queries) error {
			team, queryErr := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{PublicID: teamID, UserPublicID: principal.UserID})
			if queryErr != nil {
				return queryErr
			}
			rows, queryErr = queries.ListDocumentsForTeam(ctx, tenantdb.ListDocumentsForTeamParams{TeamID: team.ID, Limit: maximumListResults})
			return queryErr
		})
	if err != nil {
		return nil, err
	}
	views := make([]DocumentView, 0, len(rows))
	for _, row := range rows {
		views = append(views, DocumentView{ID: row.PublicID, TeamID: teamID, Title: row.Title, UpdatedBy: row.LastUpdatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return views, nil
}

func documentView(row tenantdb.Document) DocumentView {
	return DocumentView{ID: row.PublicID, Title: row.Title, UpdatedBy: row.LastUpdatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func normalizeDocumentTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > maximumDocumentTitleLength {
		return "", ErrInvalidInput
	}
	return title, nil
}
