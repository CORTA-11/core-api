package service

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TaskDates distinguishes omitted dates from explicit nulls on task updates.
type TaskDates struct {
	StartDate    *time.Time
	DueDate      *time.Time
	SetStartDate bool
	SetDueDate   bool
}

func taskDateValue(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func taskDateView(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	// Keep date serialization independent of the API process timezone.
	utc := value.Time.UTC()
	return &utc
}
