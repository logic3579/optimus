package httpcredential

import (
	"context"
	"errors"
	"gorm.io/gorm"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"strconv"
	"strings"
)

type Cipher interface {
	Seal([]byte) ([]byte, error)
	Open([]byte) ([]byte, error)
}
type InUseCounter interface {
	CountByHTTPCredentialID(context.Context, uint64) (int64, error)
}
type Service struct {
	repo    *Repo
	cipher  Cipher
	audit   *audit.Recorder
	counter InUseCounter
}

func NewService(r *Repo, c Cipher, a *audit.Recorder) *Service {
	return &Service{repo: r, cipher: c, audit: a}
}
func (s *Service) SetInUseCounter(c InUseCounter) { s.counter = c }
func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	rows, n, e := s.repo.List(ctx, q)
	if e != nil {
		return nil, e
	}
	items := make([]Summary, 0, len(rows))
	for _, m := range rows {
		items = append(items, toSummary(m))
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	return &ListResponse{items, n, q.Page, q.PageSize}, nil
}
func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	m, e := s.repo.Get(ctx, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, notFound()
	}
	if e != nil {
		return nil, e
	}
	d := Detail(toSummary(*m))
	return &d, nil
}
func validate(auth, name, user, secret string, creating bool) error {
	name = strings.TrimSpace(name)
	user = strings.TrimSpace(user)
	if name == "" || len(name) > 128 {
		return apperr.New(apperr.CodeValidation, "common.validation", "invalid name")
	}
	if auth != "basic" && auth != "bearer" {
		return apperr.New(apperr.CodeValidation, "common.validation", "invalid auth_type")
	}
	if auth == "basic" && user == "" {
		return apperr.New(apperr.CodeValidation, "common.validation", "username required")
	}
	if auth == "bearer" && user != "" {
		return apperr.New(apperr.CodeValidation, "common.validation", "username forbidden")
	}
	if creating && (secret == "" || len(secret) > 16384) {
		return apperr.New(apperr.CodeValidation, "common.validation", "invalid secret")
	}
	return nil
}
func (s *Service) Create(ctx context.Context, actor uint64, ip, ua string, r CreateRequest) (*Detail, error) {
	r.Name = strings.TrimSpace(r.Name)
	r.Username = strings.TrimSpace(r.Username)
	if e := validate(r.AuthType, r.Name, r.Username, r.Secret, true); e != nil {
		return nil, e
	}
	if _, e := s.repo.FindByName(ctx, r.Name); e == nil {
		return nil, conflict()
	}
	enc, e := s.cipher.Seal([]byte(r.Secret))
	if e != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
	}
	m := &models.HTTPCredential{Name: r.Name, AuthType: r.AuthType, SecretCiphertext: enc}
	if r.AuthType == "basic" {
		m.Username = &r.Username
	}
	if actor != 0 {
		m.CreatedByUserID = &actor
	}
	if e = s.repo.Create(ctx, m); e != nil {
		return nil, e
	}
	s.record(ctx, ptr(actor), "credentials.http_credential.create", m.ID, ip, ua, map[string]any{"name": m.Name, "auth_type": m.AuthType})
	return s.Get(ctx, m.ID)
}
func (s *Service) Update(ctx context.Context, actor uint64, ip, ua string, id uint64, r UpdateRequest) (*Detail, error) {
	m, e := s.repo.Get(ctx, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, notFound()
	}
	if e != nil {
		return nil, e
	}
	name := m.Name
	if r.Name != nil {
		name = strings.TrimSpace(*r.Name)
		if name != m.Name {
			if existing, findErr := s.repo.FindByName(ctx, name); findErr == nil && existing.ID != id {
				return nil, conflict()
			} else if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
				return nil, findErr
			}
		}
	}
	user := ""
	if m.Username != nil {
		user = *m.Username
	}
	if r.Username != nil {
		user = strings.TrimSpace(*r.Username)
	}
	if e = validate(m.AuthType, name, user, "", false); e != nil {
		return nil, e
	}
	f := map[string]any{"name": name}
	if m.AuthType == "basic" {
		f["username"] = user
	} else {
		f["username"] = nil
	}
	if r.Secret != nil {
		if *r.Secret == "" || len(*r.Secret) > 16384 {
			return nil, apperr.New(apperr.CodeValidation, "common.validation", "invalid secret")
		}
		enc, x := s.cipher.Seal([]byte(*r.Secret))
		if x != nil {
			return nil, x
		}
		f["secret_ciphertext"] = enc
	}
	if e = s.repo.Update(ctx, id, f); e != nil {
		return nil, e
	}
	s.record(ctx, ptr(actor), "credentials.http_credential.update", id, ip, ua, map[string]any{"name": name})
	return s.Get(ctx, id)
}
func (s *Service) Delete(ctx context.Context, actor uint64, ip, ua string, id uint64) error {
	m, e := s.repo.Get(ctx, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return notFound()
	}
	if e != nil {
		return e
	}
	if s.counter != nil {
		n, x := s.counter.CountByHTTPCredentialID(ctx, id)
		if x != nil {
			return x
		}
		if n > 0 {
			return apperr.New(apperr.CodeConflict, "credentials.in_use", "credential in use")
		}
	}
	if e = s.repo.Delete(ctx, id); e != nil {
		return e
	}
	s.record(ctx, ptr(actor), "credentials.http_credential.delete", id, ip, ua, map[string]any{"name": m.Name})
	return nil
}

type ConsumeRecord struct {
	Name, AuthType, Username string
	Secret                   []byte
}

func (s *Service) Consume(ctx context.Context, actor *uint64, id uint64, purpose string) (*ConsumeRecord, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return nil, apperr.New(apperr.CodeBadRequest, "credentials.invalid_purpose", "purpose required")
	}
	if actor == nil && !strings.HasPrefix(purpose, "system:") {
		return nil, apperr.New(apperr.CodeBadRequest, "credentials.system_purpose_required", "system purpose required")
	}
	m, e := s.repo.Get(ctx, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, notFound()
	}
	if e != nil {
		return nil, e
	}
	secret, e := s.cipher.Open(m.SecretCiphertext)
	if e != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_open_failed", "open failed")
	}
	u := ""
	if m.Username != nil {
		u = *m.Username
	}
	s.record(ctx, actor, "credentials.consume.http_credential", id, "", "", map[string]any{"name": m.Name, "purpose": purpose})
	return &ConsumeRecord{m.Name, m.AuthType, u, secret}, nil
}
func (s *Service) record(ctx context.Context, a *uint64, action string, id uint64, ip, ua string, p any) {
	if s.audit != nil {
		_ = s.audit.Record(ctx, audit.Event{UserID: a, Action: action, TargetType: "credentials.http_credential", TargetID: strconv.FormatUint(id, 10), Payload: p, IP: ip, UserAgent: ua})
	}
}
func toSummary(m models.HTTPCredential) Summary {
	o := Summary{ID: m.ID, Name: m.Name, AuthType: m.AuthType, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
	if m.AuthType == "basic" {
		o.Username = m.Username
	}
	if m.CreatedByUserID != nil {
		o.CreatedBy = &Actor{ID: *m.CreatedByUserID}
	}
	return o
}
func ptr(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}
func notFound() error {
	return apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
}
func conflict() error {
	return apperr.New(apperr.CodeConflict, "credentials.name_taken", "credential name already exists")
}
