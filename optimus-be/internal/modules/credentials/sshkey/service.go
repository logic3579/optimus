package sshkey

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
)

// Cipher is the subset of vault.Cipher the service depends on. Defined as an
// interface so tests can mock it without spinning up real AES.
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
	if err := validatePrivateKey([]byte(req.PrivateKey), req.Passphrase); err != nil {
		return nil, apperr.New(apperr.CodeBadRequest, "credentials.invalid_key_format", err.Error())
	}
	if _, err := s.repo.FindByName(ctx, strings.TrimSpace(req.Name)); err == nil {
		return nil, apperr.New(apperr.CodeConflict, "credentials.name_taken", "credential name already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	pkEnc, err := s.cipher.Seal([]byte(req.PrivateKey))
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
	}
	var passEnc []byte
	if req.Passphrase != "" {
		passEnc, err = s.cipher.Seal([]byte(req.Passphrase))
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
		}
	}

	m := &models.CredentialSSHKey{
		Name:          strings.TrimSpace(req.Name),
		Description:   req.Description,
		Username:      req.Username,
		PrivateKeyEnc: pkEnc,
		PassphraseEnc: passEnc,
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
	if req.Username != nil && *req.Username != m.Username {
		fields["username"] = *req.Username
		changed = append(changed, "username")
	}
	if req.PrivateKey != nil && *req.PrivateKey != "" {
		var pp string
		if req.Passphrase != nil {
			pp = *req.Passphrase
		}
		if err := validatePrivateKey([]byte(*req.PrivateKey), pp); err != nil {
			return nil, apperr.New(apperr.CodeBadRequest, "credentials.invalid_key_format", err.Error())
		}
		enc, err := s.cipher.Seal([]byte(*req.PrivateKey))
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
		}
		fields["private_key_enc"] = enc
		changed = append(changed, "private_key")
		rotated = true
	}
	if req.Passphrase != nil {
		if *req.Passphrase == "" {
			// Clear an existing passphrase.
			if len(m.PassphraseEnc) > 0 {
				fields["passphrase_enc"] = nil
				changed = append(changed, "passphrase")
			}
		} else {
			enc, err := s.cipher.Seal([]byte(*req.Passphrase))
			if err != nil {
				return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_seal_failed", "seal failed")
			}
			fields["passphrase_enc"] = enc
			changed = append(changed, "passphrase")
			rotated = true
		}
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

// ConsumeRecord is the decrypted shape returned to the consume seam.
type ConsumeRecord struct {
	Name       string
	Username   string
	PrivateKey []byte
	Passphrase []byte
}

// Consume decrypts and returns the credential, writing an audit row.
// actor==nil means a system caller — purpose must then start with "system:".
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
	pk, err := s.cipher.Open(m.PrivateKeyEnc)
	if err != nil {
		return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_open_failed", "open failed")
	}
	var pp []byte
	if len(m.PassphraseEnc) > 0 {
		pp, err = s.cipher.Open(m.PassphraseEnc)
		if err != nil {
			return nil, apperr.New(apperr.CodeInternal, "credentials.crypto_open_failed", "open failed")
		}
	}

	// Best-effort audit: never fail a consume on audit-write error.
	s.writeAudit(ctx, actor, "credentials.consume", id, "", "", map[string]any{
		"name":    m.Name,
		"purpose": purpose,
	})

	return &ConsumeRecord{
		Name:       m.Name,
		Username:   m.Username,
		PrivateKey: pk,
		Passphrase: pp,
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
		TargetType: "credentials.ssh_key",
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

func toSummary(m models.CredentialSSHKey) Summary {
	out := Summary{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Username:    m.Username,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
	if m.CreatedByUserID != nil {
		out.CreatedBy = &Actor{ID: *m.CreatedByUserID}
	}
	return out
}

// validatePrivateKey accepts a PEM-encoded SSH private key. If the key parses
// without a passphrase, the result is OK regardless of whether a passphrase
// argument was supplied (the passphrase is paired metadata, not necessarily a
// decryption key for THIS blob). If the key is encrypted, the passphrase MUST
// decrypt it.
func validatePrivateKey(pemBytes []byte, passphrase string) error {
	if _, err := ssh.ParseRawPrivateKey(pemBytes); err == nil {
		return nil
	} else if !isPassphraseMissing(err) {
		return err
	}
	if passphrase == "" {
		return errors.New("ssh: private key is encrypted but no passphrase was provided")
	}
	_, err := ssh.ParseRawPrivateKeyWithPassphrase(pemBytes, []byte(passphrase))
	return err
}

func isPassphraseMissing(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*ssh.PassphraseMissingError)
	return ok
}
