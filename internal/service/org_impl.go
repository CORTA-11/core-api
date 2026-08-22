package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
)

type orgService struct {
	pool    pgxPool
	queries *publicdb.Queries
}

func NewOrgService(pool pgxPool, queries *publicdb.Queries, databaseURL string) OrgService {
	_ = databaseURL // Kept temporarily for source compatibility with D01 callers.
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

	dbOrgs, err := qtx.GetOrgs(ctx, listResultLimit)
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
	// Derive the schema identifier from a server-generated UUID so organization
	// names and other client input never influence an SQL identifier.
	publicID := uuid.New()
	schemaName := "org_" + strings.ReplaceAll(publicID.String(), "-", "")

	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := o.queries.WithTx(tx)

	// Keep registry access explicitly transaction-local to public; pooled
	// connections may previously have served tenant-scoped work.
	setPathPublicQuery := "SET LOCAL search_path TO public"
	_, err = tx.Exec(ctx, setPathPublicQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to set search path: %w", err)
	}
	org, err := qtx.CreateOrg(ctx, publicdb.CreateOrgParams{
		Name:       name,
		PublicID:   publicID,
		SchemaName: schemaName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create org: %w", err)
	}

	// Committing the registry row is the durable handoff to the provisioner. No
	// tenant DDL runs on the request path.
	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	ret := mapDBOrgToDomain(org)
	return &ret, nil
}

func (o *orgService) UpdateOrg(ctx context.Context, publicID uuid.UUID, name string) (*Organization, error) {
	org, err := o.queries.UpdateOrg(ctx, publicdb.UpdateOrgParams{
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
	// The query enters deleting atomically with deleted_at so an in-flight
	// reconciler cannot reactivate the organization.
	org, err := o.queries.SoftDeleteOrg(ctx, publicID)
	if err != nil {
		return nil, fmt.Errorf("failed to soft delete org: %w", err)
	}
	ret := mapDBOrgToDomain(org)
	return &ret, nil
}

func (o *orgService) RestoreOrg(ctx context.Context, publicID uuid.UUID) (*Organization, error) {
	// Restore records fresh provisioning intent; the schema must pass normal
	// ledger and catalog validation before tenant traffic resumes.
	org, err := o.queries.RestoreOrg(ctx, publicID)
	if err != nil {
		return nil, fmt.Errorf("failed to restore org: %w", err)
	}
	ret := mapDBOrgToDomain(org)
	return &ret, nil
}
