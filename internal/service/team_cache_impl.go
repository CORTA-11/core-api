package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
)

type CachedTeamService struct {
	baseTeamService TeamService
	cache           CacheService
}

func NewCachedTeamService(teamService TeamService, cache CacheService) TeamService {
	return &CachedTeamService{
		baseTeamService: teamService,
		cache:           cache,
	}
}

func (c *CachedTeamService) GetTeams(ctx context.Context, schema string) ([]Team, error) {
	return c.baseTeamService.GetTeams(ctx, schema)
}

func (c *CachedTeamService) CreateTeam(ctx context.Context, name, schema string) (*Team, error) {
	return c.baseTeamService.CreateTeam(ctx, name, schema)
}

func (c *CachedTeamService) GetTeamID(ctx context.Context, slug, schema string) (int, error) {
	var teamID int

	// Check redis cache
	key := fmt.Sprintf("team:%s:schema:%s", slug, schema)
	cachedTeamID, err := c.cache.Get(ctx, key)

	switch err {
	case nil:
		// Cache hit
		slog.InfoContext(ctx, "teamID found in Redis cache", "key", key)

		teamID, err = strconv.Atoi(cachedTeamID)
		if err != nil {
			slog.ErrorContext(ctx, "invalid cached team ID",
				"key", key,
				"value", cachedTeamID,
				"error", err.Error(),
			)
			return 0, err
		}

	case ErrCacheMiss:
		slog.InfoContext(ctx, "teamID cache miss", "key", key)

		teamID, err = c.baseTeamService.GetTeamID(ctx, slug, schema)
		if err != nil {
			slog.ErrorContext(ctx, "failed to get team ID", "error", err)
			return 0, err
		}

		// Populate redis
		err := c.cache.Set(ctx, key, teamID, 0)
		if err != nil {
			slog.ErrorContext(ctx, "failed to set redis value", "error", err.Error())
			return 0, err
		}

	default:
		slog.ErrorContext(ctx, "redis GET failed", "key", key, "error", err.Error())
		return 0, fmt.Errorf("failed to set redis value")
	}

	return teamID, nil
}
