package prometheus

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
)

type resolverSequence struct {
	answers [][]netip.Addr
	calls   int
}

func (r *resolverSequence) LookupNetIP(ctx context.Context, _, _ string) ([]netip.Addr, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	i := r.calls
	r.calls++
	if i >= len(r.answers) {
		i = len(r.answers) - 1
	}
	return r.answers[i], nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTransportDialsValidatedIPAndPreservesSNI(t *testing.T) {
	resolver := &staticResolver{addrs: parseAddrs(t, "8.8.8.8")}
	policy, err := NewPolicy(nil, resolver)
	require.NoError(t, err)
	var dialAddress string
	factory := NewTransportFactory(policy, func(_ context.Context, network, address string) (net.Conn, error) {
		dialAddress = address
		return nil, errors.New("stop after address capture")
	})
	base, err := policy.ParseBaseURL("https://prom.example.com:9443")
	require.NoError(t, err)
	client, err := factory.New(base, TLSOptions{}, Auth{})
	require.NoError(t, err)
	tr := client.Transport.(*authRoundTripper).next.(*http.Transport)
	require.Equal(t, "prom.example.com", tr.TLSClientConfig.ServerName)
	_, err = tr.DialContext(context.Background(), "tcp", "prom.example.com:9443")
	require.Error(t, err)
	require.Equal(t, "8.8.8.8:9443", dialAddress)
}

func TestTransportRevalidatesEveryConnection(t *testing.T) {
	resolver := &resolverSequence{answers: [][]netip.Addr{parseAddrs(t, "8.8.8.8"), parseAddrs(t, "169.254.169.254")}}
	policy, err := NewPolicy(nil, resolver)
	require.NoError(t, err)
	var dials int
	factory := NewTransportFactory(policy, func(context.Context, string, string) (net.Conn, error) { dials++; return nil, errors.New("dial") })
	base, err := policy.ParseBaseURL("http://prom.example.com")
	require.NoError(t, err)
	client, err := factory.New(base, TLSOptions{}, Auth{})
	require.NoError(t, err)
	tr := client.Transport.(*authRoundTripper).next.(*http.Transport)
	_, _ = tr.DialContext(context.Background(), "tcp", "prom.example.com:80")
	_, err = tr.DialContext(context.Background(), "tcp", "prom.example.com:80")
	requireBizCode(t, err, apperr.CodeObservabilityQueryDestinationDenied)
	require.Equal(t, 1, dials)
}

func TestTransportAuthenticationAndRedaction(t *testing.T) {
	tests := []struct {
		name string
		auth Auth
		want string
	}{
		{"basic", Auth{Type: "basic", Username: "alice", Secret: []byte("super-secret-password")}, "Basic YWxpY2U6c3VwZXItc2VjcmV0LXBhc3N3b3Jk"},
		{"bearer", Auth{Type: "bearer", Secret: []byte("super-secret-token")}, "Bearer super-secret-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := NewPolicy(nil, &staticResolver{addrs: parseAddrs(t, "8.8.8.8")})
			require.NoError(t, err)
			factory := NewTransportFactory(policy, nil)
			base, err := policy.ParseBaseURL("https://prom.example.com")
			require.NoError(t, err)
			client, err := factory.New(base, TLSOptions{}, tt.auth)
			require.NoError(t, err)
			authRT := client.Transport.(*authRoundTripper)
			authRT.next = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, tt.want, req.Header.Get("Authorization"))
				return nil, errors.New("upstream failed")
			})
			req, err := http.NewRequest(http.MethodGet, base.String(), nil)
			require.NoError(t, err)
			_, err = client.Do(req)
			require.Error(t, err)
			require.NotContains(t, err.Error(), string(tt.auth.Secret))
			require.Empty(t, req.Header.Get("Authorization"), "caller request must not be mutated")
		})
	}
}

func TestTransportDisablesRedirects(t *testing.T) {
	policy, err := NewPolicy(nil, &staticResolver{addrs: parseAddrs(t, "8.8.8.8")})
	require.NoError(t, err)
	factory := NewTransportFactory(policy, nil)
	base, err := policy.ParseBaseURL("https://prom.example.com")
	require.NoError(t, err)
	client, err := factory.New(base, TLSOptions{}, Auth{})
	require.NoError(t, err)
	require.ErrorIs(t, client.CheckRedirect(nil, nil), http.ErrUseLastResponse)
}

func TestTransportAllowsCallerToCloseIdleConnections(t *testing.T) {
	policy, err := NewPolicy(nil, &staticResolver{addrs: parseAddrs(t, "8.8.8.8")})
	require.NoError(t, err)
	base, err := policy.ParseBaseURL("https://prom.example.com")
	require.NoError(t, err)
	client, err := NewTransportFactory(policy, nil).New(base, TLSOptions{}, Auth{})
	require.NoError(t, err)
	_, ok := client.Transport.(interface{ CloseIdleConnections() })
	require.True(t, ok)
}

func TestTransportRejectsInvalidAuthAndCA(t *testing.T) {
	policy, err := NewPolicy(nil, &staticResolver{addrs: parseAddrs(t, "8.8.8.8")})
	require.NoError(t, err)
	factory := NewTransportFactory(policy, nil)
	base, err := policy.ParseBaseURL("https://prom.example.com")
	require.NoError(t, err)
	for _, auth := range []Auth{{Type: "digest", Secret: []byte("hidden")}, {Type: "basic", Secret: []byte("hidden")}, {Type: "bearer", Username: "not-allowed", Secret: []byte("hidden")}} {
		_, err := factory.New(base, TLSOptions{}, auth)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "hidden")
	}
	_, err = factory.New(base, TLSOptions{CustomCAPEM: []byte("not a certificate")}, Auth{})
	require.Error(t, err)
}

func TestAuthRoundTripperDoesNotLeakHeaderInErrors(t *testing.T) {
	rt := &authRoundTripper{next: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
	}), header: "Bearer secret"}
	req := &http.Request{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "example.com"}, Header: make(http.Header)}
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, "Bearer secret", resp.Request.Header.Get("Authorization"))
	require.Empty(t, req.Header.Get("Authorization"))
}

func TestTransportTLSOptionsAreApplied(t *testing.T) {
	policy, err := NewPolicy(nil, &staticResolver{addrs: parseAddrs(t, "8.8.8.8")})
	require.NoError(t, err)
	base, err := policy.ParseBaseURL("https://prom.example.com")
	require.NoError(t, err)
	client, err := NewTransportFactory(policy, nil).New(base, TLSOptions{SkipVerify: true}, Auth{})
	require.NoError(t, err)
	tr := client.Transport.(*authRoundTripper).next.(*http.Transport)
	require.True(t, tr.TLSClientConfig.InsecureSkipVerify)
	require.Equal(t, "prom.example.com", tr.TLSClientConfig.ServerName)
	require.NotNil(t, (*tls.Config)(tr.TLSClientConfig))
}
