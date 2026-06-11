package repo

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
)

// Cipher is the subset of vault.Cipher the service depends on. Defined as an
// interface so tests can mock it without spinning up real AES. The single
// vault.Cipher instance built in main.go satisfies this interface and is the
// SAME cipher P1's credentials.Consumer uses — never construct a second one.
type Cipher interface {
	Seal([]byte) ([]byte, error)
	Open([]byte) ([]byte, error)
}

// InUseCounter is the seam apps/application implements. Injected post-
// construction by main.go (via SetInUseCounter) to avoid an import cycle.
type InUseCounter interface {
	CountByChartRepoID(ctx context.Context, repoID uint64) (int, error)
}

// Service owns vault encrypt/decrypt for password and audit emission.
type Service struct {
	repo   *Repo
	cipher Cipher
	audit  *audit.Recorder
	inuse  InUseCounter
}

// NewService returns a Service. The vault.Cipher is injected (NOT constructed
// here) so the whole process shares one AEAD instance.
func NewService(r *Repo, c Cipher, rec *audit.Recorder) *Service {
	return &Service{repo: r, cipher: c, audit: rec}
}

// Repo exposes the underlying repo (used by main.go's inuse wiring and tests).
func (s *Service) Repo() *Repo { return s.repo }

// SetInUseCounter wires the apps/application counter post-construction.
func (s *Service) SetInUseCounter(c InUseCounter) { s.inuse = c }

// passwordClearSentinel is the value handler.Update stuffs into req.Password
// when the JSON body has "password": null. Empty string means "keep".
const passwordClearSentinel = "\x00"

// --- queries ---------------------------------------------------------------

// List returns one page of chart repos as Summary rows.
func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	rows, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]Summary, 0, len(rows))
	for i := range rows {
		items = append(items, toSummary(&rows[i]))
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	return &ListResponse{Items: items, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

// Get returns one chart repo as a Detail, mapping ErrRecordNotFound to 40401.
func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "apps.repo.not_found", "chart repo not found")
		}
		return nil, err
	}
	d := toSummary(m)
	return &d, nil
}

// --- mutations -------------------------------------------------------------

// Create persists a new chart repo. Password is sealed via the vault cipher;
// plaintext never leaves this call.
func (s *Service) Create(ctx context.Context, actorID uint64, ip, ua string, req CreateRequest) (*Detail, error) {
	if _, err := s.repo.FindByName(ctx, req.Name); err == nil {
		return nil, apperr.New(apperr.CodeConflict, "apps.repo.name_taken", "chart repo name already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var encrypted []byte
	if req.Password != "" {
		ct, err := s.cipher.Seal([]byte(req.Password))
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "apps.repo.crypto_seal_failed", "seal failed")
		}
		encrypted = ct
	}
	m := &models.AppsChartRepo{
		Name:              req.Name,
		Type:              req.Type,
		URL:               req.URL,
		Username:          req.Username,
		EncryptedPassword: encrypted,
		Description:       req.Description,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.writeAudit(ctx, ptrIfNonZero(actorID), "apps.repo.create", m.ID, ip, ua, map[string]any{
		"name": m.Name,
		"type": m.Type,
		"url":  m.URL,
	})
	return s.Get(ctx, m.ID)
}

// Update mutates allowed fields. Type is silently ignored. Password follows
// the absent/empty/null tri-state semantics described in UpdateRequest.
func (s *Service) Update(ctx context.Context, actorID uint64, ip, ua string, id uint64, req UpdateRequest) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "apps.repo.not_found", "chart repo not found")
		}
		return nil, err
	}

	fields := map[string]any{}
	var changed []string

	if req.Name != nil && *req.Name != m.Name {
		other, err := s.repo.FindByName(ctx, *req.Name)
		if err == nil && other != nil && other.ID != m.ID {
			return nil, apperr.New(apperr.CodeConflict, "apps.repo.name_taken", "chart repo name already exists")
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		fields["name"] = *req.Name
		changed = append(changed, "name")
	}
	if req.URL != nil && *req.URL != m.URL {
		fields["url"] = *req.URL
		changed = append(changed, "url")
	}
	if req.Username != nil && *req.Username != m.Username {
		fields["username"] = *req.Username
		changed = append(changed, "username")
	}
	if req.Description != nil && *req.Description != m.Description {
		fields["description"] = *req.Description
		changed = append(changed, "description")
	}
	if req.Password != nil {
		switch *req.Password {
		case "":
			// empty string -> keep current ciphertext (no-op).
		case passwordClearSentinel:
			fields["encrypted_password"] = []byte{}
			changed = append(changed, "password")
		default:
			ct, err := s.cipher.Seal([]byte(*req.Password))
			if err != nil {
				return nil, apperr.New(apperr.CodeInternal, "apps.repo.crypto_seal_failed", "seal failed")
			}
			fields["encrypted_password"] = ct
			changed = append(changed, "password")
		}
	}

	if len(fields) > 0 {
		if err := s.repo.Update(ctx, id, fields); err != nil {
			return nil, err
		}
	}
	if len(changed) > 0 {
		s.writeAudit(ctx, ptrIfNonZero(actorID), "apps.repo.update", id, ip, ua, map[string]any{
			"name":           m.Name,
			"changed_fields": changed,
		})
	}
	return s.Get(ctx, id)
}

// Delete soft-deletes one chart repo. Refuses with CodeAppsChartRepoInUse if
// the registered InUseCounter reports any referencing applications.
func (s *Service) Delete(ctx context.Context, actorID uint64, ip, ua string, id uint64) error {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "apps.repo.not_found", "chart repo not found")
		}
		return err
	}
	if s.inuse != nil {
		n, err := s.inuse.CountByChartRepoID(ctx, id)
		if err != nil {
			return err
		}
		if n > 0 {
			return apperr.New(apperr.CodeAppsChartRepoInUse, "apps.repo.in_use", "chart repo still referenced by applications")
		}
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.writeAudit(ctx, ptrIfNonZero(actorID), "apps.repo.delete", id, ip, ua, map[string]any{
		"name": m.Name,
	})
	return nil
}

// --- internal helpers ------------------------------------------------------

// decryptPassword returns plaintext scoped to one call. Used by chart
// enumeration (charts.go) only — never returned to clients.
func (s *Service) decryptPassword(_ context.Context, m *models.AppsChartRepo) (string, error) {
	if len(m.EncryptedPassword) == 0 {
		return "", nil
	}
	pt, err := s.cipher.Open(m.EncryptedPassword)
	if err != nil {
		return "", apperr.New(apperr.CodeInternal, "apps.repo.crypto_open_failed", "open failed")
	}
	return string(pt), nil
}

func (s *Service) writeAudit(ctx context.Context, actor *uint64, action string, id uint64, ip, ua string, payload map[string]any) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID:     actor,
		Action:     action,
		TargetType: "apps.chart_repo",
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

func toSummary(m *models.AppsChartRepo) Summary {
	return Summary{
		ID:          m.ID,
		Name:        m.Name,
		Type:        m.Type,
		URL:         m.URL,
		Username:    m.Username,
		HasPassword: len(m.EncryptedPassword) > 0,
		Description: m.Description,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}
