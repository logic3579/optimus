package user

import (
	"context"
	"errors"
	"strconv"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/pagination"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/rbac"
)

type ServiceOptions struct {
	BcryptCost    int
	AdminUsername string // username of seeded built-in admin (cannot be deleted)
}

type Service struct {
	repo  *Repo
	cache *rbac.PermissionCache
	audit *audit.Recorder
	opts  ServiceOptions
}

func NewService(repo *Repo, cache *rbac.PermissionCache, rec *audit.Recorder, opts ServiceOptions) *Service {
	if opts.BcryptCost == 0 {
		opts.BcryptCost = bcrypt.DefaultCost
	}
	return &Service{repo: repo, cache: cache, audit: rec, opts: opts}
}

func (s *Service) Repo() *Repo { return s.repo }

func (s *Service) List(ctx context.Context, q ListQuery, p pagination.Params) (pagination.Page[UserSummary], error) {
	rows, total, err := s.repo.List(ctx, q, p)
	if err != nil {
		return pagination.Page[UserSummary]{}, err
	}
	items := make([]UserSummary, 0, len(rows))
	for _, u := range rows {
		items = append(items, toSummary(u))
	}
	return pagination.Of(items, total, p), nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*UserDetail, error) {
	u, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "common.not_found", "user not found")
		}
		return nil, err
	}
	roles, err := s.repo.ListRolesForUser(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return toDetail(*u, roles), nil
}

func (s *Service) Create(ctx context.Context, actorID uint64, ip, ua string, req CreateRequest) (*UserDetail, error) {
	if _, err := s.repo.FindByUsername(ctx, req.Username); err == nil {
		return nil, apperr.New(apperr.CodeUserAlreadyExists, "user.exists", "username already in use")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if _, err := s.repo.FindByEmail(ctx, req.Email); err == nil {
		return nil, apperr.New(apperr.CodeUserAlreadyExists, "user.exists", "email already in use")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.opts.BcryptCost)
	if err != nil {
		return nil, err
	}
	u := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
		DisplayName:  req.DisplayName,
		Status:       "enabled",
		CreatedBy:    &actorID,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	if len(req.RoleIDs) > 0 {
		if err := s.repo.SetRoles(ctx, u.ID, req.RoleIDs); err != nil {
			return nil, err
		}
		s.cache.InvalidateUser(u.ID)
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "user.create", TargetType: "user", TargetID: u.IDString(),
		Payload: map[string]any{"after": map[string]any{"username": u.Username, "email": u.Email}},
		IP:      ip, UserAgent: ua,
	})
	return s.Get(ctx, u.ID)
}

func (s *Service) Update(ctx context.Context, actorID uint64, ip, ua string, id uint64, req UpdateRequest) (*UserDetail, error) {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "common.not_found", "user not found")
		}
		return nil, err
	}
	fields := map[string]any{}
	if req.Email != nil && *req.Email != before.Email {
		if _, err := s.repo.FindByEmail(ctx, *req.Email); err == nil {
			return nil, apperr.New(apperr.CodeUserAlreadyExists, "user.exists", "email already in use")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		fields["email"] = *req.Email
	}
	if req.DisplayName != nil {
		fields["display_name"] = *req.DisplayName
	}
	if req.AvatarURL != nil {
		fields["avatar_url"] = *req.AvatarURL
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "user.update", TargetType: "user", TargetID: before.IDString(),
		Payload: map[string]any{"changed": fields},
		IP:      ip, UserAgent: ua,
	})
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, actorID uint64, ip, ua string, id uint64) error {
	if actorID == id {
		return apperr.New(apperr.CodeCannotDeleteSelf, "user.delete_self", "cannot delete the currently authenticated user")
	}
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "common.not_found", "user not found")
		}
		return err
	}
	if before.Username == s.opts.AdminUsername {
		return apperr.New(apperr.CodeCannotDeleteAdmin, "user.delete_admin", "cannot delete the built-in admin user")
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return err
	}
	s.cache.InvalidateUser(id)
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "user.delete", TargetType: "user", TargetID: before.IDString(),
		Payload: map[string]any{"before": map[string]any{"username": before.Username}},
		IP:      ip, UserAgent: ua,
	})
	return nil
}

func (s *Service) SetRoles(ctx context.Context, actorID uint64, ip, ua string, id uint64, roleIDs []uint64) error {
	if _, err := s.repo.Get(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "common.not_found", "user not found")
		}
		return err
	}
	if err := s.repo.SetRoles(ctx, id, roleIDs); err != nil {
		return err
	}
	s.cache.InvalidateUser(id)
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "user.set_roles", TargetType: "user", TargetID: uintToStr(id),
		Payload: map[string]any{"role_ids": roleIDs},
		IP:      ip, UserAgent: ua,
	})
	return nil
}

func (s *Service) SetStatus(ctx context.Context, actorID uint64, ip, ua string, id uint64, status string) error {
	if actorID == id && status == "disabled" {
		return apperr.New(apperr.CodeCannotDeleteSelf, "user.disable_self", "cannot disable the currently authenticated user")
	}
	if _, err := s.repo.Get(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "common.not_found", "user not found")
		}
		return err
	}
	if err := s.repo.Update(ctx, id, map[string]any{"status": status}); err != nil {
		return err
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "user.set_status", TargetType: "user", TargetID: uintToStr(id),
		Payload: map[string]any{"status": status},
		IP:      ip, UserAgent: ua,
	})
	return nil
}

func (s *Service) SetPassword(ctx context.Context, actorID uint64, ip, ua string, id uint64, password string) error {
	if _, err := s.repo.Get(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "common.not_found", "user not found")
		}
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.opts.BcryptCost)
	if err != nil {
		return err
	}
	if err := s.repo.Update(ctx, id, map[string]any{"password_hash": string(hash)}); err != nil {
		return err
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "user.reset_password", TargetType: "user", TargetID: uintToStr(id),
		IP: ip, UserAgent: ua,
	})
	return nil
}

func uintToStr(n uint64) string { return strconv.FormatUint(n, 10) }

func toSummary(u models.User) UserSummary {
	return UserSummary{
		ID: u.ID, Username: u.Username, Email: u.Email, DisplayName: u.DisplayName,
		Status: u.Status, LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt,
	}
}

func toDetail(u models.User, roles []models.Role) *UserDetail {
	refs := make([]RoleRef, 0, len(roles))
	for _, r := range roles {
		refs = append(refs, RoleRef{ID: r.ID, Code: r.Code, Name: r.Name})
	}
	return &UserDetail{
		UserSummary: toSummary(u),
		AvatarURL:   u.AvatarURL,
		Roles:       refs,
	}
}
