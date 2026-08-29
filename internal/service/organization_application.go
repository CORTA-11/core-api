package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrConflict     = errors.New("conflict")
)

const organizationListRouteID = "listOrganizations"

type OrganizationView struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	LifecycleState string     `json:"lifecycle_state"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at"`
	MyRole         string     `json:"my_role"`
}

type OrganizationPage struct {
	Items          []OrganizationView `json:"items"`
	NextCursor     *string            `json:"next_cursor"`
	PreviousCursor *string            `json:"previous_cursor"`
}

type OrganizationMemberView struct {
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	JoinedAt    time.Time `json:"joined_at"`
}

// ListMembers lists members.
func (application *OrganizationApplication) ListMembers(ctx context.Context, principal session.Principal, organizationID uuid.UUID) ([]OrganizationMemberView, error) {
	if !validPrincipal(principal) {
		return nil, authorization.ErrUnauthenticated
	}
	tx, err := application.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := publicdb.New(tx)
	membership, err := queries.GetOrganizationMembership(ctx, publicdb.GetOrganizationMembershipParams{OrganizationPublicID: organizationID, UserPublicID: principal.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, authorization.ErrResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if !authorization.OrganizationAllows(authorization.OrganizationRole(membership.Role), authorization.PermissionOrgMembersRead) {
		return nil, authorization.ErrOperationDenied
	}
	rows, err := queries.ListOrganizationMembersAfter(ctx, publicdb.ListOrganizationMembersAfterParams{OrgID: membership.OrgID, AfterJoinedAt: time.Time{}, AfterUserID: uuid.Nil, Limit: 101})
	if err != nil {
		return nil, err
	}
	views := make([]OrganizationMemberView, 0, len(rows))
	for _, row := range rows {
		views = append(views, OrganizationMemberView{UserID: row.UserID, DisplayName: row.DisplayName, Email: row.Email, Role: row.Role, JoinedAt: row.JoinedAt})
	}
	return views, tx.Commit(ctx)
}

type OrganizationApplication struct {
	pool  *pgxpool.Pool
	codec *pagination.Codec
}

// NewOrganizationApplication creates an organization application.
func NewOrganizationApplication(pool *pgxpool.Pool, codec *pagination.Codec) *OrganizationApplication {
	return &OrganizationApplication{pool: pool, codec: codec}
}

// List lists the requested resources.
func (application *OrganizationApplication) List(
	ctx context.Context,
	principal session.Principal,
	parameters pagination.Parameters,
) (OrganizationPage, error) {
	if application == nil || application.pool == nil || application.codec == nil || !validPrincipal(principal) ||
		parameters.PageSize < 1 || parameters.PageSize > pagination.MaximumPageSize {
		return OrganizationPage{}, authorization.ErrUnauthenticated
	}
	binding := pagination.Binding{RouteID: organizationListRouteID}
	cursor := pagination.Cursor{Direction: pagination.DirectionNext}
	if parameters.Cursor != "" {
		verified, err := application.codec.Verify(parameters.Cursor, binding)
		if err != nil {
			return OrganizationPage{}, ErrInvalidInput
		}
		cursor = verified
	}
	queries := publicdb.New(application.pool)
	// #nosec G115 -- PageSize was validated at no more than 100 above.
	limit := int32(parameters.PageSize + 1)
	var rows []publicdb.Org
	var err error
	if cursor.Direction == pagination.DirectionPrevious {
		rows, err = queries.ListOrganizationsForUserBefore(ctx, publicdb.ListOrganizationsForUserBeforeParams{
			UserPublicID: principal.UserID, BeforeCreatedAt: cursor.Sort.Timestamp,
			BeforePublicID: cursor.Sort.ID, Limit: limit,
		})
	} else {
		rows, err = queries.ListOrganizationsForUserAfter(ctx, publicdb.ListOrganizationsForUserAfterParams{
			UserPublicID: principal.UserID, AfterCreatedAt: cursor.Sort.Timestamp,
			AfterPublicID: cursor.Sort.ID, Limit: limit,
		})
	}
	if err != nil {
		return OrganizationPage{}, fmt.Errorf("list organizations: %w", err)
	}
	if cursor.Direction == pagination.DirectionPrevious {
		slices.Reverse(rows)
	}
	more := len(rows) > parameters.PageSize
	if more {
		if cursor.Direction == pagination.DirectionPrevious {
			rows = rows[1:]
		} else {
			rows = rows[:parameters.PageSize]
		}
	}
	page := OrganizationPage{Items: make([]OrganizationView, 0, len(rows))}
	for _, row := range rows {
		view := organizationView(row)
		membership, lookupErr := queries.GetOrganizationMembership(ctx, publicdb.GetOrganizationMembershipParams{OrganizationPublicID: row.PublicID, UserPublicID: principal.UserID})
		if lookupErr != nil {
			return OrganizationPage{}, fmt.Errorf("load organization role: %w", lookupErr)
		}
		view.MyRole = membership.Role
		page.Items = append(page.Items, view)
	}
	if len(rows) == 0 {
		return page, nil
	}
	if parameters.Cursor != "" {
		value, issueErr := application.codec.Issue(binding, pagination.Cursor{Direction: pagination.DirectionPrevious,
			Sort: pagination.SortKey{Timestamp: rows[0].CreatedAt, ID: rows[0].PublicID}})
		if issueErr != nil {
			return OrganizationPage{}, issueErr
		}
		page.PreviousCursor = &value
	}
	if more || cursor.Direction == pagination.DirectionPrevious {
		last := rows[len(rows)-1]
		value, issueErr := application.codec.Issue(binding, pagination.Cursor{Direction: pagination.DirectionNext,
			Sort: pagination.SortKey{Timestamp: last.CreatedAt, ID: last.PublicID}})
		if issueErr != nil {
			return OrganizationPage{}, issueErr
		}
		page.NextCursor = &value
	}
	return page, nil
}

// Create creates the requested resource.
func (application *OrganizationApplication) Create(
	ctx context.Context,
	principal session.Principal,
	name string,
) (OrganizationView, error) {
	name, err := normalizeResourceName(name)
	if err != nil {
		return OrganizationView{}, err
	}
	if application == nil || application.pool == nil || !validPrincipal(principal) {
		return OrganizationView{}, authorization.ErrUnauthenticated
	}
	tx, err := application.pool.Begin(ctx)
	if err != nil {
		return OrganizationView{}, fmt.Errorf("begin organization creation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := publicdb.New(tx)
	user, err := queries.GetUserByID(ctx, principal.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrganizationView{}, authorization.ErrUnauthenticated
	}
	if err != nil {
		return OrganizationView{}, fmt.Errorf("load organization creator: %w", err)
	}
	publicID := uuid.New()
	organization, err := queries.CreateOrg(ctx, publicdb.CreateOrgParams{
		Name: name, PublicID: publicID, SchemaName: tenancy.CanonicalSchema(publicID.String()),
	})
	if err != nil {
		return OrganizationView{}, classifyConflict(err)
	}
	if _, err := queries.AddOrganizationOwnerMembership(ctx, publicdb.AddOrganizationOwnerMembershipParams{
		OrgID: organization.ID, UserID: user.ID,
	}); err != nil {
		return OrganizationView{}, fmt.Errorf("create organization owner: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return OrganizationView{}, fmt.Errorf("commit organization creation: %w", err)
	}
	view := organizationView(organization)
	view.MyRole = "owner"
	return view, nil
}

// Get retrieves the requested resource.
func (application *OrganizationApplication) Get(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
) (OrganizationView, error) {
	return application.mutate(ctx, principal, organizationID, authorization.PermissionOrgRead,
		func(queries *publicdb.Queries) (publicdb.Org, error) {
			return queries.GetOrganizationByPublicIDIncludingDeleted(ctx, organizationID)
		})
}

// Update updates the requested resource.
func (application *OrganizationApplication) Update(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
	name string,
) (OrganizationView, error) {
	name, err := normalizeResourceName(name)
	if err != nil {
		return OrganizationView{}, err
	}
	return application.mutate(ctx, principal, organizationID, authorization.PermissionOrgUpdate,
		func(queries *publicdb.Queries) (publicdb.Org, error) {
			row, updateErr := queries.UpdateOrganizationIncludingDeleting(ctx, publicdb.UpdateOrganizationIncludingDeletingParams{
				Name: name, PublicID: organizationID,
			})
			return row, classifyConflict(updateErr)
		})
}

// Delete deletes the requested resource.
func (application *OrganizationApplication) Delete(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
) error {
	_, err := application.mutate(ctx, principal, organizationID, authorization.PermissionOrgDelete,
		func(queries *publicdb.Queries) (publicdb.Org, error) {
			return queries.SoftDeleteOrg(ctx, organizationID)
		})
	return err
}

// Restore handles the restore operation.
func (application *OrganizationApplication) Restore(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
) (OrganizationView, error) {
	return application.mutate(ctx, principal, organizationID, authorization.PermissionOrgRestore,
		func(queries *publicdb.Queries) (publicdb.Org, error) { return queries.RestoreOrg(ctx, organizationID) })
}

// mutate handles the mutate operation.
func (application *OrganizationApplication) mutate(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
	permission authorization.Permission,
	operation func(*publicdb.Queries) (publicdb.Org, error),
) (OrganizationView, error) {
	if application == nil || application.pool == nil || !validPrincipal(principal) {
		return OrganizationView{}, authorization.ErrUnauthenticated
	}
	if organizationID == uuid.Nil || operation == nil {
		return OrganizationView{}, authorization.ErrResourceNotFound
	}
	tx, err := application.pool.Begin(ctx)
	if err != nil {
		return OrganizationView{}, fmt.Errorf("begin organization operation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := publicdb.New(tx)
	membership, err := queries.GetOrganizationMembershipIncludingDeleted(ctx,
		publicdb.GetOrganizationMembershipIncludingDeletedParams{
			OrganizationPublicID: organizationID, UserPublicID: principal.UserID,
		})
	if errors.Is(err, pgx.ErrNoRows) {
		return OrganizationView{}, authorization.ErrResourceNotFound
	}
	if err != nil {
		return OrganizationView{}, fmt.Errorf("load organization membership: %w", err)
	}
	if !authorization.OrganizationAllows(authorization.OrganizationRole(membership.Role), permission) {
		return OrganizationView{}, authorization.ErrOperationDenied
	}
	if permission != authorization.PermissionOrgRead {
		owners, countErr := queries.CountOrganizationOwners(ctx, membership.OrgID)
		if countErr != nil {
			return OrganizationView{}, fmt.Errorf("count organization owners: %w", countErr)
		}
		if owners < 1 {
			return OrganizationView{}, authorization.ErrOperationDenied
		}
	}
	row, err := operation(queries)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrganizationView{}, authorization.ErrResourceNotFound
	}
	if err != nil {
		return OrganizationView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OrganizationView{}, fmt.Errorf("commit organization operation: %w", err)
	}
	view := organizationView(row)
	view.MyRole = membership.Role
	return view, nil
}

// normalizeResourceName normalizes resource name.
func normalizeResourceName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 200 {
		return "", ErrInvalidInput
	}
	return name, nil
}

// validPrincipal checks whether principal is valid.
func validPrincipal(principal session.Principal) bool {
	return principal.UserID != uuid.Nil && principal.SessionID != uuid.Nil
}

// organizationView organizations view.
func organizationView(row publicdb.Org) OrganizationView {
	view := OrganizationView{ID: row.PublicID, Name: row.Name, LifecycleState: row.LifecycleState,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.DeletedAt.Valid {
		deletedAt := row.DeletedAt.Time
		view.DeletedAt = &deletedAt
	}
	return view
}

// classifyConflict classifys conflict.
func classifyConflict(err error) error {
	if err == nil {
		return nil
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23505" {
		return ErrConflict
	}
	return err
}
