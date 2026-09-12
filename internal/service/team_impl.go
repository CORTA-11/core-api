package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/tenancy"
)

type teamService struct {
	executor organizationExecutor
}

func NewTeamService(executor organizationExecutor) TeamService {
	return &teamService{executor: executor}
}

func (t *teamService) GetTeams(ctx context.Context, organization tenancy.OrganizationContext) ([]Team, error) {
	var teams []tenantdb.Team
	err := t.executor.WithinOrganization(ctx, organization, func(queries *tenantdb.Queries) error {
		var queryErr error
		teams, queryErr = queries.GetTeams(ctx, listResultLimit)
		return queryErr
	})
	if err != nil {
		return nil, fmt.Errorf("fetch teams: %w", err)
	}

	domainTeams := make([]Team, 0, len(teams))

	for _, team := range teams {
		domainTeams = append(domainTeams, mapDBTeamToDomain(team))
	}

	return domainTeams, nil
}

func (t *teamService) CreateTeam(ctx context.Context, organization tenancy.OrganizationContext, name, leaderEmail string) (*Team, error) {
	slug := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	var team tenantdb.Team
	err := t.executor.WithinOrganization(ctx, organization, func(queries *tenantdb.Queries) error {
		var queryErr error
		team, queryErr = queries.CreateTeamWithCreator(ctx, tenantdb.CreateTeamWithCreatorParams{Name: name, Slug: slug, LeaderEmail: leaderEmail})
		return queryErr
	})
	if err != nil {
		return nil, fmt.Errorf("create team: %w", err)
	}

	ret := mapDBTeamToDomain(team)

	return &ret, nil
}
