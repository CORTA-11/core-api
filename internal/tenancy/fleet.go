package tenancy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ClaimDue atomically leases due or stale organizations to this process.
// SKIP LOCKED lets multiple provisioners divide fleet work without blocking;
// the advisory lock in Reconcile remains the final per-organization serializer.
func (r *Reconciler) ClaimDue(ctx context.Context, limit int) ([]uuid.UUID, error) {
	if limit < 1 || limit > MaxConcurrency {
		return nil, fmt.Errorf("claim limit must be between 1 and %d", MaxConcurrency)
	}
	rows, err := r.pool.Query(ctx, `
        WITH candidates AS (
            SELECT id
            FROM public.orgs
            WHERE deleted_at IS NULL
              AND (
                (lifecycle_state = 'provisioning' AND next_attempt_at <= now())
                OR (lifecycle_state = 'active' AND (tenant_version <> $1 OR tenant_checksum <> $2))
              )
            ORDER BY next_attempt_at, id
            FOR UPDATE SKIP LOCKED
            LIMIT $3
        )
        UPDATE public.orgs AS organization
        SET lifecycle_state = 'provisioning',
            next_attempt_at = now() + $4::interval,
            updated_at = now()
        FROM candidates
        WHERE organization.id = candidates.id
        RETURNING organization.public_id`, r.source.Version, r.source.Checksum, limit, r.config.OperationTimeout.String())
	if err != nil {
		return nil, fmt.Errorf("claim due organizations: %w", err)
	}
	defer rows.Close()
	ids := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read claimed organization: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read claimed organizations: %w", err)
	}
	return ids, nil
}

// ReconcileMany processes IDs through a bounded worker pool and closes the
// result channel after every started worker exits.
func (r *Reconciler) ReconcileMany(ctx context.Context, ids []uuid.UUID, concurrency int) <-chan Result {
	if concurrency < 1 {
		concurrency = 1
	} else if concurrency > MaxConcurrency {
		concurrency = MaxConcurrency
	}
	results := make(chan Result, concurrency)
	work := make(chan uuid.UUID, concurrency)
	var workers sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for id := range work {
				select {
				case results <- r.Reconcile(ctx, id):
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(work)
		for _, id := range ids {
			select {
			case work <- id:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()
	return results
}

// Run performs an immediate fleet scan and then polls until ctx is canceled.
// Cancellation is a graceful stop rather than a provisioner failure.
func (r *Reconciler) Run(ctx context.Context, emit func(Result)) error {
	if err := r.scan(ctx, emit); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.scan(ctx, emit); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

// scan handles the scan operation.
func (r *Reconciler) scan(ctx context.Context, emit func(Result)) error {
	// Drain full batches before sleeping so backlog throughput is bounded by
	// worker concurrency rather than the polling interval.
	for {
		ids, err := r.ClaimDue(ctx, r.config.Concurrency)
		if err != nil {
			return err
		}
		for result := range r.ReconcileMany(ctx, ids, r.config.Concurrency) {
			emit(result)
		}
		if len(ids) < r.config.Concurrency {
			return nil
		}
	}
}

// Status is an operator-facing snapshot of one organization's reconciliation
// state. Current is true only for an available, exact source match.
type Status struct {
	OrganizationID uuid.UUID      `json:"organization_id"`
	State          LifecycleState `json:"state"`
	Version        int64          `json:"version"`
	Checksum       string         `json:"checksum,omitempty"`
	Attempts       int            `json:"attempts"`
	NextAttemptAt  time.Time      `json:"next_attempt_at"`
	ErrorCode      string         `json:"error_code,omitempty"`
	ErrorDetail    string         `json:"error_detail,omitempty"`
	Current        bool           `json:"current"`
}

// Status returns one organization when id is non-nil, otherwise the whole
// non-deleted fleet in stable registry order.
func (r *Reconciler) Status(ctx context.Context, id *uuid.UUID) ([]Status, error) {
	query := `
        SELECT public_id, lifecycle_state, tenant_version, tenant_checksum,
               reconcile_attempts, next_attempt_at,
               COALESCE(last_error_code, ''), COALESCE(last_error_detail, '')
        FROM public.orgs WHERE deleted_at IS NULL`
	args := make([]any, 0, 1)
	if id != nil {
		query += ` AND public_id = $1`
		args = append(args, *id)
	}
	query += ` ORDER BY id`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read tenant fleet status: %w", err)
	}
	defer rows.Close()
	statuses := make([]Status, 0)
	for rows.Next() {
		var status Status
		if err := rows.Scan(&status.OrganizationID, &status.State, &status.Version, &status.Checksum,
			&status.Attempts, &status.NextAttemptAt, &status.ErrorCode, &status.ErrorDetail); err != nil {
			return nil, fmt.Errorf("read tenant status: %w", err)
		}
		status.Checksum = trimChecksum(status.Checksum)
		status.Current = status.State == StateActive && status.Version == r.source.Version && status.Checksum == r.source.Checksum
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tenant statuses: %w", err)
	}
	return statuses, nil
}

// AllOrganizationIDs lists public identifiers for explicit fleet reconciliation.
func (r *Reconciler) AllOrganizationIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT public_id FROM public.orgs WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read organization identity: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Retry moves failed organizations back to provisioning and clears their
// automatic-attempt budget. It does not bypass ledger or catalog validation.
func (r *Reconciler) Retry(ctx context.Context, id *uuid.UUID) (int64, error) {
	query := `
        UPDATE public.orgs
        SET lifecycle_state = 'provisioning', reconcile_attempts = 0,
            next_attempt_at = now(), last_error_code = NULL, last_error_detail = NULL,
            updated_at = now()
        WHERE deleted_at IS NULL AND lifecycle_state = 'failed'`
	args := make([]any, 0, 1)
	if id != nil {
		query += ` AND public_id = $1`
		args = append(args, *id)
	}
	command, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("retry tenant reconciliation: %w", err)
	}
	return command.RowsAffected(), nil
}

// AvailabilityChecker gates tenant traffic on registry state and the exact
// migration set embedded in the serving API binary.
type AvailabilityChecker struct {
	pool   *pgxpool.Pool
	source MigrationSet
}

// NewAvailabilityChecker constructs an API-side tenant availability gate.
func NewAvailabilityChecker(pool *pgxpool.Pool, source MigrationSet) *AvailabilityChecker {
	return &AvailabilityChecker{pool: pool, source: source}
}

// Availability distinguishes an unknown organization from a known tenant that
// must remain unavailable while provisioning, failed, deleting, or stale.
type Availability int

const (
	// AvailabilityUnknown means no non-deleted registry row exists.
	AvailabilityUnknown Availability = iota
	// AvailabilityUnavailable means the registry row is not an exact current match.
	AvailabilityUnavailable
	// AvailabilityReady means the tenant is active at the exact embedded version
	// and checksum.
	AvailabilityReady
)

// Check resolves availability without exposing the internal schema name.
func (c *AvailabilityChecker) Check(ctx context.Context, id uuid.UUID) (Availability, error) {
	var state LifecycleState
	var version int64
	var checksum string
	err := c.pool.QueryRow(ctx, `
        SELECT lifecycle_state, tenant_version, tenant_checksum
        FROM public.orgs WHERE public_id = $1 AND deleted_at IS NULL`, id).Scan(&state, &version, &checksum)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AvailabilityUnknown, nil
		}
		return AvailabilityUnknown, fmt.Errorf("resolve organization availability: %w", err)
	}
	if state != StateActive || version != c.source.Version || trimChecksum(checksum) != c.source.Checksum {
		return AvailabilityUnavailable, nil
	}
	return AvailabilityReady, nil
}
