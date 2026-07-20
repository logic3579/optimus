package datasource

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
)

type fakeRepo struct {
	row      *models.ObservabilityDatasource
	conflict bool
	panels   int64
}

func (f *fakeRepo) List(context.Context, ListQuery) ([]Detail, int64, error) { return nil, 0, nil }
func (f *fakeRepo) GetByID(context.Context, uint64) (*Detail, error) {
	if f.row == nil {
		return nil, notFound()
	}
	return detailFromModel(f.row, "cred", "cluster"), nil
}
func (f *fakeRepo) Transaction(ctx context.Context, fn func(*gorm.DB) error) error { return fn(nil) }
func (f *fakeRepo) FindNameAliveTx(context.Context, *gorm.DB, string, uint64) (bool, error) {
	return f.conflict, nil
}
func (f *fakeRepo) GetModelForUpdate(context.Context, *gorm.DB, uint64) (*models.ObservabilityDatasource, error) {
	if f.row == nil {
		return nil, notFound()
	}
	return f.row, nil
}
func (f *fakeRepo) CreateTx(_ context.Context, _ *gorm.DB, m *models.ObservabilityDatasource) error {
	m.ID = 7
	f.row = m
	return nil
}
func (f *fakeRepo) UpdateTx(_ context.Context, _ *gorm.DB, _ uint64, fields map[string]any) (int64, error) {
	return 1, nil
}
func (f *fakeRepo) SoftDeleteTx(context.Context, *gorm.DB, uint64) (int64, error) { return 1, nil }

type fakeMeta struct {
	meta HTTPMetadata
	err  error
}

func (f fakeMeta) GetHTTPMetadata(context.Context, uint64) (HTTPMetadata, error) {
	return f.meta, f.err
}

type fakeCluster struct{ ok bool }

func (f fakeCluster) Exists(context.Context, uint64) (bool, error) { return f.ok, nil }

type fakePanels struct{ n int64 }

func (f fakePanels) CountByDatasourceID(context.Context, uint64) (int64, error) { return f.n, nil }

type fakeConsumer struct {
	calls      int
	purpose    string
	credential *credentials.HTTPCredential
}

func (f *fakeConsumer) GetHTTPCredential(_ context.Context, _ uint64, p string) (*credentials.HTTPCredential, error) {
	f.calls++
	f.purpose = p
	return f.credential, nil
}

type fakeTester struct{ err error }

func (f fakeTester) Test(context.Context, Detail, *credentials.HTTPCredential) (map[string]string, error) {
	return map[string]string{"version": "2.50"}, f.err
}

type fakeAudit struct{ events []audit.Event }

func (f *fakeAudit) Record(_ context.Context, e audit.Event) error {
	f.events = append(f.events, e)
	return nil
}

func newTestService(r *fakeRepo, m fakeMeta, c fakeCluster, p fakePanels, consumer *fakeConsumer, tester fakeTester, a *fakeAudit) *Service {
	return newServiceForTest(r, m, c, p, consumer, tester, a)
}
func code(t *testing.T, err error, want apperr.Code) {
	t.Helper()
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, want, be.Code)
}

func TestServiceRejectsCredentialAuthMismatch(t *testing.T) {
	s := newTestService(&fakeRepo{}, fakeMeta{meta: HTTPMetadata{ID: 9, AuthType: "bearer"}}, fakeCluster{ok: true}, fakePanels{}, &fakeConsumer{}, fakeTester{}, &fakeAudit{})
	_, err := s.Create(context.Background(), 1, "ip", "ua", CreateRequest{Name: "prom", BaseURL: "https://prom.example.com", AuthType: "basic", HTTPCredentialID: ptr(uint64(9))})
	code(t, err, apperr.CodeObservabilityDatasourceAuthMismatch)
}

func TestServiceValidatesURLCAClusterAndUniqueness(t *testing.T) {
	cases := []struct {
		name    string
		repo    *fakeRepo
		cluster fakeCluster
		req     CreateRequest
		want    apperr.Code
	}{
		{"url", &fakeRepo{}, fakeCluster{true}, CreateRequest{Name: "x", BaseURL: "https://u:p@example.com", AuthType: "none"}, apperr.CodeObservabilityDatasourceInvalidURL},
		{"ca", &fakeRepo{}, fakeCluster{true}, CreateRequest{Name: "x", BaseURL: "https://example.com", AuthType: "none", CustomCAPEM: ptr("-----BEGIN PRIVATE KEY-----\nAA==\n-----END PRIVATE KEY-----")}, apperr.CodeObservabilityDatasourceInvalidTLS},
		{"cluster", &fakeRepo{}, fakeCluster{false}, CreateRequest{Name: "x", BaseURL: "https://example.com", AuthType: "none", ClusterID: ptr(uint64(3))}, apperr.CodeValidation},
		{"name", &fakeRepo{conflict: true}, fakeCluster{true}, CreateRequest{Name: "x", BaseURL: "https://example.com", AuthType: "none"}, apperr.CodeObservabilityDatasourceNameTaken},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestService(tt.repo, fakeMeta{}, tt.cluster, fakePanels{}, &fakeConsumer{}, fakeTester{}, &fakeAudit{})
			_, err := s.Create(context.Background(), 1, "", "", tt.req)
			code(t, err, tt.want)
		})
	}
}

func TestServiceCreateAuditsWithoutCA(t *testing.T) {
	a := &fakeAudit{}
	r := &fakeRepo{}
	s := newTestService(r, fakeMeta{}, fakeCluster{true}, fakePanels{}, &fakeConsumer{}, fakeTester{}, a)
	d, err := s.Create(context.Background(), 1, "ip", "ua", CreateRequest{Name: " prom ", BaseURL: " https://example.com/prom ", AuthType: "none", TLSSkipVerify: true, CustomCAPEM: nil})
	require.NoError(t, err)
	require.Equal(t, "prom", d.Name)
	require.True(t, d.TLSSkipVerify)
	require.Len(t, a.events, 1)
	require.NotContains(t, a.events[0].Payload, "custom_ca_pem")
}

func TestServiceDeleteRejectsPanelUsage(t *testing.T) {
	s := newTestService(&fakeRepo{row: &models.ObservabilityDatasource{ID: 1}}, fakeMeta{}, fakeCluster{true}, fakePanels{1}, &fakeConsumer{}, fakeTester{}, &fakeAudit{})
	code(t, s.Delete(context.Background(), 1, "", "", 1), apperr.CodeObservabilityDatasourceInUse)
}

func TestServiceTestConnectionAuthAndAuditRedaction(t *testing.T) {
	r := &fakeRepo{row: &models.ObservabilityDatasource{ID: 1, Name: "p", BaseURL: "https://example.com", AuthType: "bearer", HTTPCredentialID: ptr(uint64(9))}}
	c := &fakeConsumer{credential: &credentials.HTTPCredential{Secret: []byte("topsecret")}}
	a := &fakeAudit{}
	s := newTestService(r, fakeMeta{}, fakeCluster{true}, fakePanels{}, c, fakeTester{err: errors.New("raw upstream topsecret")}, a)
	_, err := s.TestConnection(context.Background(), 1, "ip", "ua", 1)
	require.Error(t, err)
	require.Equal(t, 1, c.calls)
	require.Equal(t, "observability.datasource.test", c.purpose)
	require.Len(t, a.events, 1)
	require.NotContains(t, a.events[0].Payload, "topsecret")
	require.NotContains(t, a.events[0].Payload, "raw upstream")
}

func TestServiceTestConnectionNoAuthSkipsConsumer(t *testing.T) {
	r := &fakeRepo{row: &models.ObservabilityDatasource{ID: 1, BaseURL: "https://example.com", AuthType: "none"}}
	c := &fakeConsumer{}
	s := newTestService(r, fakeMeta{}, fakeCluster{true}, fakePanels{}, c, fakeTester{}, &fakeAudit{})
	_, err := s.TestConnection(context.Background(), 1, "", "", 1)
	require.NoError(t, err)
	require.Zero(t, c.calls)
}

func ptr[T any](v T) *T { return &v }
