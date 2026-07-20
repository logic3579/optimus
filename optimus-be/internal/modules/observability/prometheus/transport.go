package prometheus

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
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
	transport := &http.Transport{
		Proxy:             nil,
		TLSClientConfig:   tlsConfig,
		ForceAttemptHTTP2: true,
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || strings.ToLower(strings.TrimSuffix(host, ".")) != strings.TrimSuffix(baseHost, ".") {
			return nil, deniedDestination()
		}
		addrs, err := f.policy.ResolveAllowed(ctx, host)
		if err != nil {
			return nil, err
		}
		return f.dial(ctx, network, net.JoinHostPort(addrs[0].String(), port))
	}
	return &http.Client{
		Transport: &authRoundTripper{next: transport, header: header},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

type authRoundTripper struct {
	next   http.RoundTripper
	header string
}

func (r *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	if r.header != "" {
		clone.Header.Set("Authorization", r.header)
	}
	return r.next.RoundTrip(clone)
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
