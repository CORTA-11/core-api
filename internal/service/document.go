package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

type DocumentProjection struct {
	DocumentView
	BodyHTML string `json:"body_html"`
}

type DocumentPatch struct {
	Title    *string
	BodyHTML *string
}

type DocumentApplication struct {
	authorizer   applicationAuthorizer
	ticketSecret []byte
}

func NewDocumentApplication(authorizer applicationAuthorizer, ticketSecret []byte) *DocumentApplication {
	return &DocumentApplication{
		authorizer: authorizer, ticketSecret: append([]byte(nil), ticketSecret...),
	}
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

func (application *DocumentApplication) Get(ctx context.Context, principal session.Principal, organizationID, teamID, documentID uuid.UUID) (DocumentProjection, error) {
	if application == nil || application.authorizer == nil || !validPrincipal(principal) || organizationID == uuid.Nil || teamID == uuid.Nil || documentID == uuid.Nil {
		return DocumentProjection{}, authorization.ErrResourceNotFound
	}
	var row tenantdb.Document
	err := application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionDocumentRead, func(queries *tenantdb.Queries) error {
		team, queryErr := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{PublicID: teamID, UserPublicID: principal.UserID})
		if queryErr != nil {
			return queryErr
		}
		row, queryErr = queries.GetDocumentForTeam(ctx, tenantdb.GetDocumentForTeamParams{TeamID: team.ID, PublicID: documentID})
		return queryErr
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = authorization.ErrResourceNotFound
		}
		return DocumentProjection{}, fmt.Errorf("get document projection: %w", err)
	}
	view := documentView(row)
	view.TeamID = teamID
	return DocumentProjection{DocumentView: view, BodyHTML: row.BodyHtml}, nil
}

func (application *DocumentApplication) Update(
	ctx context.Context, principal session.Principal, organizationID, teamID, documentID uuid.UUID, patch DocumentPatch,
) (DocumentProjection, error) {
	if patch.Title == nil && patch.BodyHTML == nil {
		return DocumentProjection{}, ErrInvalidInput
	}
	if patch.Title != nil {
		title, err := normalizeDocumentTitle(*patch.Title)
		if err != nil {
			return DocumentProjection{}, err
		}
		patch.Title = &title
	}
	if application == nil || application.authorizer == nil || !validPrincipal(principal) ||
		organizationID == uuid.Nil || teamID == uuid.Nil || documentID == uuid.Nil {
		return DocumentProjection{}, authorization.ErrResourceNotFound
	}
	var row tenantdb.Document
	err := application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionDocumentUpdate,
		func(queries *tenantdb.Queries) error {
			var queryErr error
			row, queryErr = queries.UpdateDocument(ctx, tenantdb.UpdateDocumentParams{
				Title: nullableDocumentText(patch.Title), BodyHtml: nullableDocumentText(patch.BodyHTML),
				LastUpdatedBy: principal.UserID, PublicID: documentID,
			})
			if errors.Is(queryErr, pgx.ErrNoRows) {
				return authorization.ErrResourceNotFound
			}
			return queryErr
		})
	if err != nil {
		return DocumentProjection{}, err
	}
	view := documentView(row)
	view.TeamID = teamID
	return DocumentProjection{DocumentView: view, BodyHTML: row.BodyHtml}, nil
}

func (application *DocumentApplication) Delete(
	ctx context.Context, principal session.Principal, organizationID, teamID, documentID uuid.UUID,
) error {
	if application == nil || application.authorizer == nil || !validPrincipal(principal) ||
		organizationID == uuid.Nil || teamID == uuid.Nil || documentID == uuid.Nil {
		return authorization.ErrResourceNotFound
	}
	return application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionDocumentDelete,
		func(queries *tenantdb.Queries) error {
			count, err := queries.DeleteDocument(ctx, documentID)
			if err == nil && count == 0 {
				return authorization.ErrResourceNotFound
			}
			return err
		})
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

func nullableDocumentText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}
