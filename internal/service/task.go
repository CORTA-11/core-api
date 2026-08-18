package service

import (
	"context"
	"strings"
	"time"

	"github.com/CORTA-11/core-api/internal/repository"
)

type Task struct {
	ID          int64     `json:"id"`
	TeamID      int       `json:"team_id"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

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

func IsValidTaskStatus(status string) bool {
	switch NormalizeTaskStatus(status) {
	case "todo", "in_progress", "done":
		return true
	default:
		return false
	}
}

func mapDBTaskToDomain(row repository.Task) Task {
	teamID := 0
	if row.TeamID.Valid {
		teamID = int(row.TeamID.Int64)
	}

	return Task{
		ID:          row.ID,
		TeamID:      teamID,
		Description: row.Description,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

type TaskService interface {
	GetTasks(ctx context.Context, schema string, teamID int) ([]Task, error)
	CreateTask(ctx context.Context, schema string, teamID int, desc string, status string) (*Task, error)
	UpdateTask(ctx context.Context, schema string, teamID int, taskID int, desc string, status string) (*Task, error)
	DeleteTask(ctx context.Context, schema string, teamID int, taskID int) (*Task, error)
}
