// Package ratelimit owns shared admission policies and their Redis-backed state.
package ratelimit

import (
	"errors"
	"strings"
	"time"
)

type Policy struct {
	Bucket string
	Limit  int64
	Period time.Duration
	Burst  int64
}

type Policies struct {
	LoginIP        Policy
	RegistrationIP Policy
	AccountFailure Policy
	Administrative Policy
}

// DefaultPolicies returns the default rate-limit policies.
func DefaultPolicies() Policies {
	return Policies{
		LoginIP:        Policy{"login-ip", 20, 15 * time.Minute, 20},
		RegistrationIP: Policy{"registration-ip", 5, 15 * time.Minute, 5},
		AccountFailure: Policy{"account-failure", 5, 15 * time.Minute, 5},
		Administrative: Policy{"administrative", 60, 15 * time.Minute, 20},
	}
}

// Validate checks whether a rate-limit policy is within the supported bounds.
func (policy Policy) Validate() error {
	if policy.Bucket == "" || len(policy.Bucket) > 32 || strings.ContainsAny(policy.Bucket, ": \t\r\n") {
		return errors.New("rate-limit bucket is invalid")
	}
	if policy.Limit < 1 || policy.Limit > 10_000 || policy.Burst < 1 || policy.Burst > policy.Limit ||
		policy.Period < time.Second || policy.Period > time.Hour {
		return errors.New("rate-limit policy is outside its bounds")
	}
	if policy.Period/time.Duration(policy.Limit) < time.Microsecond {
		return errors.New("rate-limit interval is too small")
	}
	return nil
}

// Validate checks whether all rate-limit policies are valid.
func (policies Policies) Validate() error {
	for _, policy := range []Policy{policies.LoginIP, policies.RegistrationIP, policies.AccountFailure, policies.Administrative} {
		if err := policy.Validate(); err != nil {
			return err
		}
	}
	return nil
}
