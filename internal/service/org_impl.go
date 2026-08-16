package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type orgService struct {
	pool    *pgxpool.Pool
	queries *repository.Queries
}

func NewOrgService(pool *pgxpool.Pool, queries *repository.Queries) OrgService {
	return &orgService{
		pool:    pool,
		queries: queries,
	}
}

func (o *orgService) GetOrgs(ctx context.Context) ([]Organization, error) {
	// start transaction
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := o.queries.WithTx(tx)

	// insert into public.orgs
	setPathPublicQuery := "SET LOCAL search_path TO public"
	_, err = tx.Exec(ctx, setPathPublicQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to set search path: %w", err)
	}

	dbOrgs, err := qtx.GetOrgs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch orgs: %w", err)
	}

	domainOrgs := make([]Organization, 0, len(dbOrgs))

	for _, dbOrg := range dbOrgs {
		domainOrgs = append(domainOrgs, mapDBOrgToDomain(dbOrg))
	}

	// commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	return domainOrgs, nil
}

func (o *orgService) CreateOrg(ctx context.Context, name string) (*Organization, error) {
	// create schema name org_<uuid without dashed>
	publicID := uuid.New()
	schemaName := "org_" + strings.ReplaceAll(publicID.String(), "-", "")

	// start transaction
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := o.queries.WithTx(tx)

	// insert into public.orgs
	setPathPublicQuery := "SET LOCAL search_path TO public"
	_, err = tx.Exec(ctx, setPathPublicQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to set search path: %w", err)
	}
	org, err := qtx.CreateOrg(ctx, repository.CreateOrgParams{
		Name:       name,
		PublicID:   publicID,
		SchemaName: schemaName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create org: %w", err)
	}

	// create schema
	schemaQuery := fmt.Sprintf("CREATE SCHEMA %s", schemaName)
	_, err = tx.Exec(ctx, schemaQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	// do tenant migrations
	err = MigrateSchema(schemaName)
	if err != nil {
		return nil, err
	}

	ret := mapDBOrgToDomain(org)
	return &ret, nil
}

func MigrateSchema(schemaName string) error {
	dbURL, err := url.Parse(os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("failed to parse database URL", "error", err)
		return fmt.Errorf("parse database URL: %w", err)
	}
	q := dbURL.Query()
	q.Set("search_path", schemaName)
	dbURL.RawQuery = q.Encode()

	m, err := migrate.New(
		"file://db/migrations/tenant",
		dbURL.String(),
	)
	if err != nil {
		slog.Error("failed to create tenant migrator", "error", err)
		return fmt.Errorf("create tenant migrator: %w", err)
	}
	defer func() {
		sourceErr, databaseErr := m.Close()
		if sourceErr != nil {
			slog.Error("failed to close tenant migration source", "error", sourceErr)
		}
		if databaseErr != nil {
			slog.Error("failed to close tenant migration database", "error", databaseErr)
		}
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		slog.Error("failed to run tenant migration", "error", err)
		return fmt.Errorf("run tenant migrations: %w", err)
	}
	return nil
}

func (o *orgService) UpdateOrg(ctx context.Context, publicID uuid.UUID, name string) (*Organization, error) {
	org, err := o.queries.UpdateOrg(ctx, repository.UpdateOrgParams{
		PublicID: publicID,
		Name:     name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update org: %w", err)
	}
	ret := mapDBOrgToDomain(org)
	return &ret, nil
}

func (o *orgService) SoftDeleteOrg(ctx context.Context, publicID uuid.UUID) (*Organization, error) {
	org, err := o.queries.SoftDeleteOrg(ctx, publicID)
	if err != nil {
		return nil, fmt.Errorf("failed to soft delete org: %w", err)
	}
	ret := mapDBOrgToDomain(org)
	return &ret, nil
}

func (o *orgService) RestoreOrg(ctx context.Context, publicID uuid.UUID) (*Organization, error) {
	org, err := o.queries.RestoreOrg(ctx, publicID)
	if err != nil {
		return nil, fmt.Errorf("failed to restore org: %w", err)
	}
	ret := mapDBOrgToDomain(org)
	return &ret, nil
}
