package cloudkey

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
)

// Cipher is the subset of vault.Cipher the service depends on.
type Cipher interface {
	Seal([]byte) ([]byte, error)
	Open([]byte) ([]byte, error)
}

type Service struct {
	repo   *Repo
	cipher Cipher
	audit  *audit.Recorder
}

func NewService(repo *Repo, cipher Cipher, rec *audit.Recorder) *Service {
	return &Service{repo: repo, cipher: cipher, audit: rec}
}

func (s *Service) Repo() *Repo { return s.repo }

// --- queries ---------------------------------------------------------------

func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	rows, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSummary(r))
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	return &ListResponse{Items: out, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
		}
		return nil, err
	}
	d := Detail(toSummary(*m))
	return &d, nil
}

// --- mutations -------------------------------------------------------------

func (s *Service) Create(ctx context.Context, actorID uint64, ip, ua string, req CreateRequest) (*Detail, error) {
	if _, err := s.repo.FindByName(ctx, strings.TrimSpace(req.Name)); err == nil {
		return nil, apperr.New(apperr.CodeConflict, "credentials.name_taken", "credential name already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	akEnc, err := s.cipher.Seal([]byte(req.AccessKeyID))
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
	}
	skEnc, err := s.cipher.Seal([]byte(req.SecretAccessKey))
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
	}

	m := &models.CredentialCloudKey{
		Name:               strings.TrimSpace(req.Name),
		Description:        req.Description,
		Provider:           req.Provider,
		Region:             req.Region,
		AccessKeyIDEnc:     akEnc,
		SecretAccessKeyEnc: skEnc,
	}
	if actorID != 0 {
		v := actorID
		m.CreatedByUserID = &v
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.writeAudit(ctx, ptrIfNonZero(actorID), "credentials.create", m.ID, ip, ua, map[string]any{
		"name": m.Name,
	})
	return s.Get(ctx, m.ID)
}

func (s *Service) Update(ctx context.Context, actorID uint64, ip, ua string, id uint64, req UpdateRequest) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
		}
		return nil, err
	}

	fields := map[string]any{}
	var changed []string
	rotated := false
	finalName := m.Name

	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n != m.Name {
			if _, err := s.repo.FindByName(ctx, n); err == nil {
				return nil, apperr.New(apperr.CodeConflict, "credentials.name_taken", "credential name already exists")
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			fields["name"] = n
			changed = append(changed, "name")
			finalName = n
		}
	}
	if req.Description != nil && *req.Description != m.Description {
		fields["description"] = *req.Description
		changed = append(changed, "description")
	}
	if req.Provider != nil && *req.Provider != m.Provider {
		fields["provider"] = *req.Provider
		changed = append(changed, "provider")
	}
	if req.Region != nil && *req.Region != m.Region {
		fields["region"] = *req.Region
		changed = append(changed, "region")
	}
	if req.AccessKeyID != nil && *req.AccessKeyID != "" {
		enc, err := s.cipher.Seal([]byte(*req.AccessKeyID))
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
		}
		fields["access_key_id_enc"] = enc
		changed = append(changed, "access_key_id")
		rotated = true
	}
	if req.SecretAccessKey != nil && *req.SecretAccessKey != "" {
		enc, err := s.cipher.Seal([]byte(*req.SecretAccessKey))
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
		}
		fields["secret_access_key_enc"] = enc
		changed = append(changed, "secret_access_key")
		rotated = true
	}

	if len(fields) > 0 {
		if err := s.repo.Update(ctx, id, fields); err != nil {
			return nil, err
		}
	}

	if len(changed) > 0 {
		action := "credentials.update"
		if rotated {
			action = "credentials.rotate"
		}
		s.writeAudit(ctx, ptrIfNonZero(actorID), action, id, ip, ua, map[string]any{
			"name":           finalName,
			"changed_fields": changed,
			"secret_rotated": rotated,
		})
	}

	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, actorID uint64, ip, ua string, id uint64) error {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
		}
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.writeAudit(ctx, ptrIfNonZero(actorID), "credentials.delete", id, ip, ua, map[string]any{
		"name": m.Name,
	})
	return nil
}

// --- consume ---------------------------------------------------------------

type ConsumeRecord struct {
	Name            string
	Provider        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
}

func (s *Service) Consume(ctx context.Context, actor *uint64, id uint64, purpose string) (*ConsumeRecord, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return nil, apperr.New(apperr.CodeBadRequest, "credentials.invalid_purpose", "purpose required")
	}
	if actor == nil && !strings.HasPrefix(purpose, "system:") {
		return nil, apperr.New(apperr.CodeBadRequest, "credentials.system_purpose_required", "system caller purpose must start with system:")
	}

	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "credentials.not_found", "credential not found")
		}
		return nil, err
	}
	ak, err := s.cipher.Open(m.AccessKeyIDEnc)
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_open_failed", "open failed")
	}
	sk, err := s.cipher.Open(m.SecretAccessKeyEnc)
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_open_failed", "open failed")
	}

	s.writeAudit(ctx, actor, "credentials.consume", id, "", "", map[string]any{
		"name":    m.Name,
		"purpose": purpose,
	})

	return &ConsumeRecord{
		Name:            m.Name,
		Provider:        m.Provider,
		Region:          m.Region,
		AccessKeyID:     string(ak),
		SecretAccessKey: string(sk),
	}, nil
}

// --- helpers ---------------------------------------------------------------

func (s *Service) writeAudit(ctx context.Context, actor *uint64, action string, id uint64, ip, ua string, payload map[string]any) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID:     actor,
		Action:     action,
		TargetType: "credentials.cloud_key",
		TargetID:   strconv.FormatUint(id, 10),
		Payload:    payload,
		IP:         ip,
		UserAgent:  ua,
	})
}

func ptrIfNonZero(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}

func toSummary(m models.CredentialCloudKey) Summary {
	out := Summary{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Provider:    m.Provider,
		Region:      m.Region,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
	if m.CreatedByUserID != nil {
		out.CreatedBy = &Actor{ID: *m.CreatedByUserID}
	}
	return out
}
