//go:build integration

package integration

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/ratelimit"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisGCRASharesAtomicHMACOnlyFiniteState(t *testing.T) {
	clientOne := testsupport.OpenRedis(t)
	clientTwo := testsupport.OpenRedis(t)
	testsupport.FlushRedis(t, clientOne)
	secret := []byte("integration-rate-limit-secret-value")
	first, err := ratelimit.NewRedis(clientOne, secret, time.Second)
	require.NoError(t, err)
	second, err := ratelimit.NewRedis(clientTwo, secret, time.Second)
	require.NoError(t, err)
	policy := ratelimit.Policy{Bucket: "login-ip", Limit: 2, Period: 2 * time.Second, Burst: 2}

	decision, err := first.Check(context.Background(), policy, "user@example.com", true)
	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	decision, err = second.Check(context.Background(), policy, "user@example.com", true)
	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	decision, err = first.Check(context.Background(), policy, "user@example.com", true)
	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Positive(t, decision.RetryAfter)

	keys, err := clientOne.Keys(context.Background(), "synodus:ratelimit:v1:*").Result()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Regexp(t, regexp.MustCompile(`^synodus:ratelimit:v1:login-ip:[0-9a-f]{64}$`), keys[0])
	assert.NotContains(t, keys[0], "user@example.com")
	ttl, err := clientOne.PTTL(context.Background(), keys[0]).Result()
	require.NoError(t, err)
	assert.Positive(t, ttl)
	require.NoError(t, first.Clear(context.Background(), policy, "user@example.com"))
	assert.Equal(t, int64(0), clientOne.DBSize(context.Background()).Val())
}

func TestRedisGCRAConcurrentAdmissionIsExact(t *testing.T) {
	client := testsupport.OpenRedis(t)
	testsupport.FlushRedis(t, client)
	limiter, err := ratelimit.NewRedis(client, []byte("integration-rate-limit-secret-value"), time.Second)
	require.NoError(t, err)
	policy := ratelimit.Policy{Bucket: "administrative", Limit: 10, Period: 10 * time.Second, Burst: 10}
	var allowed atomic.Int64
	var failures atomic.Int64
	var group sync.WaitGroup
	for range 50 {
		group.Add(1)
		go func() {
			defer group.Done()
			decision, checkErr := limiter.Check(context.Background(), policy, "192.0.2.4", true)
			if checkErr != nil {
				failures.Add(1)
				return
			}
			if decision.Allowed {
				allowed.Add(1)
			}
		}()
	}
	group.Wait()
	assert.Zero(t, failures.Load())
	assert.Equal(t, int64(10), allowed.Load())
	keys := client.Keys(context.Background(), "*").Val()
	require.Len(t, keys, 1)
	assert.NotContains(t, strings.Join(keys, ""), "192.0.2.4")
}
