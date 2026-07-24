package datasource

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
)

func testCAPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "p5-audit-redaction-test"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestDetailCustomCAPEMReturnsCopyAndStaysOutOfJSON(t *testing.T) {
	d := Detail{customCAPEM: "secret-ca"}
	first := d.CustomCAPEMCopy()
	require.Equal(t, []byte("secret-ca"), first)
	first[0] = 'X'
	require.Equal(t, []byte("secret-ca"), d.CustomCAPEMCopy())
	raw, err := json.Marshal(d)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret-ca")
}

type fakeRepo struct {
	row      *models.ObservabilityDatasource
	conflict bool
	tx       *gorm.DB
}

func (f *fakeRepo) List(context.Context, ListQuery) ([]Detail, int64, error) { return nil, 0, nil }
func (f *fakeRepo) GetByID(context.Context, uint64) (*Detail, error) {
	if f.row == nil {
		return nil, notFound()
	}
	return detailFromModel(f.row, "cred", "cluster"), nil
}
func (f *fakeRepo) Transaction(_ context.Context, fn func(*gorm.DB) error) error {
	if f.tx == nil {
		f.tx = &gorm.DB{}
	}
	return fn(f.tx)
}
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
func (f *fakeRepo) UpdateTx(_ context.Context, _ *gorm.DB, _ uint64, _ map[string]any) (int64, error) {
	return 1, nil
}
func (f *fakeRepo) SoftDeleteTx(context.Context, *gorm.DB, uint64) (int64, error) { return 1, nil }

type fakeMeta struct {
	meta  HTTPMetadata
	err   error
	gotTx *gorm.DB
}

func (f *fakeMeta) GetHTTPMetadataTx(_ context.Context, tx *gorm.DB, _ uint64) (HTTPMetadata, error) {
	f.gotTx = tx
	return f.meta, f.err
}

type fakeCluster struct {
	ok    bool
	gotTx *gorm.DB
}

func (f *fakeCluster) ExistsTx(_ context.Context, tx *gorm.DB, _ uint64) (bool, error) {
	f.gotTx = tx
	return f.ok, nil
}

type fakePanels struct {
	n     int64
	gotTx *gorm.DB
}

func (f *fakePanels) CountByDatasourceIDTx(_ context.Context, tx *gorm.DB, _ uint64) (int64, error) {
	f.gotTx = tx
	return f.n, nil
}

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

func newTestService(r *fakeRepo, m *fakeMeta, c *fakeCluster, p *fakePanels, consumer *fakeConsumer, tester fakeTester, a *fakeAudit) *Service {
	return newServiceForTest(r, m, c, p, consumer, tester, a)
}
func code(t *testing.T, err error, want apperr.Code) {
	t.Helper()
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, want, be.Code)
}

func TestServiceRejectsCredentialAuthMismatch(t *testing.T) {
	r := &fakeRepo{}
	meta := &fakeMeta{meta: HTTPMetadata{ID: 9, AuthType: "bearer"}}
	s := newTestService(r, meta, &fakeCluster{ok: true}, &fakePanels{}, &fakeConsumer{}, fakeTester{}, &fakeAudit{})
	_, err := s.Create(context.Background(), 1, "ip", "ua", CreateRequest{Name: "prom", BaseURL: "https://prom.example.com", AuthType: "basic", HTTPCredentialID: ptr(uint64(9))})
	code(t, err, apperr.CodeObservabilityDatasourceAuthMismatch)
}

func TestServiceCreateValidatesReferencesInMutationTransaction(t *testing.T) {
	r := &fakeRepo{}
	meta := &fakeMeta{meta: HTTPMetadata{ID: 9, AuthType: "basic"}}
	cluster := &fakeCluster{ok: true}
	s := newTestService(r, meta, cluster, &fakePanels{}, &fakeConsumer{}, fakeTester{}, &fakeAudit{})
	_, err := s.Create(context.Background(), 1, "", "", CreateRequest{Name: "prom", BaseURL: "https://prom.example.com", AuthType: "basic", HTTPCredentialID: ptr(uint64(9)), ClusterID: ptr(uint64(4))})
	require.NoError(t, err)
	require.Same(t, r.tx, meta.gotTx)
	require.Same(t, r.tx, cluster.gotTx)
}

func TestServiceUpdateValidatesReferencesInMutationTransaction(t *testing.T) {
	r := &fakeRepo{row: &models.ObservabilityDatasource{ID: 7, Name: "prom", BaseURL: "https://prom.example.com", AuthType: "none"}}
	meta := &fakeMeta{meta: HTTPMetadata{ID: 9, AuthType: "bearer"}}
	cluster := &fakeCluster{ok: true}
	s := newTestService(r, meta, cluster, &fakePanels{}, &fakeConsumer{}, fakeTester{}, &fakeAudit{})
	auth := "bearer"
	_, err := s.Update(context.Background(), 1, "", "", 7, UpdateRequest{AuthType: &auth, HTTPCredentialID: ptr(uint64(9)), ClusterID: ptr(uint64(4))})
	require.NoError(t, err)
	require.Same(t, r.tx, meta.gotTx)
	require.Same(t, r.tx, cluster.gotTx)
}

func TestServicePropagatesCredentialMetadataInfrastructureErrors(t *testing.T) {
	infraErr := errors.New("metadata database unavailable")
	credentialID := ptr(uint64(9))
	for _, tc := range []struct {
		name string
		run  func(*Service) error
	}{
		{"create", func(s *Service) error {
			_, err := s.Create(context.Background(), 1, "", "", CreateRequest{Name: "prom", BaseURL: "https://prom.example.com", AuthType: "bearer", HTTPCredentialID: credentialID})
			return err
		}},
		{"update", func(s *Service) error {
			auth := "bearer"
			_, err := s.Update(context.Background(), 1, "", "", 7, UpdateRequest{AuthType: &auth, HTTPCredentialID: credentialID})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeRepo{row: &models.ObservabilityDatasource{ID: 7, Name: "prom", BaseURL: "https://prom.example.com", AuthType: "none"}}
			s := newTestService(r, &fakeMeta{err: infraErr}, &fakeCluster{ok: true}, &fakePanels{}, &fakeConsumer{}, fakeTester{}, &fakeAudit{})
			err := tc.run(s)
			require.ErrorIs(t, err, infraErr)
			if bizErr, ok := apperr.AsBiz(err); ok {
				require.NotEqual(t, apperr.CodeObservabilityDatasourceAuthMismatch, bizErr.Code)
			}
		})
	}
}

func TestServiceValidatesURLCAClusterAndUniqueness(t *testing.T) {
	cases := []struct {
		name    string
		repo    *fakeRepo
		cluster *fakeCluster
		req     CreateRequest
		want    apperr.Code
	}{
		{"url", &fakeRepo{}, &fakeCluster{ok: true}, CreateRequest{Name: "x", BaseURL: "https://u:p@example.com", AuthType: "none"}, apperr.CodeObservabilityDatasourceInvalidURL},
		{"ca", &fakeRepo{}, &fakeCluster{ok: true}, CreateRequest{Name: "x", BaseURL: "https://example.com", AuthType: "none", CustomCAPEM: ptr("-----BEGIN PRIVATE KEY-----\nAA==\n-----END PRIVATE KEY-----")}, apperr.CodeObservabilityDatasourceInvalidTLS},
		{"cluster", &fakeRepo{}, &fakeCluster{ok: false}, CreateRequest{Name: "x", BaseURL: "https://example.com", AuthType: "none", ClusterID: ptr(uint64(3))}, apperr.CodeValidation},
		{"name", &fakeRepo{conflict: true}, &fakeCluster{ok: true}, CreateRequest{Name: "x", BaseURL: "https://example.com", AuthType: "none"}, apperr.CodeObservabilityDatasourceNameTaken},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestService(tt.repo, &fakeMeta{}, tt.cluster, &fakePanels{}, &fakeConsumer{}, fakeTester{}, &fakeAudit{})
			_, err := s.Create(context.Background(), 1, "", "", tt.req)
			code(t, err, tt.want)
		})
	}
}

func TestServiceCreateAuditRedactsCustomCA(t *testing.T) {
	a := &fakeAudit{}
	r := &fakeRepo{}
	s := newTestService(r, &fakeMeta{}, &fakeCluster{ok: true}, &fakePanels{}, &fakeConsumer{}, fakeTester{}, a)
	ca := testCAPEM(t)
	d, err := s.Create(context.Background(), 1, "ip", "ua", CreateRequest{Name: " prom ", BaseURL: " https://example.com/prom ", AuthType: "none", TLSSkipVerify: true, CustomCAPEM: &ca})
	require.NoError(t, err)
	require.Equal(t, "prom", d.Name)
	require.True(t, d.TLSSkipVerify)
	require.True(t, d.HasCustomCA)
	require.Len(t, a.events, 1)
	require.NotContains(t, a.events[0].Payload, "custom_ca_pem")
	payload, err := json.Marshal(a.events[0].Payload)
	require.NoError(t, err)
	require.NotContains(t, string(payload), ca)
}

func TestServiceDeleteRejectsPanelUsage(t *testing.T) {
	r := &fakeRepo{row: &models.ObservabilityDatasource{ID: 1}}
	panels := &fakePanels{n: 1}
	s := newTestService(r, &fakeMeta{}, &fakeCluster{ok: true}, panels, &fakeConsumer{}, fakeTester{}, &fakeAudit{})
	code(t, s.Delete(context.Background(), 1, "", "", 1), apperr.CodeObservabilityDatasourceInUse)
	require.Same(t, r.tx, panels.gotTx)
}

func TestServiceTestConnectionAuthAndAuditRedaction(t *testing.T) {
	r := &fakeRepo{row: &models.ObservabilityDatasource{ID: 1, Name: "p", BaseURL: "https://example.com", AuthType: "bearer", HTTPCredentialID: ptr(uint64(9))}}
	c := &fakeConsumer{credential: &credentials.HTTPCredential{Secret: []byte("topsecret")}}
	a := &fakeAudit{}
	s := newTestService(r, &fakeMeta{}, &fakeCluster{ok: true}, &fakePanels{}, c, fakeTester{err: errors.New("raw upstream topsecret")}, a)
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
	s := newTestService(r, &fakeMeta{}, &fakeCluster{ok: true}, &fakePanels{}, c, fakeTester{}, &fakeAudit{})
	_, err := s.TestConnection(context.Background(), 1, "", "", 1)
	require.NoError(t, err)
	require.Zero(t, c.calls)
}

func ptr[T any](v T) *T { return &v }
