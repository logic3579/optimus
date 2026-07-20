package prometheus

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	tr := client.Transport.(*authRoundTripper).next.(*requestBoundTransport)
	require.Equal(t, "prom.example.com", tr.template.TLSClientConfig.ServerName)
	_, err = client.Get(base.String())
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
	_, _ = client.Get(base.String())
	_, err = client.Get(base.String())
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
	var calls atomic.Int32
	client.Transport.(*authRoundTripper).next = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"http://169.254.169.254/latest/meta-data"}},
			Body:       io.NopCloser(strings.NewReader("redirect")),
			Request:    req,
		}, nil
	})
	resp, err := client.Get(base.String())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, int32(1), calls.Load(), "redirect target must not be requested")
}

func TestTransportAllowsCallerToCloseIdleConnections(t *testing.T) {
	spy := &closeIdleSpy{}
	rt := &authRoundTripper{next: spy}
	rt.CloseIdleConnections()
	require.Equal(t, int32(1), spy.calls.Load())
}

type closeIdleSpy struct{ calls atomic.Int32 }

func (s *closeIdleSpy) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unused")
}
func (s *closeIdleSpy) CloseIdleConnections() { s.calls.Add(1) }

func TestTransportCanceledDialDoesNotContinue(t *testing.T) {
	resolver := &blockingResolver{started: make(chan struct{}), exited: make(chan struct{})}
	policy, err := NewPolicy(nil, resolver)
	require.NoError(t, err)
	var dials atomic.Int32
	factory := NewTransportFactory(policy, func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("must not dial")
	})
	base, err := policy.ParseBaseURL("http://prom.example.com")
	require.NoError(t, err)
	client, err := factory.New(base, TLSOptions{}, Auth{})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		_, requestErr := client.Do(req)
		result <- requestErr
	}()

	select {
	case <-resolver.started:
	case <-time.After(time.Second):
		t.Fatal("transport resolution did not start")
	}
	cancel()
	select {
	case <-resolver.exited:
	case <-time.After(time.Second):
		t.Fatal("resolver did not exit after cancellation")
	}
	select {
	case err = <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("request did not return after cancellation")
	}
	require.Equal(t, int32(0), dials.Load())
	require.Equal(t, int32(1), resolver.calls.Load())
}

type blockingResolver struct {
	started chan struct{}
	exited  chan struct{}
	calls   atomic.Int32
}

func (r *blockingResolver) LookupNetIP(ctx context.Context, _, _ string) ([]netip.Addr, error) {
	r.calls.Add(1)
	close(r.started)
	<-ctx.Done()
	close(r.exited)
	return nil, ctx.Err()
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

func TestTransportAcceptsCAAndRejectsLeafCertificate(t *testing.T) {
	policy, err := NewPolicy(nil, &staticResolver{addrs: parseAddrs(t, "8.8.8.8")})
	require.NoError(t, err)
	factory := NewTransportFactory(policy, nil)
	base, err := policy.ParseBaseURL("https://prom.example.com")
	require.NoError(t, err)

	caPEM := generatedCertificatePEM(t, true, x509.KeyUsageCertSign|x509.KeyUsageCRLSign)
	_, err = factory.New(base, TLSOptions{CustomCAPEM: caPEM}, Auth{})
	require.NoError(t, err)

	leafPEM := generatedCertificatePEM(t, false, x509.KeyUsageDigitalSignature)
	_, err = factory.New(base, TLSOptions{CustomCAPEM: leafPEM}, Auth{})
	require.Error(t, err)
	require.Equal(t, "custom CA contains an invalid CA certificate", err.Error())
}

func generatedCertificatePEM(t *testing.T, isCA bool, usage x509.KeyUsage) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          new(big.Int).SetInt64(1),
		Subject:               pkix.Name{CommonName: "test certificate"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		KeyUsage:              usage,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
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
	tr := client.Transport.(*authRoundTripper).next.(*requestBoundTransport)
	require.True(t, tr.template.TLSClientConfig.InsecureSkipVerify)
	require.Equal(t, "prom.example.com", tr.template.TLSClientConfig.ServerName)
	require.NotNil(t, (*tls.Config)(tr.template.TLSClientConfig))
}
