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

const (
	keyAccessStatusGranted = "granted"
	keyAccessStatusDenied  = "denied"
)

// KeyAccessRequestView is a team-scoped request to read key versions that
// predate the requester's membership.
type KeyAccessRequestView struct {
	ID              uuid.UUID  `json:"id"`
	TeamID          uuid.UUID  `json:"team_id"`
	TeamName        string     `json:"team_name"`
	RequestedBy     uuid.UUID  `json:"requested_by"`
	RequestedByName string     `json:"requested_by_name"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	DecidedBy       *uuid.UUID `json:"decided_by,omitempty"`
	DecidedByName   string     `json:"decided_by_name,omitempty"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
}

type KeyAccessRequestService interface {
	CreateKeyAccessRequest(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID) (KeyAccessRequestView, error)
	ListKeyAccessRequests(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID) ([]KeyAccessRequestView, error)
	DecideKeyAccessRequest(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, requestID uuid.UUID, status string) (KeyAccessRequestView, error)
}

type keyAccessApplication struct {
	authorizer applicationAuthorizer
}

// NewKeyAccessApplication creates a KeyAccessRequestService.
func NewKeyAccessApplication(authorizer applicationAuthorizer) KeyAccessRequestService {
	return &keyAccessApplication{authorizer: authorizer}
}

// CreateKeyAccessRequest creates the caller's pending request for this team.
func (a *keyAccessApplication) CreateKeyAccessRequest(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID) (KeyAccessRequestView, error) {
	var view KeyAccessRequestView
	err := a.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionFileRead, func(queries *tenantdb.Queries) error {
		row, queryErr := queries.CreateKeyAccessRequest(ctx)
		if queryErr != nil {
			return queryErr
		}
		view = keyAccessRequestFromCreate(row)
		return nil
	})
	if isUniqueViolation(err) {
		return view, ErrConflict
	}
	return view, err
}

// ListKeyAccessRequests lists this team's requests; members see the team's list
// so the requester can watch their own status and the leader can decide.
func (a *keyAccessApplication) ListKeyAccessRequests(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID) ([]KeyAccessRequestView, error) {
	views := make([]KeyAccessRequestView, 0)
	err := a.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionFileRead, func(queries *tenantdb.Queries) error {
		rows, queryErr := queries.ListKeyAccessRequests(ctx, maximumListResults)
		if queryErr != nil {
			return queryErr
		}
		views = make([]KeyAccessRequestView, len(rows))
		for i := range rows {
			views[i] = keyAccessRequestFromList(rows[i])
		}
		return nil
	})
	return views, err
}

// DecideKeyAccessRequest approves or denies a pending request. The leader's
// client performs the re-wrap before approving; this endpoint only records it.
func (a *keyAccessApplication) DecideKeyAccessRequest(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, requestID uuid.UUID, status string) (KeyAccessRequestView, error) {
	if status != keyAccessStatusGranted && status != keyAccessStatusDenied {
		return KeyAccessRequestView{}, ErrInvalidInput
	}
	var view KeyAccessRequestView
	err := a.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionKeyAccessDecide, func(queries *tenantdb.Queries) error {
		current, queryErr := queries.GetKeyAccessRequest(ctx, requestID)
		if errors.Is(queryErr, pgx.ErrNoRows) {
			return authorization.ErrResourceNotFound
		}
		if queryErr != nil {
			return queryErr
		}
		if current.Status != "pending" {
			return ErrConflict
		}
		row, decideErr := queries.DecideKeyAccessRequest(ctx, tenantdb.DecideKeyAccessRequestParams{
			Status:   status,
			PublicID: requestID,
		})
		if errors.Is(decideErr, pgx.ErrNoRows) {
			return ErrConflict
		}
		if decideErr != nil {
			return decideErr
		}
		view = keyAccessRequestFromCreate(tenantdb.CreateKeyAccessRequestRow{
			ID:              row.ID,
			PublicID:        row.PublicID,
			TeamID:          row.TeamID,
			RequestedBy:     row.RequestedBy,
			Status:          row.Status,
			CreatedAt:       row.CreatedAt,
			DecidedBy:       row.DecidedBy,
			DecidedAt:       row.DecidedAt,
			TeamPublicID:    row.TeamPublicID,
			TeamName:        row.TeamName,
			RequestedByName: row.RequestedByName,
		})
		if row.DecidedBy.Valid {
			decidedBy, _ := uuid.FromBytes(row.DecidedBy.Bytes[:])
			view.DecidedBy = &decidedBy
			view.DecidedByName = row.DecidedByName
		}
		return nil
	})
	return view, err
}

func keyAccessRequestFromCreate(row tenantdb.CreateKeyAccessRequestRow) KeyAccessRequestView {
	view := KeyAccessRequestView{
		ID:              row.PublicID,
		TeamID:          row.TeamPublicID,
		TeamName:        row.TeamName,
		RequestedBy:     row.RequestedBy,
		RequestedByName: row.RequestedByName,
		Status:          row.Status,
		CreatedAt:       row.CreatedAt,
	}
	if row.DecidedBy.Valid {
		decidedBy, _ := uuid.FromBytes(row.DecidedBy.Bytes[:])
		view.DecidedBy = &decidedBy
	}
	if row.DecidedAt.Valid {
		decidedAt := row.DecidedAt.Time
		view.DecidedAt = &decidedAt
	}
	return view
}

func keyAccessRequestFromList(row tenantdb.ListKeyAccessRequestsRow) KeyAccessRequestView {
	view := KeyAccessRequestView{
		ID:              row.PublicID,
		TeamID:          row.TeamPublicID,
		TeamName:        row.TeamName,
		RequestedBy:     row.RequestedBy,
		RequestedByName: row.RequestedByName,
		Status:          row.Status,
		CreatedAt:       row.CreatedAt,
		DecidedByName:   row.DecidedByName,
	}
	if row.DecidedBy.Valid {
		decidedBy, _ := uuid.FromBytes(row.DecidedBy.Bytes[:])
		view.DecidedBy = &decidedBy
	}
	if row.DecidedAt.Valid {
		decidedAt := row.DecidedAt.Time
		view.DecidedAt = &decidedAt
	}
	return view
}
