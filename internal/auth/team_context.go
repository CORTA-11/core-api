package auth

import (
	"context"

	"github.com/CORTA-11/core-api/internal/repository"
)

type teamContextKey string

const teamMembershipKey teamContextKey = "team_membership"

type TeamMembership struct {
	TeamID   int64
	PublicID string
	OrgID    int64
	Role     repository.TeamRole
}

func ContextWithTeamMembership(ctx context.Context, membership *TeamMembership) context.Context {
	return context.WithValue(ctx, teamMembershipKey, membership)
}

func TeamMembershipFromContext(ctx context.Context) (*TeamMembership, bool) {
	membership, ok := ctx.Value(teamMembershipKey).(*TeamMembership)
	return membership, ok
}
