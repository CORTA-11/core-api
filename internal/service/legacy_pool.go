package service

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// pgxPool remains the narrow transaction seam used by the retained M02
// organization service tests. The authenticated v1 applications use concrete,
// explicitly authorized dependencies.
type pgxPool interface {
	Begin(context.Context) (pgx.Tx, error)
}
