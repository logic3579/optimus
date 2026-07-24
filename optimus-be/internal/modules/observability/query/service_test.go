package query

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/observability/prometheus"
)

type fakeLoader struct {
	calls atomic.Int32
	cfg   Datasource
	err   error
}

func (f *fakeLoader) Load(context.Context, uint64) (Datasource, error) {
	f.calls.Add(1)
	return f.cfg, f.err
}

type fakeConsumer struct {
	calls   atomic.Int32
	purpose string
	secret  *credentials.HTTPCredential
	err     error
}

func (f *fakeConsumer) GetHTTPCredential(_ context.Context, _ uint64, purpose string) (*credentials.HTTPCredential, error) {
	f.calls.Add(1)
	f.purpose = purpose
	return f.secret, f.err
}

type fakeClientFactory struct {
	runner Runner
	close  atomic.Int32
}

func (f *fakeClientFactory) Build(context.Context, Datasource, *credentials.HTTPCredential) (Runner, func(), error) {
	return f.runner, func() { f.close.Add(1) }, nil
}

type fakeRunner struct {
	active, max atomic.Int32
	block       chan struct{}
	errs        map[string]error
	mu          sync.Mutex
	seen        []string
}

func (f *fakeRunner) enter(ctx context.Context, q string) (prometheus.Result, error) {
	n := f.active.Add(1)
	for {
		old := f.max.Load()
		if n <= old || f.max.CompareAndSwap(old, n) {
			break
		}
	}
	defer f.active.Add(-1)
	f.mu.Lock()
	f.seen = append(f.seen, q)
	f.mu.Unlock()
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return prometheus.Result{}, ctx.Err()
		}
	}
	return prometheus.Result{ResultType: "vector"}, f.errs[q]
}
func (f *fakeRunner) Query(ctx context.Context, q string, _ time.Time) (prometheus.Result, error) {
	return f.enter(ctx, q)
}
func (f *fakeRunner) QueryRange(ctx context.Context, q string, _, _ time.Time, _ time.Duration) (prometheus.Result, error) {
	return f.enter(ctx, q)
}
func (f *fakeRunner) Labels(context.Context) ([]string, error) { return []string{"job"}, nil }
func (f *fakeRunner) LabelValues(context.Context, string) ([]string, error) {
	return []string{"api"}, nil
}

func defaultLimits() Limits {
	return Limits{MaxBatch: 12, MaxConcurrent: 4, MaxPromQLBytes: 8 << 10, MaxRange: 7 * 24 * time.Hour, MinStep: 15 * time.Second, MaxPoints: 11000, Timeout: 15 * time.Second, MaxEnrichmentIPs: 100}
}
func queryService(r Runner) (*Service, *fakeLoader, *fakeConsumer, *fakeClientFactory) {
	l := &fakeLoader{cfg: Datasource{ID: 1, BaseURL: "https://prom.example", AuthType: "bearer", CredentialID: u64(7)}}
	c := &fakeConsumer{secret: &credentials.HTTPCredential{AuthType: "bearer", Secret: []byte("secret")}}
	f := &fakeClientFactory{runner: r}
	return NewService(l, c, f, nil, defaultLimits()), l, c, f
}
func u64(v uint64) *uint64 { return &v }
func bizCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, code, be.Code)
}

func TestBatchValidationLimits(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		req  RangeRequest
	}{
		{"empty ref", RangeRequest{DatasourceID: 1, Start: now.Add(-time.Hour), End: now, Step: time.Minute, Queries: []Query{{PromQL: "up"}}}},
		{"duplicate ref", RangeRequest{DatasourceID: 1, Start: now.Add(-time.Hour), End: now, Step: time.Minute, Queries: []Query{{RefID: "a", PromQL: "up"}, {RefID: "a", PromQL: "up"}}}},
		{"thirteen", RangeRequest{DatasourceID: 1, Start: now.Add(-time.Hour), End: now, Step: time.Minute, Queries: makeQueries(13)}},
		{"promql", RangeRequest{DatasourceID: 1, Start: now.Add(-time.Hour), End: now, Step: time.Minute, Queries: []Query{{RefID: "a", PromQL: string(make([]byte, (8<<10)+1))}}}},
		{"range", RangeRequest{DatasourceID: 1, Start: now.Add(-8 * 24 * time.Hour), End: now, Step: time.Minute, Queries: makeQueries(1)}},
		{"step", RangeRequest{DatasourceID: 1, Start: now.Add(-time.Hour), End: now, Step: time.Second, Queries: makeQueries(1)}},
		{"points", RangeRequest{DatasourceID: 1, Start: now.Add(-7 * 24 * time.Hour), End: now, Step: 15 * time.Second, Queries: makeQueries(1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _, _ := queryService(&fakeRunner{})
			_, err := s.Range(context.Background(), 1, tc.req)
			require.Error(t, err)
		})
	}
}

func TestRangeStepBelowMinimumIsInvalidRequest(t *testing.T) {
	s, _, _, _ := queryService(&fakeRunner{})
	now := time.Now()
	_, err := s.Range(context.Background(), 1, RangeRequest{DatasourceID: 1, Start: now.Add(-time.Hour), End: now, Step: time.Second, Queries: makeQueries(1)})
	bizCode(t, err, apperr.CodeObservabilityQueryInvalidRequest)
}
func makeQueries(n int) []Query {
	q := make([]Query, n)
	for i := range q {
		q[i] = Query{RefID: string(rune('a' + i)), PromQL: "up"}
	}
	return q
}

func TestRangeBoundedOrderedConsumesOnceAndWipes(t *testing.T) {
	r := &fakeRunner{block: make(chan struct{})}
	s, l, c, f := queryService(r)
	now := time.Now()
	done := make(chan *BatchResult)
	go func() {
		got, _ := s.Range(context.Background(), 42, RangeRequest{DatasourceID: 1, Start: now.Add(-time.Hour), End: now, Step: time.Minute, Queries: makeQueries(8)})
		done <- got
	}()
	require.Eventually(t, func() bool { return r.max.Load() == 4 }, time.Second, time.Millisecond)
	close(r.block)
	got := <-done
	require.Equal(t, int32(4), r.max.Load())
	require.Equal(t, int32(1), l.calls.Load())
	require.Equal(t, int32(1), c.calls.Load())
	require.Equal(t, "observability.query.range", c.purpose)
	require.Equal(t, int32(1), f.close.Load())
	require.Len(t, got.Results, 8)
	for i, x := range got.Results {
		require.Equal(t, string(rune('a'+i)), x.RefID)
	}
	require.Nil(t, c.secret.Secret)
}

func TestExpressionErrorIsItemButDestinationFailsBatch(t *testing.T) {
	rejected := fakeExpressionError{apperr.New(apperr.CodeObservabilityQueryUpstreamRejected, "observability.query.upstream_rejected", "rejected")}
	r := &fakeRunner{errs: map[string]error{"bad": rejected}}
	s, _, _, _ := queryService(r)
	got, err := s.Instant(context.Background(), 1, InstantRequest{DatasourceID: 1, Queries: []Query{{RefID: "a", PromQL: "up"}, {RefID: "b", PromQL: "bad"}}})
	require.NoError(t, err)
	require.Nil(t, got.Results[1].Result)
	require.Equal(t, int(apperr.CodeObservabilityQueryUpstreamRejected), got.Results[1].Error.Code)
	r.errs = map[string]error{"up": apperr.New(apperr.CodeObservabilityQueryDestinationDenied, "observability.query.destination_denied", "denied")}
	_, err = s.Instant(context.Background(), 1, InstantRequest{DatasourceID: 1, Queries: makeQueries(1)})
	bizCode(t, err, apperr.CodeObservabilityQueryDestinationDenied)

	r.errs = map[string]error{"up": apperr.New(apperr.CodeObservabilityQueryUpstreamRejected, "observability.query.upstream_rejected", "auth rejected")}
	_, err = s.Instant(context.Background(), 1, InstantRequest{DatasourceID: 1, Queries: makeQueries(1)})
	bizCode(t, err, apperr.CodeObservabilityQueryUpstreamRejected)
}

func TestHTTPExpressionErrorDoesNotCancelSibling(t *testing.T) {
	r := &fakeRunner{errs: map[string]error{"bad": fakeExpressionError{apperr.New(apperr.CodeObservabilityQueryUpstreamRejected, "observability.query.upstream_rejected", "bad query")}}}
	s, _, _, _ := queryService(r)
	got, err := s.Instant(context.Background(), 1, InstantRequest{DatasourceID: 1, Queries: []Query{{RefID: "bad", PromQL: "bad"}, {RefID: "good", PromQL: "up"}}})
	require.NoError(t, err)
	require.NotNil(t, got.Results[0].Error)
	require.NotNil(t, got.Results[1].Result)
}

type fakeExpressionError struct{ error }

func (e fakeExpressionError) Unwrap() error                   { return e.error }
func (e fakeExpressionError) PrometheusExpressionError() bool { return true }

func TestCancellationStopsBatch(t *testing.T) {
	r := &fakeRunner{block: make(chan struct{})}
	s, _, _, _ := queryService(r)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		_, err := s.Instant(ctx, 1, InstantRequest{DatasourceID: 1, Queries: makeQueries(8)})
		done <- err
	}()
	require.Eventually(t, func() bool { return r.active.Load() == 4 }, time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}

func TestMetadataUsesOneCredentialAndPurpose(t *testing.T) {
	r := &fakeRunner{}
	s, _, c, f := queryService(r)
	labels, err := s.Labels(context.Background(), 9, 1)
	require.NoError(t, err)
	require.Equal(t, []string{"job"}, labels)
	require.Equal(t, "observability.query.metadata", c.purpose)
	require.Equal(t, int32(1), f.close.Load())
	vals, err := s.LabelValues(context.Background(), 9, 1, "job")
	require.NoError(t, err)
	require.Equal(t, []string{"api"}, vals)
	before := c.calls.Load()
	_, err = s.LabelValues(context.Background(), 9, 1, "bad/name")
	bizCode(t, err, apperr.CodeObservabilityQueryInvalidRequest)
	require.Equal(t, before, c.calls.Load())
}
