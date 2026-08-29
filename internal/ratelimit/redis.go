package ratelimit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisKeyPrefix = "synodus:ratelimit:v1:"

const gcraScript = `
local current = redis.call('TIME')
local now = (tonumber(current[1]) * 1000000) + tonumber(current[2])
local interval = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local consume = tonumber(ARGV[3])
local stored = redis.call('GET', KEYS[1])
local tat = tonumber(stored) or now
local allow_at = tat - ((burst - 1) * interval)
if now < allow_at then
  return {0, math.ceil((allow_at - now) / 1000000), redis.call('PTTL', KEYS[1])}
end
if consume == 1 then
  local new_tat = math.max(tat, now) + interval
  local ttl = math.ceil((new_tat - now + ((burst - 1) * interval)) / 1000)
  redis.call('SET', KEYS[1], new_tat, 'PX', ttl)
  return {1, 0, ttl}
end
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then ttl = 0 end
return {1, 0, ttl}
`

type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
	TTL        time.Duration
}

type Limiter interface {
	Check(context.Context, Policy, string, bool) (Decision, error)
	Clear(context.Context, Policy, string) error
}

type redisCommands interface {
	Eval(context.Context, string, []string, ...any) *redis.Cmd
	Del(context.Context, ...string) *redis.IntCmd
}

type Redis struct {
	client  redisCommands
	secret  []byte
	timeout time.Duration
}

// NewRedis creates a Redis-backed rate limiter.
func NewRedis(client redisCommands, secret []byte, timeout time.Duration) (*Redis, error) {
	if client == nil || len(secret) < 16 || timeout <= 0 || timeout > 5*time.Second {
		return nil, errors.New("invalid Redis rate limiter configuration")
	}
	return &Redis{client: client, secret: append([]byte(nil), secret...), timeout: timeout}, nil
}

// Check evaluates an identity against a rate-limit policy and optionally consumes an allowance.
func (limiter *Redis) Check(parent context.Context, policy Policy, identity string, consume bool) (Decision, error) {
	if err := policy.Validate(); err != nil || identity == "" || len(identity) > 512 {
		return Decision{}, errors.New("invalid rate-limit request")
	}
	interval := policy.Period / time.Duration(policy.Limit)
	ctx, cancel := context.WithTimeout(parent, limiter.timeout)
	defer cancel()
	result, err := limiter.client.Eval(ctx, gcraScript, []string{limiter.key(policy.Bucket, identity)},
		interval.Microseconds(), policy.Burst, boolInt(consume)).Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("rate-limit dependency: %w", err)
	}
	if len(result) != 3 {
		return Decision{}, errors.New("invalid rate-limit dependency response")
	}
	allowed, ok := boundedInteger(result[0], 0, 1)
	if !ok {
		return Decision{}, errors.New("invalid rate-limit dependency response")
	}
	retrySeconds, ok := boundedInteger(result[1], 0, 86_400)
	if !ok {
		return Decision{}, errors.New("invalid rate-limit dependency response")
	}
	ttlMillis, ok := boundedInteger(result[2], 0, int64((2*time.Hour)/time.Millisecond)+1)
	if !ok {
		return Decision{}, errors.New("invalid rate-limit dependency response")
	}
	return Decision{Allowed: allowed == 1, RetryAfter: time.Duration(retrySeconds) * time.Second,
		TTL: time.Duration(max(ttlMillis, 0)) * time.Millisecond}, nil
}

// Clear removes the stored rate-limit state for an identity and policy.
func (limiter *Redis) Clear(parent context.Context, policy Policy, identity string) error {
	if err := policy.Validate(); err != nil || identity == "" || len(identity) > 512 {
		return errors.New("invalid rate-limit request")
	}
	ctx, cancel := context.WithTimeout(parent, limiter.timeout)
	defer cancel()
	if err := limiter.client.Del(ctx, limiter.key(policy.Bucket, identity)).Err(); err != nil {
		return fmt.Errorf("rate-limit dependency: %w", err)
	}
	return nil
}

// key creates the Redis key for a rate-limit bucket and identity.
func (limiter *Redis) key(bucket, identity string) string {
	mac := hmac.New(sha256.New, limiter.secret)
	_, _ = mac.Write([]byte(bucket))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(identity))
	return redisKeyPrefix + bucket + ":" + hex.EncodeToString(mac.Sum(nil))
}

// boolInt converts a boolean value to its integer representation.
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// boundedInteger converts a supported value to an integer within the given bounds.
func boundedInteger(value any, minimum, maximum int64) (int64, bool) {
	var result int64
	switch typed := value.(type) {
	case int64:
		result = typed
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		if err != nil {
			return 0, false
		}
		result = parsed
	default:
		return 0, false
	}
	return result, result >= minimum && result <= maximum
}
