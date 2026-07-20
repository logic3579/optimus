package datasource

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/observability/prometheus"
)

type HTTPMetadata struct {
	ID             uint64
	Name, AuthType string
}
type CredentialMetadata interface {
	GetHTTPMetadata(context.Context, uint64) (HTTPMetadata, error)
}
type ClusterExistence interface {
	Exists(context.Context, uint64) (bool, error)
}
type PanelUsage interface {
	CountByDatasourceIDTx(context.Context, *gorm.DB, uint64) (int64, error)
}
type Tester interface {
	Test(context.Context, Detail, *credentials.HTTPCredential) (map[string]string, error)
}
type CredentialConsumer interface {
	GetHTTPCredential(context.Context, uint64, string) (*credentials.HTTPCredential, error)
}
type auditWriter interface {
	Record(context.Context, audit.Event) error
}
type repository interface {
	List(context.Context, ListQuery) ([]Detail, int64, error)
	GetByID(context.Context, uint64) (*Detail, error)
	Transaction(context.Context, func(*gorm.DB) error) error
	FindNameAliveTx(context.Context, *gorm.DB, string, uint64) (bool, error)
	GetModelForUpdate(context.Context, *gorm.DB, uint64) (*models.ObservabilityDatasource, error)
	CreateTx(context.Context, *gorm.DB, *models.ObservabilityDatasource) error
	UpdateTx(context.Context, *gorm.DB, uint64, map[string]any) (int64, error)
	SoftDeleteTx(context.Context, *gorm.DB, uint64) (int64, error)
}
type Service struct {
	repo     repository
	metadata CredentialMetadata
	clusters ClusterExistence
	panels   PanelUsage
	consumer CredentialConsumer
	tester   Tester
	audit    auditWriter
	auditTx  func(*gorm.DB) auditWriter
}

func NewService(r *Repo, m CredentialMetadata, c ClusterExistence, p PanelUsage, consumer CredentialConsumer, tester Tester, a *audit.Recorder) *Service {
	return &Service{repo: r, metadata: m, clusters: c, panels: p, consumer: consumer, tester: tester, audit: a, auditTx: func(tx *gorm.DB) auditWriter { return a.WithTx(tx) }}
}
func newServiceForTest(r repository, m CredentialMetadata, c ClusterExistence, p PanelUsage, consumer CredentialConsumer, tester Tester, a auditWriter) *Service {
	return &Service{repo: r, metadata: m, clusters: c, panels: p, consumer: consumer, tester: tester, audit: a, auditTx: func(*gorm.DB) auditWriter { return a }}
}

func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	items, n, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	page, size, _, err := pageValues(q.Page, q.PageSize)
	if err != nil {
		return nil, err
	}
	return &ListResponse{items, n, page, size}, nil
}
func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	return s.repo.GetByID(ctx, id)
}
func (s *Service) Create(ctx context.Context, actor uint64, ip, ua string, req CreateRequest) (*Detail, error) {
	row, err := s.validateCreate(ctx, actor, req)
	if err != nil {
		return nil, err
	}
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		conflict, err := s.repo.FindNameAliveTx(ctx, tx, row.Name, 0)
		if err != nil {
			return err
		}
		if conflict {
			return nameTaken()
		}
		if err = s.repo.CreateTx(ctx, tx, row); err != nil {
			return err
		}
		return s.record(s.auditTx(tx), ctx, actor, "observability.datasource.create", row.ID, ip, ua, map[string]any{"name": row.Name, "base_url": row.BaseURL, "auth_type": row.AuthType, "http_credential_id": row.HTTPCredentialID, "cluster_id": row.ClusterID, "tls_skip_verify": row.TLSSkipVerify, "has_custom_ca": row.CustomCAPEM != nil})
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, row.ID)
}
func (s *Service) validateCreate(ctx context.Context, actor uint64, req CreateRequest) (*models.ObservabilityDatasource, error) {
	name := strings.TrimSpace(req.Name)
	base := strings.TrimSpace(req.BaseURL)
	if name == "" || len(name) > 128 {
		return nil, validation("invalid name")
	}
	if _, err := prometheus.ParseBaseURL(base); err != nil {
		return nil, err
	}
	if err := validateCA(req.CustomCAPEM); err != nil {
		return nil, err
	}
	if err := s.validateRefs(ctx, req.AuthType, req.HTTPCredentialID, req.ClusterID); err != nil {
		return nil, err
	}
	uid := actorPtr(actor)
	return &models.ObservabilityDatasource{Name: name, BaseURL: base, AuthType: req.AuthType, HTTPCredentialID: req.HTTPCredentialID, ClusterID: req.ClusterID, TLSSkipVerify: req.TLSSkipVerify, CustomCAPEM: cleanCA(req.CustomCAPEM), Description: strings.TrimSpace(req.Description), CreatedByUserID: uid}, nil
}
func (s *Service) validateRefs(ctx context.Context, auth string, credentialID, clusterID *uint64) error {
	if auth != "none" && auth != "basic" && auth != "bearer" {
		return validation("invalid auth type")
	}
	if auth == "none" && credentialID != nil {
		return authMismatch()
	}
	if auth != "none" && credentialID == nil {
		return authMismatch()
	}
	if credentialID != nil {
		if s.metadata == nil {
			return authMismatch()
		}
		m, err := s.metadata.GetHTTPMetadata(ctx, *credentialID)
		if err != nil {
			return authMismatch()
		}
		if m.AuthType != auth {
			return authMismatch()
		}
	}
	if clusterID != nil {
		if s.clusters == nil {
			return validation("cluster not found")
		}
		ok, err := s.clusters.Exists(ctx, *clusterID)
		if err != nil {
			return err
		}
		if !ok {
			return validation("cluster not found")
		}
	}
	return nil
}
func (s *Service) Update(ctx context.Context, actor uint64, ip, ua string, id uint64, req UpdateRequest) (*Detail, error) {
	changed := []string{}
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		row, err := s.repo.GetModelForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		fields := map[string]any{}
		name := row.Name
		if req.Name != nil {
			name = strings.TrimSpace(*req.Name)
			if name == "" || len(name) > 128 {
				return validation("invalid name")
			}
			if name != row.Name {
				fields["name"] = name
				changed = append(changed, "name")
			}
		}
		base := row.BaseURL
		if req.BaseURL != nil {
			base = strings.TrimSpace(*req.BaseURL)
			if _, err := prometheus.ParseBaseURL(base); err != nil {
				return err
			}
			if base != row.BaseURL {
				fields["base_url"] = base
				changed = append(changed, "base_url")
			}
		}
		auth := row.AuthType
		if req.AuthType != nil {
			auth = *req.AuthType
		}
		cred := row.HTTPCredentialID
		if req.ClearHTTPCredential {
			cred = nil
		} else if req.HTTPCredentialID != nil {
			cred = req.HTTPCredentialID
		}
		cluster := row.ClusterID
		if req.ClearCluster {
			cluster = nil
		} else if req.ClusterID != nil {
			cluster = req.ClusterID
		}
		if err := s.validateRefs(ctx, auth, cred, cluster); err != nil {
			return err
		}
		if auth != row.AuthType {
			fields["auth_type"] = auth
			changed = append(changed, "auth_type")
		}
		if !equalPtr(cred, row.HTTPCredentialID) {
			fields["http_credential_id"] = cred
			changed = append(changed, "http_credential_id")
		}
		if !equalPtr(cluster, row.ClusterID) {
			fields["cluster_id"] = cluster
			changed = append(changed, "cluster_id")
		}
		ca := row.CustomCAPEM
		if req.ClearCustomCA {
			ca = nil
		} else if req.CustomCAPEM != nil {
			ca = cleanCA(req.CustomCAPEM)
		}
		if err := validateCA(ca); err != nil {
			return err
		}
		if !equalStringPtr(ca, row.CustomCAPEM) {
			fields["custom_ca_pem"] = ca
			changed = append(changed, "custom_ca")
		}
		if req.TLSSkipVerify != nil && *req.TLSSkipVerify != row.TLSSkipVerify {
			fields["tls_skip_verify"] = *req.TLSSkipVerify
			changed = append(changed, "tls_skip_verify")
		}
		if req.Description != nil && strings.TrimSpace(*req.Description) != row.Description {
			fields["description"] = strings.TrimSpace(*req.Description)
			changed = append(changed, "description")
		}
		if len(fields) == 0 {
			return nil
		}
		if name != row.Name {
			conflict, err := s.repo.FindNameAliveTx(ctx, tx, name, id)
			if err != nil {
				return err
			}
			if conflict {
				return nameTaken()
			}
		}
		n, err := s.repo.UpdateTx(ctx, tx, id, fields)
		if err != nil {
			return err
		}
		if n != 1 {
			return notFound()
		}
		sort.Strings(changed)
		return s.record(s.auditTx(tx), ctx, actor, "observability.datasource.update", id, ip, ua, map[string]any{"changed_fields": changed})
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, id)
}
func (s *Service) Delete(ctx context.Context, actor uint64, ip, ua string, id uint64) error {
	return s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		row, err := s.repo.GetModelForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		n, err := s.panels.CountByDatasourceIDTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if n > 0 {
			return apperr.New(apperr.CodeObservabilityDatasourceInUse, "observability.datasource.in_use", "data source is in use")
		}
		affected, err := s.repo.SoftDeleteTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if affected != 1 {
			return notFound()
		}
		return s.record(s.auditTx(tx), ctx, actor, "observability.datasource.delete", id, ip, ua, map[string]any{"name": row.Name})
	})
}
func (s *Service) TestConnection(ctx context.Context, actor uint64, ip, ua string, id uint64) (*TestResponse, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	var secret *credentials.HTTPCredential
	if d.AuthType != "none" {
		if s.consumer == nil || d.HTTPCredential == nil {
			err = authMismatch()
			s.recordTestAudit(ctx, actor, ip, ua, id, err)
			return nil, err
		}
		secret, err = s.consumer.GetHTTPCredential(ctx, d.HTTPCredential.ID, "observability.datasource.test")
		if err != nil {
			s.recordTestAudit(ctx, actor, ip, ua, id, err)
			return nil, err
		}
		defer credentials.WipeHTTPCredential(secret)
	}
	info, testErr := s.tester.Test(ctx, *d, secret)
	auditErr := s.recordTestAudit(ctx, actor, ip, ua, id, testErr)
	if testErr != nil {
		return nil, testErr
	}
	if auditErr != nil {
		return nil, auditErr
	}
	return &TestResponse{Reachable: true, Version: info["version"]}, nil
}
func (s *Service) recordTestAudit(ctx context.Context, actor uint64, ip, ua string, id uint64, testErr error) error {
	payload := map[string]any{"reachable": testErr == nil}
	if be, ok := apperr.AsBiz(testErr); ok {
		payload["error_code"] = be.Code
	}
	return s.record(s.audit, ctx, actor, "observability.datasource.test", id, ip, ua, payload)
}
func (s *Service) record(w auditWriter, ctx context.Context, actor uint64, action string, id uint64, ip, ua string, payload any) error {
	if w == nil {
		return nil
	}
	return w.Record(ctx, audit.Event{UserID: actorPtr(actor), Action: action, TargetType: "observability_datasource", TargetID: strconv.FormatUint(id, 10), Payload: payload, IP: ip, UserAgent: ua})
}
func validateCA(raw *string) error {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	rest := []byte(*raw)
	count := 0
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" {
			return invalidTLS()
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !cert.IsCA {
			return invalidTLS()
		}
		count++
		rest = []byte(strings.TrimSpace(string(next)))
	}
	if count == 0 {
		return invalidTLS()
	}
	return nil
}
func cleanCA(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}
func actorPtr(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}
func equalPtr(a, b *uint64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func equalStringPtr(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
func validation(msg string) error { return apperr.New(apperr.CodeValidation, "common.validation", msg) }
func authMismatch() error {
	return apperr.New(apperr.CodeObservabilityDatasourceAuthMismatch, "observability.datasource.auth_mismatch", "data source authentication mismatch")
}
func invalidTLS() error {
	return apperr.New(apperr.CodeObservabilityDatasourceInvalidTLS, "observability.datasource.invalid_tls", "invalid data source TLS configuration")
}
