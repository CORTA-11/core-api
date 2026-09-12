package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/sync/semaphore"
)

const maximumHashConcurrency = 16

var (
	ErrInvalidHashConfig = errors.New("invalid password hash configuration")
	ErrHashCapacity      = errors.New("password hashing capacity unavailable")
	ErrHashDependency    = errors.New("password hashing dependency unavailable")
)

type HashConfig struct {
	Concurrency      int
	AdmissionTimeout time.Duration
}

func (config HashConfig) withDefaults() (HashConfig, error) {
	if config.Concurrency == 0 {
		config.Concurrency = max(1, min(4, runtime.GOMAXPROCS(0)))
	}
	if config.AdmissionTimeout == 0 {
		config.AdmissionTimeout = 2 * time.Second
	}
	if config.Concurrency < 1 || config.Concurrency > maximumHashConcurrency ||
		config.AdmissionTimeout <= 0 || config.AdmissionTimeout > 30*time.Second {
		return HashConfig{}, ErrInvalidHashConfig
	}
	return config, nil
}

type PasswordVerification struct {
	Match       bool
	NeedsRehash bool
}

type PasswordHasher interface {
	Hash(context.Context, string) (string, error)
	Verify(context.Context, string, string) (PasswordVerification, error)
}

type argon2Parameters struct {
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
	saltBytes   uint32
	outputBytes uint32
}

var targetArgon2Parameters = argon2Parameters{
	memoryKiB:   64 * 1024,
	iterations:  3,
	parallelism: 4,
	saltBytes:   16,
	outputBytes: 32,
}

type argon2Derive func(password, salt []byte, parameters argon2Parameters) []byte

type argon2PasswordHasher struct {
	config    HashConfig
	admission *semaphore.Weighted
	random    io.Reader
	derive    argon2Derive
}

var processAdmissions sync.Map

func NewPasswordHasher(config HashConfig) (*argon2PasswordHasher, error) {
	validated, err := config.withDefaults()
	if err != nil {
		return nil, err
	}
	admission, _ := processAdmissions.LoadOrStore(
		validated.Concurrency,
		semaphore.NewWeighted(int64(validated.Concurrency)),
	)
	return &argon2PasswordHasher{
		config:    validated,
		admission: admission.(*semaphore.Weighted),
		random:    rand.Reader,
		derive:    deriveArgon2ID,
	}, nil
}

func newPasswordHasher(config HashConfig, random io.Reader, derive argon2Derive) (*argon2PasswordHasher, error) {
	validated, err := config.withDefaults()
	if err != nil {
		return nil, err
	}
	if random == nil || derive == nil {
		return nil, ErrInvalidHashConfig
	}
	return &argon2PasswordHasher{
		config:    validated,
		admission: semaphore.NewWeighted(int64(validated.Concurrency)),
		random:    random,
		derive:    derive,
	}, nil
}

func (hasher *argon2PasswordHasher) Hash(ctx context.Context, password string) (string, error) {
	salt := make([]byte, targetArgon2Parameters.saltBytes)
	if _, err := io.ReadFull(hasher.random, salt); err != nil {
		return "", ErrHashDependency
	}

	digest, err := hasher.run(ctx, []byte(password), salt, targetArgon2Parameters)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		targetArgon2Parameters.memoryKiB,
		targetArgon2Parameters.iterations,
		targetArgon2Parameters.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(digest),
	), nil
}

func (hasher *argon2PasswordHasher) Verify(ctx context.Context, password, encoded string) (PasswordVerification, error) {
	parsed, err := parseArgon2IDHash(encoded)
	if err != nil {
		return PasswordVerification{}, err
	}
	parameters := argon2Parameters{
		memoryKiB:   parsed.memoryKiB,
		iterations:  parsed.iterations,
		parallelism: parsed.parallelism,
		saltBytes:   uint32(len(parsed.salt)),   // #nosec G115 -- parser caps salt at 32 bytes.
		outputBytes: uint32(len(parsed.digest)), // #nosec G115 -- parser caps output at 64 bytes.
	}
	actual, err := hasher.run(ctx, []byte(password), parsed.salt, parameters)
	if err != nil {
		return PasswordVerification{}, err
	}
	match := subtle.ConstantTimeCompare(actual, parsed.digest) == 1
	return PasswordVerification{
		Match:       match,
		NeedsRehash: match && parameters != targetArgon2Parameters,
	}, nil
}

func (hasher *argon2PasswordHasher) run(
	ctx context.Context,
	password []byte,
	salt []byte,
	parameters argon2Parameters,
) ([]byte, error) {
	waitContext, cancel := context.WithTimeout(ctx, hasher.config.AdmissionTimeout)
	defer cancel()
	if err := hasher.admission.Acquire(waitContext, 1); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, ErrHashCapacity
	}
	defer hasher.admission.Release(1)

	// Context cancellation intentionally does not interrupt Argon2 after work
	// begins. Hard parser ceilings bound the operation and the permit is held
	// until it completes.
	return hasher.derive(password, salt, parameters), nil
}

func deriveArgon2ID(password, salt []byte, parameters argon2Parameters) []byte {
	return argon2.IDKey(
		password,
		salt,
		parameters.iterations,
		parameters.memoryKiB,
		parameters.parallelism,
		parameters.outputBytes,
	)
}
