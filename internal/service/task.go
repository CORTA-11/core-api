package service

import (
	"context"
	"time"

	"github.com/CORTA-11/core-api/internal/repository"
)

type Task struct {
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func mapDBTaskToDomain(row repository.Task) Task {
	return Task{
		Description: row.Description,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

type TaskService interface {
	GetTasks(ctx context.Context, schema string, teamID int) ([]Task, error)
	CreateTask(ctx context.Context, schema string, teamID int, desc string) (*Task, error)
}
