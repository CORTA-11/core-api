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

type DocumentState struct {
	CanonicalState []byte    `json:"canonical_state"`
	Title          string    `json:"title"`
	BodyHTML       string    `json:"body_html"`
	UpdatedBy      uuid.UUID `json:"updated_by"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type DocumentStateWrite struct {
	CanonicalState []byte `json:"canonical_state"`
	Title          string `json:"title"`
	BodyHTML       string `json:"body_html"`
}

type DocumentApplication struct {
	authorizer   applicationAuthorizer
	roomCloser   documentRoomCloser
	ticketSecret []byte
}

type DocumentService interface {
	List(context.Context, session.Principal, uuid.UUID, uuid.UUID) ([]DocumentView, error)
	Get(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID) (DocumentProjection, error)
	Create(context.Context, session.Principal, uuid.UUID, uuid.UUID, string) (DocumentView, error)
	Update(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID, DocumentPatch) (DocumentProjection, error)
	Delete(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID) error
	IssueSocketTicket(context.Context, session.Principal, string, uuid.UUID, uuid.UUID, uuid.UUID) (string, error)
	LoadState(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (DocumentState, error)
	StoreState(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, DocumentStateWrite) (DocumentState, error)
}

type documentRoomCloser interface {
	CloseDocumentRoom(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
}

func NewDocumentApplication(
	authorizer applicationAuthorizer, ticketSecret []byte, roomCloser documentRoomCloser,
) *DocumentApplication {
	return &DocumentApplication{
		authorizer: authorizer, roomCloser: roomCloser, ticketSecret: append([]byte(nil), ticketSecret...),
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
	err := application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionDocumentDelete,
		func(queries *tenantdb.Queries) error {
			if err := application.roomCloser.CloseDocumentRoom(ctx, organizationID, teamID, documentID); err != nil {
				return fmt.Errorf("close Document Room before deletion: %w", err)
			}
			count, err := queries.DeleteDocument(ctx, documentID)
			if err == nil && count == 0 {
				return authorization.ErrResourceNotFound
			}
			return err
		})
	return err
}

func (application *DocumentApplication) LoadState(
	ctx context.Context, editorID, organizationID, teamID, documentID uuid.UUID,
) (DocumentState, error) {
	row, err := application.collaborationDocument(ctx, editorID, organizationID, teamID, documentID,
		authorization.PermissionDocumentRead, func(queries *tenantdb.Queries, resolvedTeamID int64) (tenantdb.Document, error) {
			return queries.GetDocumentForTeam(ctx, tenantdb.GetDocumentForTeamParams{
				TeamID: resolvedTeamID, PublicID: documentID,
			})
		})
	if err != nil {
		return DocumentState{}, fmt.Errorf("load Document collaboration state: %w", err)
	}
	return documentState(row), nil
}

func (application *DocumentApplication) StoreState(
	ctx context.Context, editorID, organizationID, teamID, documentID uuid.UUID, state DocumentStateWrite,
) (DocumentState, error) {
	title, titleErr := normalizeDocumentTitle(state.Title)
	if titleErr != nil || len(state.CanonicalState) == 0 {
		return DocumentState{}, ErrInvalidInput
	}
	row, err := application.collaborationDocument(ctx, editorID, organizationID, teamID, documentID,
		authorization.PermissionDocumentUpdate, func(queries *tenantdb.Queries, resolvedTeamID int64) (tenantdb.Document, error) {
			return queries.StoreDocumentState(ctx, tenantdb.StoreDocumentStateParams{
				CanonicalState: state.CanonicalState, Title: title, BodyHtml: state.BodyHTML,
				LastUpdatedBy: editorID, TeamID: resolvedTeamID, PublicID: documentID,
			})
		})
	if err != nil {
		return DocumentState{}, fmt.Errorf("store Document collaboration state: %w", err)
	}
	return documentState(row), nil
}

func (application *DocumentApplication) collaborationDocument(
	ctx context.Context,
	editorID, organizationID, teamID, documentID uuid.UUID,
	permission authorization.Permission,
	query func(*tenantdb.Queries, int64) (tenantdb.Document, error),
) (tenantdb.Document, error) {
	principal, ok := collaborationPrincipal(editorID)
	if application == nil || application.authorizer == nil || !ok || organizationID == uuid.Nil ||
		teamID == uuid.Nil || documentID == uuid.Nil {
		return tenantdb.Document{}, authorization.ErrResourceNotFound
	}
	var row tenantdb.Document
	err := application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, permission,
		func(queries *tenantdb.Queries) error {
			team, resolveErr := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{
				PublicID: teamID, UserPublicID: editorID,
			})
			if resolveErr != nil {
				return resolveErr
			}
			row, resolveErr = query(queries, team.ID)
			return resolveErr
		})
	if errors.Is(err, pgx.ErrNoRows) {
		err = authorization.ErrResourceNotFound
	}
	return row, err
}

func documentView(row tenantdb.Document) DocumentView {
	return DocumentView{ID: row.PublicID, Title: row.Title, UpdatedBy: row.LastUpdatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func documentState(row tenantdb.Document) DocumentState {
	return DocumentState{
		CanonicalState: row.CanonicalState, Title: row.Title, BodyHTML: row.BodyHtml,
		UpdatedBy: row.LastUpdatedBy, UpdatedAt: row.UpdatedAt,
	}
}

func collaborationPrincipal(editorID uuid.UUID) (session.Principal, bool) {
	if editorID == uuid.Nil {
		return session.Principal{}, false
	}
	return session.Principal{UserID: editorID, SessionID: editorID}, true
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
