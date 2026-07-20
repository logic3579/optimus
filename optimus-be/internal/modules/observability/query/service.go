package query

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/assets"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/observability/prometheus"
)

type Datasource struct {
	ID                uint64
	BaseURL, AuthType string
	CredentialID      *uint64
	TLSSkipVerify     bool
	CustomCAPEM       []byte
}
type DatasourceLoader interface {
	Load(context.Context, uint64) (Datasource, error)
}
type DatasourceLoaderFunc func(context.Context, uint64) (Datasource, error)

func (f DatasourceLoaderFunc) Load(ctx context.Context, id uint64) (Datasource, error) {
	return f(ctx, id)
}

type CredentialConsumer interface {
	GetHTTPCredential(context.Context, uint64, string) (*credentials.HTTPCredential, error)
}
type Runner interface {
	Query(context.Context, string, time.Time) (prometheus.Result, error)
	QueryRange(context.Context, string, time.Time, time.Time, time.Duration) (prometheus.Result, error)
	Labels(context.Context) ([]string, error)
	LabelValues(context.Context, string) ([]string, error)
}
type ClientFactory interface {
	Build(context.Context, Datasource, *credentials.HTTPCredential) (Runner, func(), error)
}
type Limits struct {
	MaxBatch, MaxConcurrent, MaxPromQLBytes int
	MaxRange, MinStep, Timeout              time.Duration
	MaxPoints, MaxEnrichmentIPs             int
}

var labelNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type Service struct {
	loader      DatasourceLoader
	credentials CredentialConsumer
	clients     ClientFactory
	assets      assets.Consumer
	limits      Limits
}

func NewService(l DatasourceLoader, c CredentialConsumer, f ClientFactory, a assets.Consumer, limits Limits) *Service {
	return &Service{loader: l, credentials: c, clients: f, assets: a, limits: limits}
}

func (s *Service) Instant(ctx context.Context, actor uint64, req InstantRequest) (*BatchResult, error) {
	if err := s.validate(req.DatasourceID, req.Queries); err != nil {
		return nil, err
	}
	return s.run(ctx, actor, req.DatasourceID, "observability.query.instant", req.Queries, req.EnrichAssets, func(ctx context.Context, r Runner, q string) (prometheus.Result, error) {
		return r.Query(ctx, q, req.Time)
	})
}
func (s *Service) Range(ctx context.Context, actor uint64, req RangeRequest) (*BatchResult, error) {
	if err := s.validate(req.DatasourceID, req.Queries); err != nil {
		return nil, err
	}
	span := req.End.Sub(req.Start)
	if req.Start.IsZero() || req.End.IsZero() || span < 0 {
		return nil, invalid()
	}
	if req.Step <= 0 || req.Step < s.limits.MinStep {
		return nil, invalid()
	}
	if span > s.limits.MaxRange || int64(span/req.Step)+1 > int64(s.limits.MaxPoints) {
		return nil, limit()
	}
	return s.run(ctx, actor, req.DatasourceID, "observability.query.range", req.Queries, req.EnrichAssets, func(ctx context.Context, r Runner, q string) (prometheus.Result, error) {
		return r.QueryRange(ctx, q, req.Start, req.End, req.Step)
	})
}
func (s *Service) validate(id uint64, qs []Query) error {
	if id == 0 || len(qs) == 0 {
		return invalid()
	}
	if len(qs) > s.limits.MaxBatch {
		return limit()
	}
	seen := map[string]struct{}{}
	for _, q := range qs {
		ref := strings.TrimSpace(q.RefID)
		if ref == "" || strings.TrimSpace(q.PromQL) == "" {
			return invalid()
		}
		if _, ok := seen[ref]; ok {
			return invalid()
		}
		seen[ref] = struct{}{}
		if len(q.PromQL) > s.limits.MaxPromQLBytes {
			return limit()
		}
	}
	return nil
}
func (s *Service) prepare(ctx context.Context, actor, id uint64, purpose string) (Runner, func(), error) {
	d, err := s.loader.Load(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	var secret *credentials.HTTPCredential
	if d.AuthType != "none" {
		if s.credentials == nil || d.CredentialID == nil {
			return nil, nil, invalid()
		}
		secret, err = s.credentials.GetHTTPCredential(credentials.WithActor(ctx, actor), *d.CredentialID, purpose)
		if err != nil {
			return nil, nil, err
		}
	}
	runner, closeFn, err := s.clients.Build(ctx, d, secret)
	if err != nil {
		credentials.WipeHTTPCredential(secret)
		return nil, nil, err
	}
	return runner, func() {
		if closeFn != nil {
			closeFn()
		}
		credentials.WipeHTTPCredential(secret)
	}, nil
}
func (s *Service) run(ctx context.Context, actor, id uint64, purpose string, qs []Query, enrichAssets bool, call func(context.Context, Runner, string) (prometheus.Result, error)) (*BatchResult, error) {
	runner, cleanup, err := s.prepare(ctx, actor, id, purpose)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	runCtx := ctx
	cancel := func() {}
	if s.limits.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, s.limits.Timeout)
	}
	defer cancel()
	out := make([]ItemResult, len(qs))
	g, gctx := errgroup.WithContext(runCtx)
	concurrency := s.limits.MaxConcurrent
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	for i := range qs {
		i := i
		g.Go(func() error {
			select {
			case sem <- struct{}{}:
			case <-gctx.Done():
				return gctx.Err()
			}
			defer func() { <-sem }()
			result, err := call(gctx, runner, qs[i].PromQL)
			out[i].RefID = qs[i].RefID
			if err == nil {
				out[i].Result = &result
				return nil
			}
			if be, ok := apperr.AsBiz(err); ok && be.Code == apperr.CodeObservabilityQueryUpstreamRejected && prometheus.IsExpressionError(err) {
				out[i].Error = &ItemError{Code: int(be.Code), Message: be.Message, MessageKey: be.MessageKey}
				return nil
			}
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	batch := &BatchResult{Results: out}
	if enrichAssets {
		batch.AssetContext = enrich(ctx, s.assets, out, s.limits.MaxEnrichmentIPs)
	}
	return batch, nil
}
func (s *Service) Labels(ctx context.Context, actor, id uint64) ([]string, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	r, cleanup, err := s.prepare(ctx, actor, id, "observability.query.metadata")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return r.Labels(ctx)
}
func (s *Service) LabelValues(ctx context.Context, actor, id uint64, label string) ([]string, error) {
	if !labelNamePattern.MatchString(label) {
		return nil, invalid()
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	r, cleanup, err := s.prepare(ctx, actor, id, "observability.query.metadata")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return r.LabelValues(ctx, label)
}
func (s *Service) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.limits.Timeout > 0 {
		return context.WithTimeout(ctx, s.limits.Timeout)
	}
	return ctx, func() {}
}
func invalid() error {
	return apperr.New(apperr.CodeObservabilityQueryInvalidRequest, "observability.query.invalid_request", "invalid query request")
}
func limit() error {
	return apperr.New(apperr.CodeObservabilityQueryLimitExceeded, "observability.query.limit_exceeded", "query limit exceeded")
}

type PrometheusClientFactory struct {
	Transport *prometheus.TransportFactory
	MaxBody   int64
	MaxSeries int
}

func (f PrometheusClientFactory) Build(_ context.Context, d Datasource, secret *credentials.HTTPCredential) (Runner, func(), error) {
	base, err := url.Parse(d.BaseURL)
	if err != nil {
		return nil, nil, invalid()
	}
	auth := prometheus.Auth{Type: d.AuthType}
	if auth.Type == "none" {
		auth.Type = ""
	}
	if secret != nil {
		auth.Username = secret.Username
		auth.Secret = secret.Secret
	}
	hc, err := f.Transport.New(base, prometheus.TLSOptions{SkipVerify: d.TLSSkipVerify, CustomCAPEM: d.CustomCAPEM}, auth)
	if err != nil {
		return nil, nil, err
	}
	return prometheus.NewClient(hc, base, f.MaxBody, f.MaxSeries), hc.CloseIdleConnections, nil
}
