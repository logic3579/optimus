package kubeconfig

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"k8s.io/client-go/tools/clientcmd"

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
	if err := validateKubeconfig([]byte(req.Kubeconfig)); err != nil {
		return nil, apperr.New(apperr.CodeBadRequest, "credentials.invalid_key_format", err.Error())
	}
	if _, err := s.repo.FindByName(ctx, strings.TrimSpace(req.Name)); err == nil {
		return nil, apperr.New(apperr.CodeConflict, "credentials.name_taken", "credential name already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	enc, err := s.cipher.Seal([]byte(req.Kubeconfig))
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
	}

	m := &models.CredentialKubeconfig{
		Name:             strings.TrimSpace(req.Name),
		Description:      req.Description,
		DefaultNamespace: req.DefaultNamespace,
		KubeconfigEnc:    enc,
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
	if req.DefaultNamespace != nil && *req.DefaultNamespace != m.DefaultNamespace {
		fields["default_namespace"] = *req.DefaultNamespace
		changed = append(changed, "default_namespace")
	}
	if req.Kubeconfig != nil && *req.Kubeconfig != "" {
		if err := validateKubeconfig([]byte(*req.Kubeconfig)); err != nil {
			return nil, apperr.New(apperr.CodeBadRequest, "credentials.invalid_key_format", err.Error())
		}
		enc, err := s.cipher.Seal([]byte(*req.Kubeconfig))
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
		}
		fields["kubeconfig_enc"] = enc
		changed = append(changed, "kubeconfig")
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
	Name             string
	DefaultNamespace string
	YAML             []byte
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
	yaml, err := s.cipher.Open(m.KubeconfigEnc)
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_open_failed", "open failed")
	}

	s.writeAudit(ctx, actor, "credentials.consume", id, "", "", map[string]any{
		"name":    m.Name,
		"purpose": purpose,
	})

	return &ConsumeRecord{
		Name:             m.Name,
		DefaultNamespace: m.DefaultNamespace,
		YAML:             yaml,
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
		TargetType: "credentials.kubeconfig",
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

func toSummary(m models.CredentialKubeconfig) Summary {
	out := Summary{
		ID:               m.ID,
		Name:             m.Name,
		Description:      m.Description,
		DefaultNamespace: m.DefaultNamespace,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
	if m.CreatedByUserID != nil {
		out.CreatedBy = &Actor{ID: *m.CreatedByUserID}
	}
	return out
}

func validateKubeconfig(raw []byte) error {
	cfg, err := clientcmd.Load(raw)
	if err != nil {
		return err
	}
	if len(cfg.Contexts) == 0 {
		return errors.New("kubeconfig has no contexts")
	}
	return nil
}
