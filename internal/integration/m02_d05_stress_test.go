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
	d05StressOperations = 4_000
	d05StressWorkers    = 2
	d05StressTimeout    = 60 * time.Second
)

type d05StressScope struct {
	name   string
	team   tenancy.TeamContext
	taskID uuid.UUID
}

type d05StressResult struct {
	worker      int
	completed   int
	scopeCounts [4]int
	err         error
}

func TestM02D05StressPooledTenantContextCleanup(t *testing.T) {
	fixture := newD05Fixture(t)
	scopes := []d05StressScope{
		{name: "alpha-one", team: fixture.resolveTeam(t, fixture.users.shared, fixture.orgs[0], fixture.orgs[0].teams[0]), taskID: fixture.orgs[0].teams[0].taskID},
		{name: "alpha-two", team: fixture.resolveTeam(t, fixture.users.shared, fixture.orgs[0], fixture.orgs[0].teams[1]), taskID: fixture.orgs[0].teams[1].taskID},
		{name: "beta-one", team: fixture.resolveTeam(t, fixture.users.shared, fixture.orgs[1], fixture.orgs[1].teams[0]), taskID: fixture.orgs[1].teams[0].taskID},
		{name: "beta-two", team: fixture.resolveTeam(t, fixture.users.shared, fixture.orgs[1], fixture.orgs[1].teams[1]), taskID: fixture.orgs[1].teams[1].taskID},
	}

	ctx, cancel := context.WithTimeout(context.Background(), d05StressTimeout)
	defer cancel()
	exerciseBothD05RuntimeConnections(t, ctx, fixture, scopes)

	results := make(chan d05StressResult, d05StressWorkers)
	var workers sync.WaitGroup
	workers.Add(d05StressWorkers)
	for worker := 0; worker < d05StressWorkers; worker++ {
		go func(worker int) {
			defer workers.Done()
			result := d05StressResult{worker: worker}
			for operation := worker; operation < d05StressOperations; operation += d05StressWorkers {
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
	workerErrors := make([]error, 0, d05StressWorkers)
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
	assert.Equal(t, d05StressOperations, completed)
	assert.Equal(t, [4]int{1_000, 1_000, 1_000, 1_000}, scopeCounts)
	assert.EqualValues(t, 2, fixture.runtimePool.Stat().TotalConns())
	assertD05PoolClean(t, fixture.runtimePool)
}

func exerciseBothD05RuntimeConnections(
	t *testing.T,
	ctx context.Context,
	fixture *d05Fixture,
	scopes []d05StressScope,
) {
	t.Helper()
	ready := make(chan int, d05StressWorkers)
	release := make(chan struct{})
	results := make(chan error, d05StressWorkers)
	for worker := 0; worker < d05StressWorkers; worker++ {
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

	readyWorkers := make(map[int]struct{}, d05StressWorkers)
	for len(readyWorkers) < d05StressWorkers {
		select {
		case worker := <-ready:
			readyWorkers[worker] = struct{}{}
		case err := <-results:
			cancelD05ConnectionExercise(release)
			require.NoError(t, err)
		case <-ctx.Done():
			cancelD05ConnectionExercise(release)
			require.NoError(t, ctx.Err())
		}
	}
	assert.EqualValues(t, 2, fixture.runtimePool.Stat().TotalConns())
	assert.EqualValues(t, 2, fixture.runtimePool.Stat().AcquiredConns())
	close(release)
	for range d05StressWorkers {
		require.NoError(t, <-results)
	}
}

func cancelD05ConnectionExercise(release chan struct{}) {
	select {
	case <-release:
	default:
		close(release)
	}
}
