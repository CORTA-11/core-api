package service

import (
	"context"
	"errors"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ContentAccessSnapshot contains metadata only. RLS restricts requests to their
// requester and the immutable creator, even when the caller leads the team.
type ContentAccessSnapshot struct {
	Items    []tenantdb.ListContentAccessRow         `json:"items"`
	Requests []tenantdb.ListContentAccessRequestsRow `json:"requests"`
}

type ContentAccessService interface {
	List(context.Context, session.Principal, uuid.UUID, uuid.UUID) (ContentAccessSnapshot, error)
	Request(context.Context, session.Principal, uuid.UUID, uuid.UUID, string, uuid.UUID) error
	Decide(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID, string) error
	Grant(context.Context, session.Principal, uuid.UUID, uuid.UUID, string, uuid.UUID, uuid.UUID) error
}

type contentAccessApplication struct{ authorizer applicationAuthorizer }

func NewContentAccessApplication(authorizer applicationAuthorizer) ContentAccessService {
	return &contentAccessApplication{authorizer: authorizer}
}

func (a *contentAccessApplication) List(ctx context.Context, p session.Principal, orgID, teamID uuid.UUID) (ContentAccessSnapshot, error) {
	snapshot := ContentAccessSnapshot{Items: make([]tenantdb.ListContentAccessRow, 0), Requests: make([]tenantdb.ListContentAccessRequestsRow, 0)}
	err := a.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionDocumentRead, func(q *tenantdb.Queries) error {
		items, err := q.ListContentAccess(ctx, maximumListResults)
		if err != nil {
			return err
		}
		requests, err := q.ListContentAccessRequests(ctx, maximumListResults)
		if err != nil {
			return err
		}
		if items != nil {
			snapshot.Items = items
		}
		if requests != nil {
			snapshot.Requests = requests
		}
		return nil
	})
	return snapshot, err
}

func validContent(kind string, id uuid.UUID) bool {
	return (kind == "document" || kind == "file") && id != uuid.Nil
}

func (a *contentAccessApplication) Request(ctx context.Context, p session.Principal, orgID, teamID uuid.UUID, kind string, id uuid.UUID) error {
	if !validContent(kind, id) {
		return ErrInvalidInput
	}
	return a.mutate(ctx, p, orgID, teamID, func(q *tenantdb.Queries) error {
		_, err := q.RequestContentAccess(ctx, tenantdb.RequestContentAccessParams{Kind: kind, ResourceID: id})
		return err
	})
}

func (a *contentAccessApplication) Decide(ctx context.Context, p session.Principal, orgID, teamID, requestID uuid.UUID, status string) error {
	if requestID == uuid.Nil || (status != "granted" && status != "denied") {
		return ErrInvalidInput
	}
	return a.mutate(ctx, p, orgID, teamID, func(q *tenantdb.Queries) error {
		_, err := q.DecideContentAccess(ctx, tenantdb.DecideContentAccessParams{PublicID: requestID, Status: status})
		return err
	})
}

func (a *contentAccessApplication) Grant(ctx context.Context, p session.Principal, orgID, teamID uuid.UUID, kind string, id, memberID uuid.UUID) error {
	if !validContent(kind, id) || memberID == uuid.Nil {
		return ErrInvalidInput
	}
	return a.mutate(ctx, p, orgID, teamID, func(q *tenantdb.Queries) error {
		_, err := q.GrantContentAccess(ctx, tenantdb.GrantContentAccessParams{Kind: kind, ResourceID: id, RequestedBy: memberID})
		return err
	})
}

func (a *contentAccessApplication) mutate(ctx context.Context, p session.Principal, orgID, teamID uuid.UUID, query func(*tenantdb.Queries) error) error {
	err := a.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionDocumentRead, query)
	if errors.Is(err, pgx.ErrNoRows) {
		return authorization.ErrResourceNotFound
	}
	return err
}
