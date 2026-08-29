package tenancy

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// LifecycleState describes whether an organization's tenant schema may serve
// requests or still requires reconciliation or deletion.
type LifecycleState string

const (
	// StateProvisioning is retryable and unavailable to tenant requests.
	StateProvisioning LifecycleState = "provisioning"
	// StateActive is usable only when its version and checksum are also current.
	StateActive LifecycleState = "active"
	// StateFailed requires operator repair and an explicit retry.
	StateFailed LifecycleState = "failed"
	// StateDeleting prevents reconciliation from recreating or activating a tenant.
	StateDeleting LifecycleState = "deleting"
)

// Organization is the internal registry snapshot used during reconciliation.
// ID is never exposed externally; PublicID is the command and API identity.
type Organization struct {
	ID                int64
	PublicID          uuid.UUID
	SchemaName        string
	LifecycleState    LifecycleState
	TenantVersion     int64
	TenantChecksum    string
	ReconcileAttempts int
	NextAttemptAt     time.Time
}

// Result is the bounded, JSON-safe outcome emitted for one organization.
// ErrorDetail is sanitized before it reaches this type.
type Result struct {
	OrganizationID uuid.UUID      `json:"organization_id"`
	State          LifecycleState `json:"state"`
	Version        int64          `json:"version"`
	Checksum       string         `json:"checksum,omitempty"`
	Attempts       int            `json:"attempts"`
	ErrorCode      string         `json:"error_code,omitempty"`
	ErrorDetail    string         `json:"error_detail,omitempty"`
}

type permanentError struct {
	code   string
	detail string
}

// Error returns the error message.
func (e *permanentError) Error() string { return e.detail }

// permanent handles the permanent operation.
func permanent(code, detail string) error { return &permanentError{code: code, detail: detail} }

// errorFields errors fields.
func errorFields(err error) (code, detail string, isPermanent bool) {
	var target *permanentError
	if errors.As(err, &target) {
		return target.code, target.detail, true
	}
	// Driver errors and migration SQL may contain credentials or internal
	// details, so transient failures intentionally share one public message.
	return "reconciliation_failed", "tenant reconciliation could not be completed", false
}
