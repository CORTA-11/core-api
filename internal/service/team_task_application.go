package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	teamListRouteID = "listTeams"
	taskListRouteID = "listTasks"
)

type applicationAuthorizer interface {
	WithinOrganization(context.Context, session.Principal, uuid.UUID, authorization.Permission, authorization.TenantCallback) error
	WithinTeam(context.Context, session.Principal, uuid.UUID, uuid.UUID, authorization.Permission, authorization.TenantCallback) error
}

type TeamView struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	MyRole    string    `json:"my_role"`
}

type TeamPage struct {
	Items          []TeamView `json:"items"`
	NextCursor     *string    `json:"next_cursor"`
	PreviousCursor *string    `json:"previous_cursor"`
}

type TeamMemberView struct {
	UserID   uuid.UUID `json:"user_id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

func (application *TeamTaskApplication) ListTeamMembers(ctx context.Context, principal session.Principal, organizationID, teamID uuid.UUID) ([]TeamMemberView, error) {
	var result []TeamMemberView
	err := application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionTeamMembersRead, func(queries *tenantdb.Queries) error {
		rows, err := queries.ListBoundTeamMembers(ctx, 101)
		if err != nil {
			return err
		}
		result = make([]TeamMemberView, 0, len(rows))
		for _, row := range rows {
			result = append(result, TeamMemberView{
				UserID: row.UserPublicID, Name: row.DisplayName, Email: row.Email,
				Role: row.Role, JoinedAt: row.JoinedAt,
			})
		}
		return nil
	})
	return result, err
}

func (application *TeamTaskApplication) AddTeamMember(ctx context.Context, principal session.Principal, organizationID, teamID uuid.UUID, email string) (TeamMemberView, error) {
	canonical, err := (identity.EmailCanonicalizer{}).Canonicalize(email)
	if err != nil {
		return TeamMemberView{}, ErrInvalidInput
	}
	var result TeamMemberView
	err = application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionTeamMembersManage, func(queries *tenantdb.Queries) error {
		row, queryErr := queries.AddTeamContributor(ctx, canonical.Key)
		if queryErr != nil {
			return classifyTeamMemberError(queryErr)
		}
		result = TeamMemberView{UserID: row.UserPublicID, Role: row.Role, JoinedAt: row.CreatedAt}
		return nil
	})
	return result, err
}

func classifyTeamMemberError(err error) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		if databaseError.Code == "23505" {
			return ErrConflict
		}
		if databaseError.Code == "P0002" {
			return authorization.ErrResourceNotFound
		}
	}
	return err
}

type TaskView struct {
	ID          uuid.UUID `json:"id"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TaskPage struct {
	Items          []TaskView `json:"items"`
	NextCursor     *string    `json:"next_cursor"`
	PreviousCursor *string    `json:"previous_cursor"`
}

type TeamTaskApplication struct {
	authorizer applicationAuthorizer
	codec      *pagination.Codec
}

func NewTeamTaskApplication(authorizer applicationAuthorizer, codec *pagination.Codec) *TeamTaskApplication {
	return &TeamTaskApplication{authorizer: authorizer, codec: codec}
}

func (application *TeamTaskApplication) ListTeams(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
	parameters pagination.Parameters,
) (TeamPage, error) {
	if err := application.valid(principal, organizationID, parameters); err != nil {
		return TeamPage{}, err
	}
	binding := pagination.Binding{RouteID: teamListRouteID, OrganizationID: &organizationID}
	cursor, err := application.cursor(parameters.Cursor, binding)
	if err != nil {
		return TeamPage{}, err
	}
	var rows []tenantdb.Team
	err = application.authorizer.WithinOrganization(ctx, principal, organizationID, authorization.PermissionOrgRead,
		func(queries *tenantdb.Queries) error {
			// #nosec G115 -- PageSize was validated at no more than 100 above.
			limit := int32(parameters.PageSize + 1)
			if cursor.Direction == pagination.DirectionPrevious {
				var queryErr error
				rows, queryErr = queries.GetTeamsBefore(ctx, tenantdb.GetTeamsBeforeParams{
					BeforeCreatedAt: cursor.Sort.Timestamp, BeforePublicID: cursor.Sort.ID, Limit: limit,
				})
				return queryErr
			}
			var queryErr error
			rows, queryErr = queries.GetTeamsAfter(ctx, tenantdb.GetTeamsAfterParams{
				AfterCreatedAt: cursor.Sort.Timestamp, AfterPublicID: cursor.Sort.ID, Limit: limit,
			})
			return queryErr
		})
	if err != nil {
		return TeamPage{}, err
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
	page := TeamPage{Items: make([]TeamView, 0, len(rows))}
	for _, row := range rows {
		view := teamView(row)
		_ = application.authorizer.WithinTeam(ctx, principal, organizationID, row.PublicID, authorization.PermissionTeamRead, func(queries *tenantdb.Queries) error {
			role, roleErr := queries.GetCurrentTeamRole(ctx, row.ID)
			view.MyRole = role
			return roleErr
		})
		page.Items = append(page.Items, view)
	}
	if err := application.teamLinks(&page, rows, binding, cursor.Direction, parameters.Cursor != "", more); err != nil {
		return TeamPage{}, err
	}
	return page, nil
}

func (application *TeamTaskApplication) CreateTeam(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
	name string,
	leaderEmail string,
) (TeamView, error) {
	name, err := normalizeResourceName(name)
	if err != nil {
		return TeamView{}, err
	}
	leaderEmail = strings.TrimSpace(leaderEmail)
	if leaderEmail == "" {
		return TeamView{}, ErrInvalidInput
	}
	if application == nil || application.authorizer == nil || !validPrincipal(principal) || organizationID == uuid.Nil {
		return TeamView{}, authorization.ErrResourceNotFound
	}
	var row tenantdb.Team
	err = application.authorizer.WithinOrganization(ctx, principal, organizationID, authorization.PermissionTeamCreate,
		func(queries *tenantdb.Queries) error {
			var queryErr error
			row, queryErr = queries.CreateTeamWithCreator(ctx, tenantdb.CreateTeamWithCreatorParams{
				Name: name, Slug: deterministicTeamSlug(name), LeaderEmail: leaderEmail,
			})
			return classifyConflict(queryErr)
		})
	if err != nil {
		return TeamView{}, err
	}
	view := teamView(row)
	view.MyRole = ""
	return view, nil
}

func (application *TeamTaskApplication) ListTasks(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
	teamID uuid.UUID,
	parameters pagination.Parameters,
) (TaskPage, error) {
	if err := application.valid(principal, organizationID, parameters); err != nil {
		return TaskPage{}, err
	}
	if teamID == uuid.Nil {
		return TaskPage{}, authorization.ErrResourceNotFound
	}
	binding := pagination.Binding{RouteID: taskListRouteID, OrganizationID: &organizationID, TeamID: &teamID}
	cursor, err := application.cursor(parameters.Cursor, binding)
	if err != nil {
		return TaskPage{}, err
	}
	var rows []tenantdb.Task
	err = application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionTaskRead,
		func(queries *tenantdb.Queries) error {
			// #nosec G115 -- PageSize was validated at no more than 100 above.
			limit := int32(parameters.PageSize + 1)
			if cursor.Direction == pagination.DirectionPrevious {
				var queryErr error
				rows, queryErr = queries.GetTasksBefore(ctx, tenantdb.GetTasksBeforeParams{
					BeforeCreatedAt: cursor.Sort.Timestamp, BeforePublicID: cursor.Sort.ID, Limit: limit,
				})
				return queryErr
			}
			var queryErr error
			rows, queryErr = queries.GetTasksAfter(ctx, tenantdb.GetTasksAfterParams{
				AfterCreatedAt: cursor.Sort.Timestamp, AfterPublicID: cursor.Sort.ID, Limit: limit,
			})
			return queryErr
		})
	if err != nil {
		return TaskPage{}, err
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
	page := TaskPage{Items: make([]TaskView, 0, len(rows))}
	for _, row := range rows {
		page.Items = append(page.Items, taskView(row))
	}
	if err := application.taskLinks(&page, rows, binding, cursor.Direction, parameters.Cursor != "", more); err != nil {
		return TaskPage{}, err
	}
	return page, nil
}

func (application *TeamTaskApplication) CreateTask(
	ctx context.Context, principal session.Principal, organizationID, teamID uuid.UUID, description, status string,
) (TaskView, error) {
	description, status, err := validateTaskWrite(description, status)
	if err != nil {
		return TaskView{}, err
	}
	if application == nil || application.authorizer == nil || !validPrincipal(principal) ||
		organizationID == uuid.Nil || teamID == uuid.Nil {
		return TaskView{}, authorization.ErrResourceNotFound
	}
	var row tenantdb.Task
	err = application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionTaskCreate,
		func(queries *tenantdb.Queries) error {
			var queryErr error
			row, queryErr = queries.CreateTask(ctx, tenantdb.CreateTaskParams{Description: description, Status: status})
			return classifyConflict(queryErr)
		})
	if err != nil {
		return TaskView{}, err
	}
	return taskView(row), nil
}

func (application *TeamTaskApplication) UpdateTask(
	ctx context.Context, principal session.Principal, organizationID, teamID, taskID uuid.UUID, description, status string,
) (TaskView, error) {
	description, status, err := validateTaskWrite(description, status)
	if err != nil {
		return TaskView{}, err
	}
	if taskID == uuid.Nil {
		return TaskView{}, authorization.ErrResourceNotFound
	}
	if application == nil || application.authorizer == nil || !validPrincipal(principal) ||
		organizationID == uuid.Nil || teamID == uuid.Nil {
		return TaskView{}, authorization.ErrResourceNotFound
	}
	var row tenantdb.Task
	err = application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionTaskUpdate,
		func(queries *tenantdb.Queries) error {
			var queryErr error
			row, queryErr = queries.UpdateTask(ctx, tenantdb.UpdateTaskParams{
				PublicID: taskID, Description: description, Status: status,
			})
			if errors.Is(queryErr, pgx.ErrNoRows) {
				return authorization.ErrResourceNotFound
			}
			return classifyConflict(queryErr)
		})
	if err != nil {
		return TaskView{}, err
	}
	return taskView(row), nil
}

func (application *TeamTaskApplication) DeleteTask(
	ctx context.Context, principal session.Principal, organizationID, teamID, taskID uuid.UUID,
) error {
	if application == nil || application.authorizer == nil || !validPrincipal(principal) ||
		organizationID == uuid.Nil || teamID == uuid.Nil || taskID == uuid.Nil {
		return authorization.ErrResourceNotFound
	}
	return application.authorizer.WithinTeam(ctx, principal, organizationID, teamID, authorization.PermissionTaskDelete,
		func(queries *tenantdb.Queries) error {
			_, err := queries.DeleteTask(ctx, taskID)
			if errors.Is(err, pgx.ErrNoRows) {
				return authorization.ErrResourceNotFound
			}
			return err
		})
}

func (application *TeamTaskApplication) valid(
	principal session.Principal, organizationID uuid.UUID, parameters pagination.Parameters,
) error {
	if application == nil || application.authorizer == nil || application.codec == nil || !validPrincipal(principal) {
		return authorization.ErrUnauthenticated
	}
	if organizationID == uuid.Nil {
		return authorization.ErrResourceNotFound
	}
	if parameters.PageSize < 1 || parameters.PageSize > pagination.MaximumPageSize {
		return ErrInvalidInput
	}
	return nil
}

func (application *TeamTaskApplication) cursor(token string, binding pagination.Binding) (pagination.Cursor, error) {
	if token == "" {
		return pagination.Cursor{Direction: pagination.DirectionNext}, nil
	}
	cursor, err := application.codec.Verify(token, binding)
	if err != nil {
		return pagination.Cursor{}, ErrInvalidInput
	}
	return cursor, nil
}

func (application *TeamTaskApplication) teamLinks(
	page *TeamPage, rows []tenantdb.Team, binding pagination.Binding,
	direction pagination.Direction, hadCursor, more bool,
) error {
	if len(rows) == 0 {
		return nil
	}
	if hadCursor {
		value, err := application.codec.Issue(binding, pagination.Cursor{Direction: pagination.DirectionPrevious,
			Sort: pagination.SortKey{Timestamp: rows[0].CreatedAt, ID: rows[0].PublicID}})
		if err != nil {
			return err
		}
		page.PreviousCursor = &value
	}
	if more || direction == pagination.DirectionPrevious {
		last := rows[len(rows)-1]
		value, err := application.codec.Issue(binding, pagination.Cursor{Direction: pagination.DirectionNext,
			Sort: pagination.SortKey{Timestamp: last.CreatedAt, ID: last.PublicID}})
		if err != nil {
			return err
		}
		page.NextCursor = &value
	}
	return nil
}

func (application *TeamTaskApplication) taskLinks(
	page *TaskPage, rows []tenantdb.Task, binding pagination.Binding,
	direction pagination.Direction, hadCursor, more bool,
) error {
	if len(rows) == 0 {
		return nil
	}
	if hadCursor {
		value, err := application.codec.Issue(binding, pagination.Cursor{Direction: pagination.DirectionPrevious,
			Sort: pagination.SortKey{Timestamp: rows[0].CreatedAt, ID: rows[0].PublicID}})
		if err != nil {
			return err
		}
		page.PreviousCursor = &value
	}
	if more || direction == pagination.DirectionPrevious {
		last := rows[len(rows)-1]
		value, err := application.codec.Issue(binding, pagination.Cursor{Direction: pagination.DirectionNext,
			Sort: pagination.SortKey{Timestamp: last.CreatedAt, ID: last.PublicID}})
		if err != nil {
			return err
		}
		page.NextCursor = &value
	}
	return nil
}

func deterministicTeamSlug(name string) string {
	var builder strings.Builder
	separator := false
	for _, character := range strings.ToLower(strings.TrimSpace(name)) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if separator && builder.Len() > 0 && builder.Len() < 99 {
				builder.WriteByte('-')
			}
			if builder.Len() < 100 {
				builder.WriteRune(character)
			}
			separator = false
		} else if unicode.IsSpace(character) || unicode.IsPunct(character) || character > unicode.MaxASCII {
			separator = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug != "" {
		return slug
	}
	digest := sha256.Sum256([]byte(name))
	return "team-" + hex.EncodeToString(digest[:6])
}

func validateTaskWrite(description, status string) (string, string, error) {
	description = strings.TrimSpace(description)
	if description == "" || !utf8.ValidString(description) || utf8.RuneCountInString(description) > 4096 {
		return "", "", ErrInvalidInput
	}
	switch status {
	case "todo", "in_progress", "done":
		return description, status, nil
	default:
		return "", "", ErrInvalidInput
	}
}

func teamView(row tenantdb.Team) TeamView {
	return TeamView{ID: row.PublicID, Name: row.Name, Slug: row.Slug,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func taskView(row tenantdb.Task) TaskView {
	return TaskView{ID: row.PublicID, Description: row.Description, Status: row.Status,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
