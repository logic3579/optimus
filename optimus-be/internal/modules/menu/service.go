package menu

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
)

type Service struct {
	repo  *Repo
	audit *audit.Recorder
}

func NewService(repo *Repo, rec *audit.Recorder) *Service {
	return &Service{repo: repo, audit: rec}
}

func (s *Service) Repo() *Repo { return s.repo }

func (s *Service) Tree(ctx context.Context) ([]MenuNode, error) {
	return s.repo.Tree(ctx)
}

func (s *Service) Create(ctx context.Context, actorID uint64, ip, ua string, req CreateRequest) (*MenuNode, error) {
	if _, err := s.repo.FindByCode(ctx, req.Code); err == nil {
		return nil, apperr.New(apperr.CodeMenuAlreadyExists, "menu.exists", "menu code already in use")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if req.ParentID != nil {
		if _, err := s.repo.Get(ctx, *req.ParentID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperr.New(apperr.CodeNotFound, "menu.parent_not_found", "parent menu not found")
			}
			return nil, err
		}
	}
	m := &models.Menu{
		ParentID: req.ParentID, Code: req.Code, Name: req.Name,
		Path: req.Path, Component: req.Component, Icon: req.Icon,
		PermissionCode: req.PermissionCode, SortOrder: req.SortOrder, Hidden: req.Hidden,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "menu.create", TargetType: "menu", TargetID: strconv.FormatUint(m.ID, 10),
		Payload: map[string]any{"after": map[string]any{"code": m.Code}},
		IP:      ip, UserAgent: ua,
	})
	return toNode(*m), nil
}

func (s *Service) Update(ctx context.Context, actorID uint64, ip, ua string, id uint64, req UpdateRequest) (*MenuNode, error) {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "common.not_found", "menu not found")
		}
		return nil, err
	}
	fields := map[string]any{}
	if req.ParentID != nil {
		if *req.ParentID == id {
			return nil, apperr.New(apperr.CodeBadRequest, "menu.cycle", "menu cannot be its own parent")
		}
		// Walk up from the proposed parent; if we reach `id`, it's a cycle.
		cur := *req.ParentID
		for cur != 0 {
			if cur == id {
				return nil, apperr.New(apperr.CodeBadRequest, "menu.cycle", "cycle detected in menu hierarchy")
			}
			p, err := s.repo.Get(ctx, cur)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, apperr.New(apperr.CodeNotFound, "menu.parent_not_found", "parent menu not found")
				}
				return nil, err
			}
			if p.ParentID == nil {
				break
			}
			cur = *p.ParentID
		}
		fields["parent_id"] = *req.ParentID
	}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.Path != nil {
		fields["path"] = *req.Path
	}
	if req.Component != nil {
		fields["component"] = *req.Component
	}
	if req.Icon != nil {
		fields["icon"] = *req.Icon
	}
	if req.PermissionCode != nil {
		fields["permission_code"] = *req.PermissionCode
	}
	if req.SortOrder != nil {
		fields["sort_order"] = *req.SortOrder
	}
	if req.Hidden != nil {
		fields["hidden"] = *req.Hidden
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "menu.update", TargetType: "menu", TargetID: strconv.FormatUint(before.ID, 10),
		Payload: map[string]any{"changed": fields},
		IP:      ip, UserAgent: ua,
	})
	after, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return toNode(*after), nil
}

func (s *Service) Delete(ctx context.Context, actorID uint64, ip, ua string, id uint64) error {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "common.not_found", "menu not found")
		}
		return err
	}
	hasChildren, err := s.repo.HasChildren(ctx, id)
	if err != nil {
		return err
	}
	if hasChildren {
		return apperr.New(apperr.CodeConflict, "menu.has_children", "menu has child menus; remove them first")
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return err
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: &actorID, Action: "menu.delete", TargetType: "menu", TargetID: strconv.FormatUint(before.ID, 10),
		Payload: map[string]any{"before": map[string]any{"code": before.Code}},
		IP:      ip, UserAgent: ua,
	})
	return nil
}

func toNode(m models.Menu) *MenuNode {
	return &MenuNode{
		ID: m.ID, ParentID: m.ParentID, Code: m.Code, Name: m.Name,
		Path: m.Path, Component: m.Component, Icon: m.Icon,
		PermissionCode: m.PermissionCode, SortOrder: m.SortOrder, Hidden: m.Hidden,
	}
}
