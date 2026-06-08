package role

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/rbac"
)

type Service struct {
	repo  *Repo
	cache *rbac.PermissionCache
	audit *audit.Recorder
}

func NewService(repo *Repo, cache *rbac.PermissionCache, rec *audit.Recorder) *Service {
	return &Service{repo: repo, cache: cache, audit: rec}
}

func (s *Service) Repo() *Repo { return s.repo }

func (s *Service) List(ctx context.Context) ([]RoleSummary, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RoleSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSummary(r))
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*RoleDetail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "common.not_found", "role not found")
		}
		return nil, err
	}
	codes, err := s.repo.ListPermissionCodes(ctx, id)
	if err != nil {
		return nil, err
	}
	return &RoleDetail{RoleSummary: toSummary(*m), PermissionCodes: codes}, nil
}

func (s *Service) Create(ctx context.Context, actorID uint64, ip, ua string, req CreateRequest) (*RoleDetail, error) {
	if _, err := s.repo.FindByCode(ctx, req.Code); err == nil {
		return nil, apperr.New(apperr.CodeRoleAlreadyExists, "role.exists", "role code already in use")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	m := &models.Role{Code: req.Code, Name: req.Name, Description: req.Description, IsBuiltin: false}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "role.create", TargetType: "role", TargetID: strconv.FormatUint(m.ID, 10),
		Payload: map[string]any{"after": map[string]any{"code": m.Code, "name": m.Name}},
		IP:      ip, UserAgent: ua,
	})
	return s.Get(ctx, m.ID)
}

func (s *Service) Update(ctx context.Context, actorID uint64, ip, ua string, id uint64, req UpdateRequest) (*RoleDetail, error) {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "common.not_found", "role not found")
		}
		return nil, err
	}
	fields := map[string]any{}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.Description != nil {
		fields["description"] = *req.Description
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "role.update", TargetType: "role", TargetID: strconv.FormatUint(before.ID, 10),
		Payload: map[string]any{"changed": fields},
		IP:      ip, UserAgent: ua,
	})
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, actorID uint64, ip, ua string, id uint64) error {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "common.not_found", "role not found")
		}
		return err
	}
	if before.IsBuiltin {
		return apperr.New(apperr.CodeBuiltinRoleImmutable, "role.builtin", "built-in role cannot be deleted")
	}
	userIDs, err := s.repo.SoftDelete(ctx, id)
	if err != nil {
		return err
	}
	for _, uid := range userIDs {
		s.cache.InvalidateUser(uid)
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "role.delete", TargetType: "role", TargetID: strconv.FormatUint(before.ID, 10),
		Payload: map[string]any{"before": map[string]any{"code": before.Code}},
		IP:      ip, UserAgent: ua,
	})
	return nil
}

func (s *Service) SetPermissions(ctx context.Context, actorID uint64, ip, ua string, id uint64, codes []string) error {
	if _, err := s.repo.Get(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "common.not_found", "role not found")
		}
		return err
	}
	userIDs, err := s.repo.UserIDsForRole(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.SetPermissionsByCode(ctx, id, codes); err != nil {
		return err
	}
	for _, uid := range userIDs {
		s.cache.InvalidateUser(uid)
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "role.set_permissions", TargetType: "role", TargetID: strconv.FormatUint(id, 10),
		Payload: map[string]any{"codes": codes},
		IP:      ip, UserAgent: ua,
	})
	return nil
}

func toSummary(r models.Role) RoleSummary {
	return RoleSummary{
		ID: r.ID, Code: r.Code, Name: r.Name, Description: r.Description,
		IsBuiltin: r.IsBuiltin, CreatedAt: r.CreatedAt,
	}
}
