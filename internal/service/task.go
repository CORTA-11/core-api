package service

import (
	"context"
	"time"
)

type Task struct {
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TaskService interface {
	GetTasks(ctx context.Context, schema string, teamID int) ([]Task, error)
	CreateTask(ctx context.Context, schema string, teamID int, desc string) (*Task, error)
}
