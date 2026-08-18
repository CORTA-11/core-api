package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type taskService struct {
	pool    *pgxpool.Pool
	queries *repository.Queries
}

func NewTaskService(pool *pgxpool.Pool, queries *repository.Queries) TaskService {
	return &taskService{
		pool:    pool,
		queries: queries,
	}
}

func (t *taskService) GetTasks(ctx context.Context, schema string, teamID int) ([]Task, error) {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %q", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := setSchema(ctx, tx, schema); err != nil {
		return nil, fmt.Errorf("failed to set search_path: %q", err)
	}

	qtx := t.queries.WithTx(tx)

	tasks, err := qtx.GetTasks(ctx, int64(teamID))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tasks: %q", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %q", err)
	}

	domainTasks := make([]Task, 0, len(tasks))

	for _, task := range tasks {
		domainTasks = append(domainTasks, mapDBTaskToDomain(task))
	}

	return domainTasks, nil
}

func (t *taskService) CreateTask(ctx context.Context, schema string, teamID int, desc string, status string) (*Task, error) {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return nil, fmt.Errorf("task description is required")
	}

	status = NormalizeTaskStatus(status)
	if !IsValidTaskStatus(status) {
		return nil, fmt.Errorf("task status is invalid")
	}

	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %q", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := setSchema(ctx, tx, schema); err != nil {
		return nil, fmt.Errorf("failed to set search_path: %q", err)
	}

	qtx := t.queries.WithTx(tx)

	task, err := qtx.CreateTask(ctx, repository.CreateTaskParams{
		TeamID:      intToPgtypeInt8(teamID),
		Description: desc,
		Status:      status,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %q", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %q", err)
	}

	ret := mapDBTaskToDomain(task)

	return &ret, nil
}

func (t *taskService) UpdateTask(ctx context.Context, schema string, teamID int, taskID int, desc string, status string) (*Task, error) {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return nil, fmt.Errorf("task description is required")
	}

	status = NormalizeTaskStatus(status)
	if !IsValidTaskStatus(status) {
		return nil, fmt.Errorf("task status is invalid")
	}

	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %q", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := setSchema(ctx, tx, schema); err != nil {
		return nil, fmt.Errorf("failed to set search_path: %q", err)
	}

	qtx := t.queries.WithTx(tx)

	task, err := qtx.UpdateTask(ctx, repository.UpdateTaskParams{
		ID:          int64(taskID),
		TeamID:      intToPgtypeInt8(teamID),
		Description: desc,
		Status:      status,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update task: %q", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %q", err)
	}

	ret := mapDBTaskToDomain(task)
	return &ret, nil
}

func (t *taskService) DeleteTask(ctx context.Context, schema string, teamID int, taskID int) (*Task, error) {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %q", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := setSchema(ctx, tx, schema); err != nil {
		return nil, fmt.Errorf("failed to set search_path: %q", err)
	}

	qtx := t.queries.WithTx(tx)

	task, err := qtx.DeleteTask(ctx, repository.DeleteTaskParams{
		ID:     int64(taskID),
		TeamID: intToPgtypeInt8(teamID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to delete task: %q", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %q", err)
	}

	ret := mapDBTaskToDomain(task)
	return &ret, nil
}
