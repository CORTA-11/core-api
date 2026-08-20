package tenancy

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type LifecycleState string

const (
	StateProvisioning LifecycleState = "provisioning"
	StateActive       LifecycleState = "active"
	StateFailed       LifecycleState = "failed"
	StateDeleting     LifecycleState = "deleting"
)

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

func (e *permanentError) Error() string { return e.detail }

func permanent(code, detail string) error { return &permanentError{code: code, detail: detail} }

func errorFields(err error) (code, detail string, isPermanent bool) {
	var target *permanentError
	if errors.As(err, &target) {
		return target.code, target.detail, true
	}
	return "reconciliation_failed", "tenant reconciliation could not be completed", false
}
