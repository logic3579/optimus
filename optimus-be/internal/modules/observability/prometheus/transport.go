package prometheus

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type Auth struct {
	Type     string
	Username string
	Secret   []byte
}

type TLSOptions struct {
	SkipVerify  bool
	CustomCAPEM []byte
}

type dialContextFunc func(context.Context, string, string) (net.Conn, error)

type TransportFactory struct {
	policy *Policy
	dial   dialContextFunc
}

func NewTransportFactory(policy *Policy, dial dialContextFunc) *TransportFactory {
	if dial == nil {
		dialer := &net.Dialer{}
		dial = dialer.DialContext
	}
	return &TransportFactory{policy: policy, dial: dial}
}

func (f *TransportFactory) New(base *url.URL, tlsOpt TLSOptions, auth Auth) (*http.Client, error) {
	if f == nil || f.policy == nil || base == nil {
		return nil, errors.New("invalid Prometheus transport configuration")
	}
	validatedBase, err := ParseBaseURL(base.String())
	if err != nil {
		return nil, err
	}
	header, err := authorizationHeader(auth)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := makeTLSConfig(validatedBase.Hostname(), tlsOpt)
	if err != nil {
		return nil, err
	}
	baseHost := strings.ToLower(validatedBase.Hostname())
	template := &http.Transport{
		Proxy:             nil,
		TLSClientConfig:   tlsConfig,
		ForceAttemptHTTP2: true,
	}
	transport := &requestBoundTransport{
		template: template,
		policy:   f.policy,
		dial:     f.dial,
		baseHost: baseHost,
		active:   make(map[*http.Transport]struct{}),
	}
	origin := newOriginBinding(validatedBase)
	return &http.Client{
		Transport: &authRoundTripper{next: transport, header: header, origin: origin},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

type requestBoundTransport struct {
	template *http.Transport
	policy   *Policy
	dial     dialContextFunc
	baseHost string

	mu     sync.Mutex
	active map[*http.Transport]struct{}
}

func (r *requestBoundTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := r.template.Clone()
	requestCtx := req.Context()
	transport.DialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		ctx, cancel := context.WithCancel(dialCtx)
		stop := context.AfterFunc(requestCtx, cancel)
		defer func() {
			stop()
			cancel()
		}()
		host, port, err := net.SplitHostPort(address)
		if err != nil || strings.ToLower(strings.TrimSuffix(host, ".")) != strings.TrimSuffix(r.baseHost, ".") {
			return nil, deniedDestination()
		}
		addrs, err := r.policy.ResolveAllowed(ctx, host)
		if err != nil {
			return nil, err
		}
		return r.dial(ctx, network, net.JoinHostPort(addrs[0].String(), port))
	}
	r.mu.Lock()
	r.active[transport] = struct{}{}
	r.mu.Unlock()
	cleanup := func() {
		transport.CloseIdleConnections()
		r.mu.Lock()
		delete(r.active, transport)
		r.mu.Unlock()
	}
	response, err := transport.RoundTrip(req)
	if err != nil {
		cleanup()
		return nil, err
	}
	if response.Body == nil {
		cleanup()
		return response, nil
	}
	response.Body = &cleanupReadCloser{body: response.Body, cleanup: cleanup}
	return response, nil
}

func (r *requestBoundTransport) CloseIdleConnections() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for transport := range r.active {
		transport.CloseIdleConnections()
	}
}

type cleanupReadCloser struct {
	body    io.ReadCloser
	cleanup func()

	cleanupOnce sync.Once
	closeOnce   sync.Once
	closeErr    error
}

func (r *cleanupReadCloser) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if errors.Is(err, io.EOF) {
		r.cleanupOnce.Do(r.cleanup)
	}
	return n, err
}

func (r *cleanupReadCloser) Close() error {
	r.closeOnce.Do(func() { r.closeErr = r.body.Close() })
	r.cleanupOnce.Do(r.cleanup)
	return r.closeErr
}

type authRoundTripper struct {
	next   http.RoundTripper
	header string
	origin originBinding
}

func (r *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if !r.origin.allows(req.URL) {
		return nil, deniedDestination()
	}
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	if r.header != "" {
		clone.Header.Set("Authorization", r.header)
	}
	return r.next.RoundTrip(clone)
}

type originBinding struct {
	scheme string
	host   string
	port   string
	prefix string
}

func newOriginBinding(base *url.URL) originBinding {
	prefix := strings.TrimSuffix(base.Path, "/")
	if prefix == "" {
		prefix = "/"
	}
	return originBinding{
		scheme: base.Scheme,
		host:   strings.ToLower(base.Hostname()),
		port:   effectivePort(base),
		prefix: prefix,
	}
}

func (o originBinding) allows(target *url.URL) bool {
	if target == nil || target.User != nil || target.Fragment != "" ||
		strings.HasSuffix(target.Host, ":") ||
		target.Scheme != o.scheme || strings.ToLower(target.Hostname()) != o.host ||
		effectivePort(target) != o.port || !validPort(target) || !safeEscapedPath(target) {
		return false
	}
	path := target.Path
	if path == "" {
		path = "/"
	}
	return o.prefix == "/" || path == o.prefix || strings.HasPrefix(path, o.prefix+"/")
}

func effectivePort(target *url.URL) string {
	if port := target.Port(); port != "" {
		return port
	}
	if target.Scheme == "https" {
		return "443"
	}
	return "80"
}

func (r *authRoundTripper) CloseIdleConnections() {
	if closer, ok := r.next.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func authorizationHeader(auth Auth) (string, error) {
	switch auth.Type {
	case "":
		if auth.Username != "" || len(auth.Secret) != 0 {
			return "", errors.New("authentication material requires an authentication type")
		}
		return "", nil
	case "basic":
		if auth.Username == "" || len(auth.Secret) == 0 {
			return "", errors.New("basic authentication requires username and secret")
		}
		value := auth.Username + ":" + string(auth.Secret)
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(value)), nil
	case "bearer":
		if auth.Username != "" || len(auth.Secret) == 0 {
			return "", errors.New("bearer authentication requires only a secret")
		}
		return "Bearer " + string(auth.Secret), nil
	default:
		return "", errors.New("unsupported authentication type")
	}
}

func makeTLSConfig(serverName string, options TLSOptions) (*tls.Config, error) {
	config := &tls.Config{ServerName: serverName, InsecureSkipVerify: options.SkipVerify, MinVersion: tls.VersionTLS12} // #nosec G402 -- explicitly configured per data source.
	if len(options.CustomCAPEM) == 0 {
		return config, nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	rest := options.CustomCAPEM
	count := 0
	for len(strings.TrimSpace(string(rest))) > 0 {
		block, remainder := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("custom CA must contain only PEM certificates")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.New("custom CA contains an invalid CA certificate")
		}
		if !cert.BasicConstraintsValid || !cert.IsCA ||
			(cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageCertSign == 0) {
			return nil, errors.New("custom CA contains an invalid CA certificate")
		}
		pool.AddCert(cert)
		count++
		rest = remainder
	}
	if count == 0 {
		return nil, errors.New("custom CA contains no certificates")
	}
	config.RootCAs = pool
	return config, nil
}
