package identity

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashConfigDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	config, err := (HashConfig{}).withDefaults()
	require.NoError(t, err)
	assert.Equal(t, max(1, min(4, runtime.GOMAXPROCS(0))), config.Concurrency)
	assert.Equal(t, 2*time.Second, config.AdmissionTimeout)

	for _, invalid := range []HashConfig{
		{Concurrency: -1},
		{Concurrency: 17},
		{AdmissionTimeout: -time.Second},
		{AdmissionTimeout: 30*time.Second + time.Nanosecond},
	} {
		_, err := invalid.withDefaults()
		assert.ErrorIs(t, err, ErrInvalidHashConfig)
	}
}

func TestPasswordHasherHashAndVerify(t *testing.T) {
	t.Parallel()

	hasher, err := NewPasswordHasher(HashConfig{Concurrency: 1, AdmissionTimeout: time.Second})
	require.NoError(t, err)
	encoded, err := hasher.Hash(context.Background(), "correct horse battery staple")
	require.NoError(t, err)
	assert.NotContains(t, encoded, "correct horse")

	verification, err := hasher.Verify(context.Background(), "correct horse battery staple", encoded)
	require.NoError(t, err)
	assert.True(t, verification.Match)
	assert.False(t, verification.NeedsRehash)

	verification, err = hasher.Verify(context.Background(), "incorrect credential value", encoded)
	require.NoError(t, err)
	assert.False(t, verification.Match)
	assert.False(t, verification.NeedsRehash)
}

func TestPasswordHasherBoundsAdmissionAndDoesNotLeakPermits(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var workCalls atomic.Int32
	derive := func(password, salt []byte, parameters argon2Parameters) []byte {
		workCalls.Add(1)
		started <- struct{}{}
		<-release
		return bytes.Repeat([]byte{1}, int(parameters.outputBytes))
	}
	hasher, err := newPasswordHasher(
		HashConfig{Concurrency: 1, AdmissionTimeout: 20 * time.Millisecond},
		bytes.NewReader(make([]byte, 48)),
		derive,
	)
	require.NoError(t, err)

	firstResult := make(chan error, 1)
	go func() {
		_, hashErr := hasher.Hash(context.Background(), "first password value")
		firstResult <- hashErr
	}()
	<-started

	_, err = hasher.Hash(context.Background(), "second password value")
	assert.ErrorIs(t, err, ErrHashCapacity)
	assert.Equal(t, int32(1), workCalls.Load())

	close(release)
	require.NoError(t, <-firstResult)

	hasher.derive = func(password, salt []byte, parameters argon2Parameters) []byte {
		return bytes.Repeat([]byte{2}, int(parameters.outputBytes))
	}
	_, err = hasher.Hash(context.Background(), "third password value")
	require.NoError(t, err)
}

func TestPasswordHasherAdmissionObservesCancellationButStartedWorkCompletes(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	derive := func(password, salt []byte, parameters argon2Parameters) []byte {
		started <- struct{}{}
		<-release
		return bytes.Repeat([]byte{1}, int(parameters.outputBytes))
	}
	hasher, err := newPasswordHasher(
		HashConfig{Concurrency: 1, AdmissionTimeout: time.Second},
		bytes.NewReader(make([]byte, 32)),
		derive,
	)
	require.NoError(t, err)

	firstContext, cancelFirst := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, hashErr := hasher.Hash(firstContext, "first password value")
		firstResult <- hashErr
	}()
	<-started
	cancelFirst()

	secondContext, cancelSecond := context.WithCancel(context.Background())
	cancelSecond()
	_, err = hasher.Hash(secondContext, "second password value")
	assert.True(t, errors.Is(err, context.Canceled))

	select {
	case err := <-firstResult:
		t.Fatalf("started work returned before completion: %v", err)
	default:
	}
	close(release)
	require.NoError(t, <-firstResult)
}
