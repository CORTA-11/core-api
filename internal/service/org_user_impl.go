package service

import (
	"context"
	"fmt"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
)

type orgUserService struct {
	pool    pgxPool
	queries *publicdb.Queries
}

// NewOrgUserService creates a new instance of OrgUserService.
func NewOrgUserService(pool pgxPool, queries *publicdb.Queries) OrgUserService {
	return &orgUserService{
		pool:    pool,
		queries: queries,
	}
}

func (s *orgUserService) AddUserToOrg(ctx context.Context, orgPublicID uuid.UUID, userPublicID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)

	_, err = tx.Exec(ctx, "SET LOCAL search_path TO public")
	if err != nil {
		return err
	}

	orgID, err := qtx.GetOrgID(ctx, orgPublicID)
	if err != nil {
		return fmt.Errorf("failed to find organization: %w", err)
	}

	repoUser, err := qtx.GetUserByID(ctx, userPublicID)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}

	_, err = qtx.AddUserToOrg(ctx, publicdb.AddUserToOrgParams{
		OrgID:  orgID,
		UserID: repoUser.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to add user to org: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *orgUserService) RemoveUserFromOrg(ctx context.Context, orgPublicID uuid.UUID, userPublicID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)

	_, err = tx.Exec(ctx, "SET LOCAL search_path TO public")
	if err != nil {
		return err
	}

	orgID, err := qtx.GetOrgID(ctx, orgPublicID)
	if err != nil {
		return fmt.Errorf("failed to find organization: %w", err)
	}

	repoUser, err := qtx.GetUserByID(ctx, userPublicID)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}

	_, err = qtx.RemoveUserFromOrg(ctx, publicdb.RemoveUserFromOrgParams{
		OrgID:  orgID,
		UserID: repoUser.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to remove user from org: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *orgUserService) GetOrgsForUser(ctx context.Context, userPublicID uuid.UUID) ([]Organization, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)

	_, err = tx.Exec(ctx, "SET LOCAL search_path TO public")
	if err != nil {
		return nil, err
	}

	repoUser, err := qtx.GetUserByID(ctx, userPublicID)
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	dbOrgs, err := qtx.GetOrgsForUser(ctx, publicdb.GetOrgsForUserParams{
		UserID: repoUser.ID,
		Limit:  listResultLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get orgs for user: %w", err)
	}

	domainOrgs := make([]Organization, len(dbOrgs))
	for i, dbOrg := range dbOrgs {
		domainOrgs[i] = mapDBOrgToDomain(dbOrg)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return domainOrgs, nil
}

func (s *orgUserService) GetUsersInOrg(ctx context.Context, orgPublicID uuid.UUID) ([]User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)

	_, err = tx.Exec(ctx, "SET LOCAL search_path TO public")
	if err != nil {
		return nil, err
	}

	orgID, err := qtx.GetOrgID(ctx, orgPublicID)
	if err != nil {
		return nil, fmt.Errorf("failed to find organization: %w", err)
	}

	dbUsers, err := qtx.GetUsersInOrg(ctx, publicdb.GetUsersInOrgParams{
		OrgID: orgID,
		Limit: listResultLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get users in org: %w", err)
	}

	domainUsers := make([]User, len(dbUsers))
	for i, dbUser := range dbUsers {
		domainUsers[i] = mapDBUserToDomain(dbUser)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return domainUsers, nil
}

func (s *orgUserService) GetNumberOfUsersInOrg(ctx context.Context, orgPublicID uuid.UUID) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)

	_, err = tx.Exec(ctx, "SET LOCAL search_path TO public")
	if err != nil {
		return 0, err
	}

	orgID, err := qtx.GetOrgID(ctx, orgPublicID)
	if err != nil {
		return 0, fmt.Errorf("failed to find organization: %w", err)
	}

	count, err := qtx.GetNumberOfUsersInOrg(ctx, orgID)
	if err != nil {
		return 0, fmt.Errorf("failed to get number of users in org: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	return count, nil
}
