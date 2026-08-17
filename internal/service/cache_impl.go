package service

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type cacheService struct {
	client *redis.Client
}

func NewCacheService(client *redis.Client) CacheService {
	return &cacheService{
		client: client,
	}
}

func (c *cacheService) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCacheMiss
	}
	return val, err
}

func (c *cacheService) Set(ctx context.Context, key string, val any, expire time.Duration) error {
	return c.client.Set(ctx, key, val, 0).Err()
}
