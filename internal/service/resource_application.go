package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var resourceCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{0,63}$`)

type AvailabilityWindow struct {
	Weekday int    `json:"weekday"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

type ResourceWrite struct {
	Name         string               `json:"name"`
	Code         string               `json:"code"`
	Kind         string               `json:"kind"`
	Location     string               `json:"location"`
	Enabled      bool                 `json:"enabled"`
	Availability []AvailabilityWindow `json:"availability"`
}

type ResourcePatch struct {
	Name         *string               `json:"name"`
	Code         *string               `json:"code"`
	Kind         *string               `json:"kind"`
	Location     *string               `json:"location"`
	Enabled      *bool                 `json:"enabled"`
	Availability *[]AvailabilityWindow `json:"availability"`
}

type ResourceView struct {
	ID uuid.UUID `json:"id"`
	ResourceWrite
}

type BookingView struct {
	ID              uuid.UUID  `json:"id"`
	ResourceID      uuid.UUID  `json:"resource_id"`
	ResourceName    string     `json:"resource_name"`
	StartTime       time.Time  `json:"start_time"`
	EndTime         time.Time  `json:"end_time"`
	DetailsVisible  bool       `json:"details_visible"`
	TeamPublicID    *uuid.UUID `json:"team_public_id"`
	TeamName        *string    `json:"team_name"`
	RequestedByName *string    `json:"requested_by_name"`
	Purpose         *string    `json:"purpose"`
}

type ResourceRequestView struct {
	ID              uuid.UUID  `json:"id"`
	ResourceID      uuid.UUID  `json:"resource_id"`
	ResourceName    string     `json:"resource_name"`
	TeamPublicID    uuid.UUID  `json:"team_public_id"`
	TeamName        string     `json:"team_name"`
	RequestedBy     uuid.UUID  `json:"requested_by"`
	RequestedByName string     `json:"requested_by_name"`
	StartTime       time.Time  `json:"start_time"`
	EndTime         time.Time  `json:"end_time"`
	Purpose         string     `json:"purpose"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	DecidedAt       *time.Time `json:"decided_at"`
}

type ResourceApplication struct {
	authorizer applicationAuthorizer
	now        func() time.Time
}

// NewResourceApplication creates a resource application.
func NewResourceApplication(authorizer applicationAuthorizer) *ResourceApplication {
	return &ResourceApplication{authorizer: authorizer, now: time.Now}
}

// List lists the requested resources.
func (a *ResourceApplication) List(ctx context.Context, p session.Principal, org uuid.UUID) ([]ResourceView, error) {
	items := []ResourceView{}
	err := a.authorizer.WithinOrganization(ctx, p, org, authorization.PermissionResourceRead, func(q *tenantdb.Queries) error {
		rows, err := q.ListResources(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			view, err := resourceView(row)
			if err != nil {
				return err
			}
			items = append(items, view)
		}
		return nil
	})
	return items, err
}

// Create creates the requested resource.
func (a *ResourceApplication) Create(ctx context.Context, p session.Principal, org uuid.UUID, input ResourceWrite) (ResourceView, error) {
	input, availability, err := validateResource(input)
	if err != nil {
		return ResourceView{}, err
	}
	var row tenantdb.Resource
	err = a.authorizer.WithinOrganization(ctx, p, org, authorization.PermissionResourceManage, func(q *tenantdb.Queries) error {
		row, err = q.CreateResource(ctx, tenantdb.CreateResourceParams{Name: input.Name, Code: input.Code, Kind: input.Kind, Location: input.Location, Enabled: input.Enabled, Availability: availability})
		return classifyResourceError(err)
	})
	if err != nil {
		return ResourceView{}, err
	}
	return resourceView(row)
}

// Update updates the requested resource.
func (a *ResourceApplication) Update(ctx context.Context, p session.Principal, org, id uuid.UUID, patch ResourcePatch) (ResourceView, error) {
	if patch.Name == nil && patch.Code == nil && patch.Kind == nil && patch.Location == nil && patch.Enabled == nil && patch.Availability == nil {
		return ResourceView{}, ErrInvalidInput
	}
	var result ResourceView
	err := a.authorizer.WithinOrganization(ctx, p, org, authorization.PermissionResourceManage, func(q *tenantdb.Queries) error {
		row, err := q.GetResource(ctx, id)
		if err != nil {
			return classifyResourceError(err)
		}
		current, err := resourceView(row)
		if err != nil {
			return err
		}
		input := current.ResourceWrite
		if patch.Name != nil {
			input.Name = *patch.Name
		}
		if patch.Code != nil {
			input.Code = *patch.Code
		}
		if patch.Kind != nil {
			input.Kind = *patch.Kind
		}
		if patch.Location != nil {
			input.Location = *patch.Location
		}
		if patch.Enabled != nil {
			input.Enabled = *patch.Enabled
		}
		if patch.Availability != nil {
			input.Availability = *patch.Availability
		}
		input, availability, err := validateResource(input)
		if err != nil {
			return err
		}
		updated, err := q.UpdateResource(ctx, tenantdb.UpdateResourceParams{Name: input.Name, Code: input.Code, Kind: input.Kind, Location: input.Location, Enabled: input.Enabled, Availability: availability, PublicID: id})
		if err != nil {
			return classifyResourceError(err)
		}
		result, err = resourceView(updated)
		return err
	})
	return result, err
}

// Delete deletes the requested resource.
func (a *ResourceApplication) Delete(ctx context.Context, p session.Principal, org, id uuid.UUID) error {
	return a.authorizer.WithinOrganization(ctx, p, org, authorization.PermissionResourceManage, func(q *tenantdb.Queries) error {
		count, err := q.DeleteResource(ctx, id)
		if err != nil {
			return classifyResourceError(err)
		}
		if count == 0 {
			return authorization.ErrResourceNotFound
		}
		return nil
	})
}

// ListBookings lists bookings.
func (a *ResourceApplication) ListBookings(ctx context.Context, p session.Principal, org uuid.UUID) ([]BookingView, error) {
	items := []BookingView{}
	err := a.authorizer.WithinOrganization(ctx, p, org, authorization.PermissionResourceRead, func(q *tenantdb.Queries) error {
		rows, err := q.ListBookings(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			view := BookingView{ID: row.PublicID, ResourceID: row.ResourcePublicID, ResourceName: row.ResourceName, StartTime: row.StartTime, EndTime: row.EndTime, DetailsVisible: row.DetailsVisible}
			if row.DetailsVisible {
				view.TeamPublicID = &row.TeamPublicID
				view.TeamName = &row.TeamName
				view.RequestedByName = &row.RequestedByName
				view.Purpose = &row.Purpose
			}
			items = append(items, view)
		}
		return nil
	})
	return items, err
}

// Request handles the request operation.
func (a *ResourceApplication) Request(ctx context.Context, p session.Principal, org, resourceID, teamID uuid.UUID, start, end time.Time, purpose string) (ResourceRequestView, error) {
	purpose = strings.TrimSpace(purpose)
	if resourceID == uuid.Nil || teamID == uuid.Nil || !validInterval(start, end, a.now()) || purpose == "" || utf8.RuneCountInString(purpose) > 1000 {
		return ResourceRequestView{}, ErrInvalidInput
	}
	var createdID uuid.UUID
	err := a.authorizer.WithinTeam(ctx, p, org, teamID, authorization.PermissionResourceRequest, func(q *tenantdb.Queries) error {
		resource, err := q.GetResource(ctx, resourceID)
		if err != nil {
			return classifyResourceError(err)
		}
		if !resource.Enabled {
			return ErrConflict
		}
		windows, err := decodeAvailability(resource.Availability)
		if err != nil || !withinAvailability(start, end, windows) {
			return ErrInvalidInput
		}
		overlap, err := q.ApprovedResourceRequestOverlap(ctx, tenantdb.ApprovedResourceRequestOverlapParams{ResourceID: resource.ID, StartTime: start, EndTime: end, RequestPublicID: uuid.Nil})
		if err != nil {
			return err
		}
		if overlap {
			return ErrConflict
		}
		row, err := q.CreateResourceRequest(ctx, tenantdb.CreateResourceRequestParams{ResourcePublicID: resourceID, TeamPublicID: teamID, StartTime: start.UTC(), EndTime: end.UTC(), Purpose: purpose})
		if err != nil {
			return classifyResourceError(err)
		}
		createdID = row.PublicID
		return nil
	})
	if err != nil {
		return ResourceRequestView{}, err
	}
	return a.getRequest(ctx, p, org, createdID, authorization.PermissionResourceRead)
}

// ListRequests lists requests.
func (a *ResourceApplication) ListRequests(ctx context.Context, p session.Principal, org uuid.UUID) ([]ResourceRequestView, error) {
	items := []ResourceRequestView{}
	err := a.authorizer.WithinOrganization(ctx, p, org, authorization.PermissionResourceRead, func(q *tenantdb.Queries) error {
		rows, err := q.ListResourceRequests(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			items = append(items, requestListView(row))
		}
		return nil
	})
	return items, err
}

// Decide handles the decide operation.
func (a *ResourceApplication) Decide(ctx context.Context, p session.Principal, org, id uuid.UUID, status string) (ResourceRequestView, error) {
	if status != "approved" && status != "rejected" {
		return ResourceRequestView{}, ErrInvalidInput
	}
	var result ResourceRequestView
	err := a.authorizer.WithinOrganization(ctx, p, org, authorization.PermissionResourceDecide, func(q *tenantdb.Queries) error {
		request, err := q.GetResourceRequest(ctx, id)
		if err != nil {
			return classifyResourceError(err)
		}
		if request.Status != "pending" {
			return ErrConflict
		}
		if status == "approved" {
			locked, err := q.LockResourceForDecision(ctx, id)
			if err != nil {
				return classifyResourceError(err)
			}
			windows, decodeErr := decodeAvailability(locked.Availability)
			if decodeErr != nil || !locked.Enabled || !withinAvailability(request.StartTime, request.EndTime, windows) {
				return ErrConflict
			}
			if request.StartTime.Before(a.now().UTC()) {
				return ErrConflict
			}
			overlap, err := q.ApprovedResourceRequestOverlap(ctx, tenantdb.ApprovedResourceRequestOverlapParams{ResourceID: locked.ID, StartTime: request.StartTime, EndTime: request.EndTime, RequestPublicID: id})
			if err != nil {
				return err
			}
			if overlap {
				return ErrConflict
			}
		}
		_, err = q.DecideResourceRequest(ctx, tenantdb.DecideResourceRequestParams{PublicID: id, Status: status})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return classifyResourceError(err)
		}
		refreshed, err := q.GetResourceRequest(ctx, id)
		if err != nil {
			return err
		}
		result = requestView(refreshed)
		return nil
	})
	return result, err
}

// getRequest gets request.
func (a *ResourceApplication) getRequest(ctx context.Context, p session.Principal, org, id uuid.UUID, permission authorization.Permission) (ResourceRequestView, error) {
	var result ResourceRequestView
	err := a.authorizer.WithinOrganization(ctx, p, org, permission, func(q *tenantdb.Queries) error {
		row, err := q.GetResourceRequest(ctx, id)
		if err != nil {
			return classifyResourceError(err)
		}
		result = requestView(row)
		return nil
	})
	return result, err
}

// validateResource validates resource.
func validateResource(input ResourceWrite) (ResourceWrite, []byte, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Code = strings.ToUpper(strings.TrimSpace(input.Code))
	input.Location = strings.TrimSpace(input.Location)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 255 || !resourceCodePattern.MatchString(input.Code) || utf8.RuneCountInString(input.Location) > 255 || !validKind(input.Kind) {
		return ResourceWrite{}, nil, ErrInvalidInput
	}
	seen := map[int]bool{}
	for _, window := range input.Availability {
		if window.Weekday < 0 || window.Weekday > 6 || seen[window.Weekday] || !validClock(window.Start) || !validClock(window.End) || window.Start >= window.End {
			return ResourceWrite{}, nil, ErrInvalidInput
		}
		seen[window.Weekday] = true
	}
	encoded, err := json.Marshal(input.Availability)
	if err != nil {
		return ResourceWrite{}, nil, ErrInvalidInput
	}
	return input, encoded, nil
}

// validKind checks whether kind is valid.
func validKind(kind string) bool {
	return kind == "gpu" || kind == "instrument" || kind == "room" || kind == "workstation"
}

// validClock checks whether clock is valid.
func validClock(value string) bool {
	parsed, err := time.Parse("15:04", value)
	return err == nil && parsed.Format("15:04") == value
}

// validInterval checks whether interval is valid.
func validInterval(start, end, now time.Time) bool {
	return !start.IsZero() && !end.IsZero() && start.Before(end) && !start.Before(now.UTC())
}

// withinAvailability withins availability.
func withinAvailability(start, end time.Time, windows []AvailabilityWindow) bool {
	start = start.UTC()
	end = end.UTC()
	if start.Year() != end.Year() || start.YearDay() != end.YearDay() {
		return false
	}
	clockStart, clockEnd := start.Format("15:04"), end.Format("15:04")
	for _, window := range windows {
		if window.Weekday == int(start.Weekday()) && clockStart >= window.Start && clockEnd <= window.End {
			return true
		}
	}
	return false
}

// decodeAvailability decodes availability.
func decodeAvailability(data []byte) ([]AvailabilityWindow, error) {
	var windows []AvailabilityWindow
	err := json.Unmarshal(data, &windows)
	return windows, err
}

// resourceView resources view.
func resourceView(row tenantdb.Resource) (ResourceView, error) {
	windows, err := decodeAvailability(row.Availability)
	if err != nil {
		return ResourceView{}, err
	}
	return ResourceView{ID: row.PublicID, ResourceWrite: ResourceWrite{Name: row.Name, Code: row.Code, Kind: row.Kind, Location: row.Location, Enabled: row.Enabled, Availability: windows}}, nil
}

// decidedAt decideds at.
func decidedAt(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}

// requestListView requests list view.
func requestListView(row tenantdb.ListResourceRequestsRow) ResourceRequestView {
	return ResourceRequestView{ID: row.PublicID, ResourceID: row.ResourcePublicID, ResourceName: row.ResourceName, TeamPublicID: row.TeamPublicID, TeamName: row.TeamName, RequestedBy: row.RequestedBy, RequestedByName: row.RequestedByName, StartTime: row.StartTime, EndTime: row.EndTime, Purpose: row.Purpose, Status: row.Status, CreatedAt: row.CreatedAt, DecidedAt: decidedAt(row.DecidedAt)}
}

// requestView requests view.
func requestView(row tenantdb.GetResourceRequestRow) ResourceRequestView {
	return ResourceRequestView{ID: row.PublicID, ResourceID: row.ResourcePublicID, ResourceName: row.ResourceName, TeamPublicID: row.TeamPublicID, TeamName: row.TeamName, RequestedBy: row.RequestedBy, RequestedByName: row.RequestedByName, StartTime: row.StartTime, EndTime: row.EndTime, Purpose: row.Purpose, Status: row.Status, CreatedAt: row.CreatedAt, DecidedAt: decidedAt(row.DecidedAt)}
}

// classifyResourceError classifys resource error.
func classifyResourceError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return authorization.ErrResourceNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" || pgErr.Code == "23503" || pgErr.Code == "23001" {
			return ErrConflict
		}
	}
	return err
}
