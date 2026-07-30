package prometheus

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	apperr "optimus-be/internal/infra/errors"
)

const (
	destinationDeniedKey = "observability.query.destination_denied"
	invalidURLKey        = "observability.datasource.invalid_url"
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type Policy struct {
	allowed             []netip.Prefix
	resolver            Resolver
	allowLoopbackDBTest bool
}

var permanentlyDeniedPrefixes = mustPrefixes(
	"0.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24",
	"203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	"::/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/32",
	"2001:2::/48", "2001:10::/28", "2001:20::/28", "2001:db8::/32", "2002::/16",
	"fe80::/10", "fec0::/10", "ff00::/8",
)

var metadataPrefixes = mustPrefixes("169.254.169.254/32", "fd00:ec2::254/128")

func NewPolicy(cidrs []string, resolver Resolver) (*Policy, error) {
	allowed := make([]netip.Prefix, 0, len(cidrs))
	for _, raw := range cidrs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			return nil, err
		}
		allowed = append(allowed, prefix.Masked())
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &Policy{allowed: allowed, resolver: resolver}, nil
}

// ParseBaseURL validates a data-source base URL without resolving it.
func ParseBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || parsed.Hostname() == "" {
		return nil, apperr.New(apperr.CodeObservabilityDatasourceInvalidURL, invalidURLKey, "invalid observability data source URL")
	}
	if strings.HasSuffix(parsed.Host, ":") || !validPort(parsed) || !safeEscapedPath(parsed) {
		return nil, apperr.New(apperr.CodeObservabilityDatasourceInvalidURL, invalidURLKey, "invalid observability data source URL")
	}
	return parsed, nil
}

func validPort(parsed *url.URL) bool {
	port := parsed.Port()
	if port == "" {
		return true
	}
	value, err := strconv.Atoi(port)
	return err == nil && value >= 1 && value <= 65535
}

func safeEscapedPath(parsed *url.URL) bool {
	if strings.Contains(parsed.Path, "\\") {
		return false
	}
	for _, rawSegment := range strings.Split(parsed.EscapedPath(), "/") {
		for range len(rawSegment) + 1 {
			segment, err := url.PathUnescape(rawSegment)
			if err != nil || segment == "." || segment == ".." || strings.ContainsAny(segment, "/\\") {
				return false
			}
			if segment == rawSegment {
				break
			}
			rawSegment = segment
		}
	}
	return true
}

func (p *Policy) ParseBaseURL(raw string) (*url.URL, error) { return ParseBaseURL(raw) }

func (p *Policy) ResolveAllowed(ctx context.Context, host string) ([]netip.Addr, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	addrs, err := p.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, deniedDestination()
	}
	if len(addrs) == 0 {
		return nil, deniedDestination()
	}
	result := make([]netip.Addr, 0, len(addrs))
	for _, raw := range addrs {
		addr := raw.Unmap()
		if !p.allowedAddress(addr) {
			return nil, deniedDestination()
		}
		result = append(result, addr)
	}
	return result, nil
}

func (p *Policy) allowedAddress(addr netip.Addr) bool {
	if !addr.IsValid() || containedBy(addr, metadataPrefixes) {
		return false
	}
	if addr.IsLoopback() {
		return p.allowLoopbackDBTest && containedBy(addr, p.allowed)
	}
	if !addr.IsGlobalUnicast() || containedBy(addr, permanentlyDeniedPrefixes) {
		return false
	}
	if !addr.IsPrivate() {
		return true
	}
	return containedBy(addr, p.allowed)
}

func deniedDestination() error {
	return apperr.New(apperr.CodeObservabilityQueryDestinationDenied, destinationDeniedKey, "observability query destination denied")
}

func containedBy(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func mustPrefixes(values ...string) []netip.Prefix {
	result := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		result = append(result, netip.MustParsePrefix(value))
	}
	return result
}
