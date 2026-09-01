package httpx

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
)

const defaultAllowedOrigins = "http://localhost:3000,http://127.0.0.1:3000"

type OriginPolicy struct {
	values  []string
	allowed map[string]struct{}
}

// ParseOriginPolicy parses origin policy.
func ParseOriginPolicy(raw, environment string) (OriginPolicy, error) {
	if strings.TrimSpace(raw) == "" {
		raw = defaultAllowedOrigins
	}
	policy := OriginPolicy{allowed: make(map[string]struct{})}
	for _, item := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(item)
		parsed, err := url.Parse(origin)
		if err != nil || origin == "" || origin == "*" || strings.EqualFold(origin, "null") ||
			parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" ||
			parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
			(parsed.Scheme != "https" && parsed.Scheme != "http") || !validOriginHost(parsed) {
			return OriginPolicy{}, fmt.Errorf("origin must contain only an http(s) scheme and host")
		}
		if environment == "production" && parsed.Scheme != "https" {
			return OriginPolicy{}, fmt.Errorf("production origins must use https")
		}
		if parsed.Scheme == "http" && !isLoopbackHostname(parsed.Hostname()) {
			return OriginPolicy{}, fmt.Errorf("http origins must use localhost or a loopback address")
		}
		if _, exists := policy.allowed[origin]; exists {
			return OriginPolicy{}, fmt.Errorf("duplicate origin")
		}
		policy.allowed[origin] = struct{}{}
		policy.values = append(policy.values, origin)
	}
	if len(policy.values) == 0 {
		return OriginPolicy{}, fmt.Errorf("at least one origin is required")
	}
	return policy, nil
}

// Allows handles the allows operation.
func (policy OriginPolicy) Allows(origin string) bool {
	_, ok := policy.allowed[origin]
	return ok && origin != ""
}

// Values handles the values operation.
func (policy OriginPolicy) Values() []string { return slices.Clone(policy.values) }

// validOriginHost checks whether origin host is valid.
func validOriginHost(origin *url.URL) bool {
	hostname := origin.Hostname()
	if hostname == "" || strings.ContainsAny(hostname, "*%") {
		return false
	}
	if port := origin.Port(); port != "" {
		if _, err := net.LookupPort("tcp", port); err != nil {
			return false
		}
	}
	return true
}

// isLoopbackHostname checks whether loopback hostname.
func isLoopbackHostname(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
