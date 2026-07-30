package prometheus

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
)

type staticResolver struct {
	addrs []netip.Addr
	err   error
}

func (r *staticResolver) LookupNetIP(ctx context.Context, _, _ string) ([]netip.Addr, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.addrs, r.err
}

func TestPolicyParseBaseURL(t *testing.T) {
	p, err := NewPolicy(nil, &staticResolver{})
	require.NoError(t, err)

	for _, raw := range []string{
		"file:///etc/passwd", "ftp://prom.example.com", "https://user:pass@prom.example.com",
		"https://prom.example.com?x=1", "https://prom.example.com/#frag", "https:///prefix",
		"https://prom.example.com:", "https://prom.example.com:0", "https://prom.example.com:65536",
		"https://prom.example.com:not-a-port",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := p.ParseBaseURL(raw)
			requireBizCode(t, err, apperr.CodeObservabilityDatasourceInvalidURL)
		})
	}

	got, err := p.ParseBaseURL("https://prom.example.com/prefix/")
	require.NoError(t, err)
	require.Equal(t, "prom.example.com", got.Hostname())
}

func TestPolicyResolveAllowedClassifiesEveryAnswer(t *testing.T) {
	tests := []struct {
		name    string
		ips     []string
		allowed []string
		ok      bool
	}{
		{"public", []string{"8.8.8.8", "2606:4700:4700::1111"}, nil, true},
		{"ipv4 unspecified", []string{"0.0.0.0"}, nil, false},
		{"ipv6 unspecified", []string{"::"}, nil, false},
		{"ipv4 loopback", []string{"127.0.0.1"}, []string{"127.0.0.0/8"}, false},
		{"ipv6 loopback", []string{"::1"}, []string{"::1/128"}, false},
		{"ipv4 link local", []string{"169.254.1.1"}, []string{"169.254.0.0/16"}, false},
		{"ipv6 link local", []string{"fe80::1"}, []string{"fe80::/10"}, false},
		{"ipv4 multicast", []string{"224.0.0.1"}, nil, false},
		{"ipv6 multicast", []string{"ff02::1"}, nil, false},
		{"ipv4 documentation", []string{"192.0.2.4"}, nil, false},
		{"ipv6 documentation", []string{"2001:db8::4"}, nil, false},
		{"ipv4 benchmark", []string{"198.18.0.1"}, nil, false},
		{"ipv4 reserved", []string{"240.0.0.1"}, nil, false},
		{"ipv6 discard only", []string{"100::1"}, nil, false},
		{"ipv6 benchmark", []string{"2001:2::1"}, nil, false},
		{"ipv6 orchid", []string{"2001:20::1"}, nil, false},
		{"ipv6 deprecated site local", []string{"fec0::1"}, nil, false},
		{"NAT64 encoded private", []string{"64:ff9b::a00:1"}, []string{"64:ff9b::/96"}, false},
		{"deprecated IPv4-compatible private", []string{"::a00:1"}, []string{"::/0"}, false},
		{"deprecated IPv4-compatible metadata", []string{"::a9fe:a9fe"}, []string{"::/0"}, false},
		{"NAT64 encoded metadata", []string{"64:ff9b::a9fe:a9fe"}, []string{"::/0"}, false},
		{"local NAT64 encoded private", []string{"64:ff9b:1::a00:1"}, []string{"::/0"}, false},
		{"6to4 encoded private", []string{"2002:a00:1::"}, []string{"::/0"}, false},
		{"6to4 encoded metadata", []string{"2002:a9fe:a9fe::"}, []string{"::/0"}, false},
		{"Teredo denied", []string{"2001:0:4136:e378:8000:63bf:3fff:fdd2"}, nil, false},
		{"private denied", []string{"10.2.3.4"}, nil, false},
		{"private allowed", []string{"10.2.3.4"}, []string{"10.2.3.0/24"}, true},
		{"ULA denied", []string{"fd12::4"}, nil, false},
		{"ULA allowed", []string{"fd12::4"}, []string{"fd12::/64"}, true},
		{"metadata ipv4 always denied", []string{"169.254.169.254"}, []string{"0.0.0.0/0"}, false},
		{"metadata ipv6 always denied", []string{"fd00:ec2::254"}, []string{"::/0"}, false},
		{"mapped loopback denied", []string{"::ffff:127.0.0.1"}, nil, false},
		{"mapped private allowed", []string{"::ffff:10.2.3.4"}, []string{"10.2.3.0/24"}, true},
		{"mixed answer denied", []string{"8.8.8.8", "127.0.0.1"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewPolicy(tt.allowed, &staticResolver{addrs: parseAddrs(t, tt.ips...)})
			require.NoError(t, err)
			got, err := p.ResolveAllowed(context.Background(), "prom.example.com")
			if tt.ok {
				require.NoError(t, err)
				require.NotEmpty(t, got)
				for _, addr := range got {
					require.False(t, addr.Is4In6())
				}
				return
			}
			requireBizCode(t, err, apperr.CodeObservabilityQueryDestinationDenied)
		})
	}
}

func TestPolicyResolveAllowedHandlesResolverFailures(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		p, err := NewPolicy(nil, &staticResolver{})
		require.NoError(t, err)
		_, err = p.ResolveAllowed(context.Background(), "empty.example.com")
		requireBizCode(t, err, apperr.CodeObservabilityQueryDestinationDenied)
		require.NotContains(t, err.Error(), "empty.example.com")
	})
	t.Run("resolver detail redacted", func(t *testing.T) {
		p, err := NewPolicy(nil, &staticResolver{err: errors.New("dns backend secret detail")})
		require.NoError(t, err)
		_, err = p.ResolveAllowed(context.Background(), "prom.example.com")
		requireBizCode(t, err, apperr.CodeObservabilityQueryDestinationDenied)
		require.NotContains(t, err.Error(), "secret detail")
	})
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		p, err := NewPolicy(nil, &staticResolver{})
		require.NoError(t, err)
		_, err = p.ResolveAllowed(ctx, "prom.example.com")
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestPolicyRejectsInvalidAllowlist(t *testing.T) {
	_, err := NewPolicy([]string{"not-a-cidr"}, &staticResolver{})
	require.Error(t, err)
}

func parseAddrs(t *testing.T, values ...string) []netip.Addr {
	t.Helper()
	result := make([]netip.Addr, 0, len(values))
	for _, value := range values {
		addr, err := netip.ParseAddr(value)
		require.NoError(t, err)
		result = append(result, addr)
	}
	return result
}

func requireBizCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	require.Error(t, err)
	got, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, code, got.Code)
}
