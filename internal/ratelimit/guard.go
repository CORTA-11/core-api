package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/identity"
)

const InvalidEmailIdentity = "invalid-email-v1"

type AccountIdentity struct{ value string }

type LoginGuard struct {
	limiter  Limiter
	policies Policies
}

func NewLoginGuard(limiter Limiter, policies Policies) (*LoginGuard, error) {
	if limiter == nil || policies.Validate() != nil {
		return nil, errors.New("invalid login guard configuration")
	}
	return &LoginGuard{limiter: limiter, policies: policies}, nil
}

func (guard *LoginGuard) Admit(ctx context.Context, address netip.Addr, email string) (AccountIdentity, Decision, error) {
	decision, err := guard.AdmitIP(ctx, address)
	if err != nil || !decision.Allowed {
		return AccountIdentity{}, decision, err
	}
	account, decision, err := guard.AdmitAccount(ctx, email)
	return account, decision, err
}

func (guard *LoginGuard) AdmitIP(ctx context.Context, address netip.Addr) (Decision, error) {
	if !address.IsValid() {
		return Decision{}, errors.New("trusted client address is required")
	}
	return guard.limiter.Check(ctx, guard.policies.LoginIP, address.Unmap().String(), true)
}

func (guard *LoginGuard) AdmitAccount(ctx context.Context, email string) (AccountIdentity, Decision, error) {
	account := AccountIdentity{value: InvalidEmailIdentity}
	if canonical, canonicalErr := (identity.EmailCanonicalizer{}).Canonicalize(email); canonicalErr == nil {
		account.value = canonical.Key
	}
	decision, err := guard.limiter.Check(ctx, guard.policies.AccountFailure, account.value, false)
	return account, decision, err
}

func (guard *LoginGuard) RecordFailure(ctx context.Context, account AccountIdentity) (Decision, error) {
	if account.value == "" {
		return Decision{}, errors.New("account identity is required")
	}
	return guard.limiter.Check(ctx, guard.policies.AccountFailure, account.value, true)
}

func (guard *LoginGuard) ClearSuccess(ctx context.Context, account AccountIdentity) error {
	if account.value == "" {
		return errors.New("account identity is required")
	}
	return guard.limiter.Clear(ctx, guard.policies.AccountFailure, account.value)
}

func NewAdministrativeMiddleware(limiter Limiter, policy Policy) (func(http.Handler) http.Handler, error) {
	if limiter == nil || policy.Validate() != nil {
		return nil, errors.New("invalid administrative limiter configuration")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			client, ok := httpx.ClientFromContext(request.Context())
			if !ok || !client.Address.IsValid() {
				writeAdmissionProblem(writer, request, Decision{}, errors.New("trusted client is unavailable"))
				return
			}
			decision, err := limiter.Check(request.Context(), policy, client.Address.Unmap().String(), true)
			if err != nil || !decision.Allowed {
				writeAdmissionProblem(writer, request, decision, err)
				return
			}
			next.ServeHTTP(writer, request)
		})
	}, nil
}

func writeAdmissionProblem(writer http.ResponseWriter, request *http.Request, decision Decision, err error) {
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.NewError(httpx.ProblemDependencyUnavailable, err))
		return
	}
	writer.Header().Set("Retry-After", strconv.FormatInt(ceilSeconds(decision.RetryAfter), 10))
	_ = httpx.WriteProblem(writer, request, httpx.NewError(httpx.ProblemRateLimited, nil))
}

func ceilSeconds(duration time.Duration) int64 {
	seconds := int64((duration + time.Second - 1) / time.Second)
	return max(seconds, 1)
}
