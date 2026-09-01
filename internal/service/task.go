package service

import (
	"context"
	"strings"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
)

type Task struct {
	PublicID    uuid.UUID `json:"public_id"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NormalizeTaskStatus normalizes task status.
func NormalizeTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "todo":
		return "todo"
	case "in_progress", "in-progress":
		return "in_progress"
	case "done":
		return "done"
	default:
		return strings.TrimSpace(status)
	}
}

// IsValidTaskStatus checks whether valid task status.
func IsValidTaskStatus(status string) bool {
	switch NormalizeTaskStatus(status) {
	case "todo", "in_progress", "done":
		return true
	default:
		return false
	}
}

// mapDBTaskToDomain maps dbta sk to domain.
func mapDBTaskToDomain(row tenantdb.Task) Task {
	return Task{
		PublicID:    row.PublicID,
		Description: row.Description,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

type TaskService interface {
	GetTasks(ctx context.Context, team tenancy.TeamContext) ([]Task, error)
	CreateTask(ctx context.Context, team tenancy.TeamContext, desc string, status string) (*Task, error)
	UpdateTask(ctx context.Context, team tenancy.TeamContext, taskID uuid.UUID, desc string, status string) (*Task, error)
	DeleteTask(ctx context.Context, team tenancy.TeamContext, taskID uuid.UUID) (*Task, error)
}

type teamExecutor interface {
	WithinTeam(context.Context, tenancy.TeamContext, func(*tenantdb.Queries) error) error
}
