package menu

import (
	"context"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }
func (r *Repo) DB() *gorm.DB    { return r.db }

func (r *Repo) Create(ctx context.Context, m *models.Menu) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *Repo) Get(ctx context.Context, id uint64) (*models.Menu, error) {
	var m models.Menu
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) FindByCode(ctx context.Context, code string) (*models.Menu, error) {
	var m models.Menu
	if err := r.db.WithContext(ctx).Where("code = ?", code).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) List(ctx context.Context) ([]models.Menu, error) {
	var rows []models.Menu
	if err := r.db.WithContext(ctx).Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&models.Menu{}).Where("id = ?", id).Updates(fields).Error
}

func (r *Repo) SoftDelete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&models.Menu{}, id).Error
}

func (r *Repo) HasChildren(ctx context.Context, id uint64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Menu{}).Where("parent_id = ?", id).Count(&n).Error
	return n > 0, err
}

// Tree returns all menus as a nested MenuNode tree (parents first).
func (r *Repo) Tree(ctx context.Context) ([]MenuNode, error) {
	rows, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	byParent := map[uint64][]models.Menu{}
	const rootKey uint64 = 0
	for _, m := range rows {
		key := rootKey
		if m.ParentID != nil {
			key = *m.ParentID
		}
		byParent[key] = append(byParent[key], m)
	}
	var build func(parentID uint64) []MenuNode
	build = func(parentID uint64) []MenuNode {
		kids := byParent[parentID]
		out := make([]MenuNode, 0, len(kids))
		for _, m := range kids {
			out = append(out, MenuNode{
				ID: m.ID, ParentID: m.ParentID, Code: m.Code, Name: m.Name,
				Path: m.Path, Component: m.Component, Icon: m.Icon,
				PermissionCode: m.PermissionCode, SortOrder: m.SortOrder, Hidden: m.Hidden,
				Children: build(m.ID),
			})
		}
		return out
	}
	return build(rootKey), nil
}
