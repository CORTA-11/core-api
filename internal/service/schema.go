package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func setSchema(ctx context.Context, tx pgx.Tx, schema string) error {
	setSearchPathQuery := fmt.Sprintf("SET LOCAL search_path TO %s", schema)
	_, err := tx.Exec(ctx, setSearchPathQuery)
	return err
}

func intToPgtypeInt8(val int) pgtype.Int8 {
	return pgtype.Int8{
		Int64: int64(val),
		Valid: true,
	}
}

func SchemaName(orgID uuid.UUID) string {
	return "org_" + strings.ReplaceAll(orgID.String(), "-", "")
}
