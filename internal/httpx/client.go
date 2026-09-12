package httpx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
)

const maximumForwardedHops = 10

var ErrInvalidForwarding = errors.New("invalid forwarding headers")

type Client struct {
	Address netip.Addr
	Scheme  string
	Host    string
}

type TrustedProxies struct{ prefixes []netip.Prefix }

type clientContextKey struct{}

func ParseTrustedProxies(raw string) (TrustedProxies, error) {
	var policy TrustedProxies
	if strings.TrimSpace(raw) == "" {
		return policy, nil
	}
	seen := make(map[netip.Prefix]struct{})
	for _, item := range strings.Split(raw, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			return TrustedProxies{}, fmt.Errorf("parse trusted proxy CIDR: %w", err)
		}
		prefix = normalizePrefix(prefix)
		if _, exists := seen[prefix]; exists {
			return TrustedProxies{}, fmt.Errorf("duplicate trusted proxy CIDR")
		}
		seen[prefix] = struct{}{}
		policy.prefixes = append(policy.prefixes, prefix)
	}
	return policy, nil
}

func (policy TrustedProxies) CIDRs() []string {
	result := make([]string, len(policy.prefixes))
	for index, prefix := range policy.prefixes {
		result[index] = prefix.String()
	}
	return result
}

func (policy TrustedProxies) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		client, err := policy.Derive(request)
		if err != nil {
			_ = WriteProblem(writer, request, NewError(ProblemInvalidRequest, err))
			return
		}
		ctx := context.WithValue(request.Context(), clientContextKey{}, client)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func ClientFromContext(ctx context.Context) (Client, bool) {
	client, ok := ctx.Value(clientContextKey{}).(Client)
	return client, ok
}

func (policy TrustedProxies) Derive(request *http.Request) (Client, error) {
	peer, err := parseAddress(request.RemoteAddr)
	if err != nil {
		return Client{}, ErrInvalidForwarding
	}
	client := Client{Address: peer, Scheme: requestScheme(request), Host: request.Host}
	if !policy.contains(peer) {
		return client, nil
	}
	standard := request.Header.Values("Forwarded")
	xFamily := hasXForwarding(request.Header)
	if len(standard) > 0 && xFamily {
		return Client{}, ErrInvalidForwarding
	}
	if len(standard) > 0 {
		return policy.deriveForwarded(client, strings.Join(standard, ","))
	}
	if xFamily {
		return policy.deriveXForwarded(client, request.Header)
	}
	return client, nil
}

type forwardedHop struct {
	address netip.Addr
	proto   string
	host    string
}

func (policy TrustedProxies) deriveForwarded(base Client, raw string) (Client, error) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 || len(parts) > maximumForwardedHops {
		return Client{}, ErrInvalidForwarding
	}
	hops := make([]forwardedHop, 0, len(parts))
	for _, part := range parts {
		var hop forwardedHop
		for _, parameter := range strings.Split(part, ";") {
			name, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if !ok || value == "" {
				return Client{}, ErrInvalidForwarding
			}
			value = strings.Trim(value, `"`)
			switch strings.ToLower(name) {
			case "for":
				if hop.address.IsValid() {
					return Client{}, ErrInvalidForwarding
				}
				address, err := parseAddress(value)
				if err != nil {
					return Client{}, ErrInvalidForwarding
				}
				hop.address = address
			case "proto":
				hop.proto = strings.ToLower(value)
			case "host":
				hop.host = value
			}
		}
		if !hop.address.IsValid() || !validForwardedMetadata(hop.proto, hop.host) {
			return Client{}, ErrInvalidForwarding
		}
		hops = append(hops, hop)
	}
	base.Address = policy.clientAddress(hopAddresses(hops), base.Address)
	for _, hop := range hops {
		if hop.proto != "" {
			base.Scheme = hop.proto
		}
		if hop.host != "" {
			base.Host = hop.host
		}
		if hop.proto != "" || hop.host != "" {
			break
		}
	}
	return base, nil
}

func (policy TrustedProxies) deriveXForwarded(base Client, header http.Header) (Client, error) {
	rawAddresses := strings.Join(header.Values("X-Forwarded-For"), ",")
	parts := strings.Split(rawAddresses, ",")
	if rawAddresses == "" || len(parts) > maximumForwardedHops {
		return Client{}, ErrInvalidForwarding
	}
	addresses := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		address, err := parseAddress(strings.TrimSpace(part))
		if err != nil {
			return Client{}, ErrInvalidForwarding
		}
		addresses = append(addresses, address)
	}
	proto := singleForwardedValue(header, "X-Forwarded-Proto")
	host := singleForwardedValue(header, "X-Forwarded-Host")
	if !validForwardedMetadata(proto, host) {
		return Client{}, ErrInvalidForwarding
	}
	base.Address = policy.clientAddress(addresses, base.Address)
	if proto != "" {
		base.Scheme = strings.ToLower(proto)
	}
	if host != "" {
		base.Host = host
	}
	return base, nil
}

func (policy TrustedProxies) clientAddress(chain []netip.Addr, direct netip.Addr) netip.Addr {
	selected := direct
	for index := len(chain) - 1; index >= 0 && policy.contains(selected); index-- {
		selected = chain[index].Unmap()
	}
	return selected
}

func (policy TrustedProxies) contains(address netip.Addr) bool {
	address = address.Unmap()
	return slices.ContainsFunc(policy.prefixes, func(prefix netip.Prefix) bool { return prefix.Contains(address) })
}

func normalizePrefix(prefix netip.Prefix) netip.Prefix {
	if prefix.Addr().Is4In6() && prefix.Bits() >= 96 {
		return netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96).Masked()
	}
	return prefix.Masked()
}

func parseAddress(value string) (netip.Addr, error) {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	if address, err := netip.ParseAddr(value); err == nil {
		return address.Unmap(), nil
	}
	if addressPort, err := netip.ParseAddrPort(value); err == nil {
		return addressPort.Addr().Unmap(), nil
	}
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		return netip.Addr{}, err
	}
	address, err := netip.ParseAddr(host)
	return address.Unmap(), err
}

func requestScheme(request *http.Request) string {
	if request.TLS != nil {
		return "https"
	}
	return "http"
}

func hasXForwarding(header http.Header) bool {
	return header.Get("X-Forwarded-For") != "" || header.Get("X-Forwarded-Proto") != "" || header.Get("X-Forwarded-Host") != ""
}

func singleForwardedValue(header http.Header, name string) string {
	values := header.Values(name)
	if len(values) > 1 || (len(values) == 1 && strings.Contains(values[0], ",")) {
		return "\x00"
	}
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func validForwardedMetadata(proto, host string) bool {
	if proto != "" && proto != "http" && proto != "https" {
		return false
	}
	if host == "" {
		return true
	}
	parsed, err := url.Parse("//" + host)
	return err == nil && parsed.Host == host && parsed.Hostname() != "" && parsed.User == nil && !strings.ContainsAny(host, "\x00/@?#")
}

func hopAddresses(hops []forwardedHop) []netip.Addr {
	addresses := make([]netip.Addr, len(hops))
	for index := range hops {
		addresses[index] = hops[index].address
	}
	return addresses
}
