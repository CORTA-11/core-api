package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
)

type taskService struct {
	executor teamExecutor
}

// NewTaskService creates a task service.
func NewTaskService(executor teamExecutor) TaskService {
	return &taskService{executor: executor}
}

// GetTasks gets tasks.
func (service *taskService) GetTasks(ctx context.Context, team tenancy.TeamContext) ([]Task, error) {
	var rows []tenantdb.Task
	err := service.executor.WithinTeam(ctx, team, func(queries *tenantdb.Queries) error {
		var queryErr error
		rows, queryErr = queries.GetTasks(ctx, listResultLimit)
		return queryErr
	})
	if err != nil {
		return nil, fmt.Errorf("fetch tasks: %w", err)
	}
	tasks := make([]Task, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, mapDBTaskToDomain(row))
	}
	return tasks, nil
}

// CreateTask creates task.
func (service *taskService) CreateTask(ctx context.Context, team tenancy.TeamContext, description, status string) (*Task, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return nil, fmt.Errorf("task description is required")
	}
	status = NormalizeTaskStatus(status)
	if !IsValidTaskStatus(status) {
		return nil, fmt.Errorf("task status is invalid")
	}
	var row tenantdb.Task
	err := service.executor.WithinTeam(ctx, team, func(queries *tenantdb.Queries) error {
		var queryErr error
		row, queryErr = queries.CreateTask(ctx, tenantdb.CreateTaskParams{Description: description, Status: status})
		return queryErr
	})
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	task := mapDBTaskToDomain(row)
	return &task, nil
}

// UpdateTask updates task.
func (service *taskService) UpdateTask(ctx context.Context, team tenancy.TeamContext, taskID uuid.UUID, description, status string) (*Task, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return nil, fmt.Errorf("task description is required")
	}
	status = NormalizeTaskStatus(status)
	if !IsValidTaskStatus(status) {
		return nil, fmt.Errorf("task status is invalid")
	}
	var row tenantdb.Task
	err := service.executor.WithinTeam(ctx, team, func(queries *tenantdb.Queries) error {
		var queryErr error
		row, queryErr = queries.UpdateTask(ctx, tenantdb.UpdateTaskParams{PublicID: taskID, Description: description, Status: status})
		return queryErr
	})
	if err != nil {
		return nil, fmt.Errorf("update task: %w", err)
	}
	task := mapDBTaskToDomain(row)
	return &task, nil
}

// DeleteTask deletes task.
func (service *taskService) DeleteTask(ctx context.Context, team tenancy.TeamContext, taskID uuid.UUID) (*Task, error) {
	var row tenantdb.Task
	err := service.executor.WithinTeam(ctx, team, func(queries *tenantdb.Queries) error {
		var queryErr error
		row, queryErr = queries.DeleteTask(ctx, taskID)
		return queryErr
	})
	if err != nil {
		return nil, fmt.Errorf("delete task: %w", err)
	}
	task := mapDBTaskToDomain(row)
	return &task, nil
}
