//go:build isolation

package integration_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	tenantContextStressOperations = 4_000
	tenantContextStressWorkers    = 2
	tenantContextStressTimeout    = 60 * time.Second
)

type tenantContextStressScope struct {
	name   string
	team   tenancy.TeamContext
	taskID uuid.UUID
}

type tenantContextStressResult struct {
	worker      int
	completed   int
	scopeCounts [4]int
	err         error
}

func TestTenantContextStressCleansPooledConnections(t *testing.T) {
	fixture := newTenantBoundaryFixture(t)
	scopes := []tenantContextStressScope{
		{name: "alpha-one", team: fixture.resolveTeam(t, fixture.users.shared, fixture.orgs[0], fixture.orgs[0].teams[0]), taskID: fixture.orgs[0].teams[0].taskID},
		{name: "alpha-two", team: fixture.resolveTeam(t, fixture.users.shared, fixture.orgs[0], fixture.orgs[0].teams[1]), taskID: fixture.orgs[0].teams[1].taskID},
		{name: "beta-one", team: fixture.resolveTeam(t, fixture.users.shared, fixture.orgs[1], fixture.orgs[1].teams[0]), taskID: fixture.orgs[1].teams[0].taskID},
		{name: "beta-two", team: fixture.resolveTeam(t, fixture.users.shared, fixture.orgs[1], fixture.orgs[1].teams[1]), taskID: fixture.orgs[1].teams[1].taskID},
	}

	ctx, cancel := context.WithTimeout(context.Background(), tenantContextStressTimeout)
	defer cancel()
	exerciseBothRuntimePoolConnections(t, ctx, fixture, scopes)

	results := make(chan tenantContextStressResult, tenantContextStressWorkers)
	var workers sync.WaitGroup
	workers.Add(tenantContextStressWorkers)
	for worker := 0; worker < tenantContextStressWorkers; worker++ {
		go func(worker int) {
			defer workers.Done()
			result := tenantContextStressResult{worker: worker}
			for operation := worker; operation < tenantContextStressOperations; operation += tenantContextStressWorkers {
				scopeIndex := operation % len(scopes)
				scope := scopes[scopeIndex]
				tasks, err := fixture.taskService.GetTasks(ctx, scope.team)
				if err != nil {
					result.err = fmt.Errorf("worker %d operation %d scope %s: %w", worker, operation, scope.name, err)
					break
				}
				gotTaskID := uuid.Nil
				if len(tasks) != 0 {
					gotTaskID = tasks[0].PublicID
				}
				if len(tasks) != 1 || tasks[0].PublicID != scope.taskID {
					result.err = fmt.Errorf(
						"worker %d operation %d scope %s: got %d tasks with first ID %s",
						worker, operation, scope.name, len(tasks), gotTaskID,
					)
					break
				}
				result.completed++
				result.scopeCounts[scopeIndex]++
			}
			results <- result
		}(worker)
	}
	workers.Wait()
	close(results)

	completed := 0
	scopeCounts := [4]int{}
	workerErrors := make([]error, 0, tenantContextStressWorkers)
	for result := range results {
		completed += result.completed
		for scopeIndex, count := range result.scopeCounts {
			scopeCounts[scopeIndex] += count
		}
		if result.err != nil {
			workerErrors = append(workerErrors, result.err)
		}
	}
	require.Empty(t, workerErrors)
	assert.Equal(t, tenantContextStressOperations, completed)
	assert.Equal(t, [4]int{1_000, 1_000, 1_000, 1_000}, scopeCounts)
	assert.EqualValues(t, 2, fixture.runtimePool.Stat().TotalConns())
	assertRuntimePoolClean(t, fixture.runtimePool)
}

func exerciseBothRuntimePoolConnections(
	t *testing.T,
	ctx context.Context,
	fixture *tenantBoundaryFixture,
	scopes []tenantContextStressScope,
) {
	t.Helper()
	ready := make(chan int, tenantContextStressWorkers)
	release := make(chan struct{})
	results := make(chan error, tenantContextStressWorkers)
	for worker := 0; worker < tenantContextStressWorkers; worker++ {
		go func(worker int) {
			results <- fixture.executor.WithinTeam(ctx, scopes[worker].team, func(queries *tenantdb.Queries) error {
				ready <- worker
				select {
				case <-release:
					_, err := queries.IsolationProbeTasks(ctx, 1)
					return err
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		}(worker)
	}

	readyWorkers := make(map[int]struct{}, tenantContextStressWorkers)
	for len(readyWorkers) < tenantContextStressWorkers {
		select {
		case worker := <-ready:
			readyWorkers[worker] = struct{}{}
		case err := <-results:
			cancelRuntimePoolConnectionExercise(release)
			require.NoError(t, err)
		case <-ctx.Done():
			cancelRuntimePoolConnectionExercise(release)
			require.NoError(t, ctx.Err())
		}
	}
	assert.EqualValues(t, 2, fixture.runtimePool.Stat().TotalConns())
	assert.EqualValues(t, 2, fixture.runtimePool.Stat().AcquiredConns())
	close(release)
	for range tenantContextStressWorkers {
		require.NoError(t, <-results)
	}
}

func cancelRuntimePoolConnectionExercise(release chan struct{}) {
	select {
	case <-release:
	default:
		close(release)
	}
}
