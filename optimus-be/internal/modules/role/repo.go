package role

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

func (r *Repo) Create(ctx context.Context, m *models.Role) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *Repo) Get(ctx context.Context, id uint64) (*models.Role, error) {
	var m models.Role
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) FindByCode(ctx context.Context, code string) (*models.Role, error) {
	var m models.Role
	if err := r.db.WithContext(ctx).Where("code = ?", code).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// List returns all non-deleted roles, ordered by id.
func (r *Repo) List(ctx context.Context) ([]models.Role, error) {
	var rows []models.Role
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&models.Role{}).Where("id = ?", id).Updates(fields).Error
}

// SoftDelete soft-deletes the role and hard-deletes its user_roles and
// role_permissions bindings in one transaction. Returns the user IDs that
// were affected so the caller can invalidate their permission caches.
func (r *Repo) SoftDelete(ctx context.Context, id uint64) ([]uint64, error) {
	var affected []uint64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.UserRole{}).Where("role_id = ?", id).Pluck("user_id", &affected).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", id).Delete(&models.UserRole{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", id).Delete(&models.RolePermission{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Role{}, id).Error
	})
	return affected, err
}

func (r *Repo) ListPermissionCodes(ctx context.Context, roleID uint64) ([]string, error) {
	var codes []string
	err := r.db.WithContext(ctx).
		Table("permissions p").
		Joins("JOIN role_permissions rp ON rp.permission_id = p.id").
		Where("rp.role_id = ?", roleID).
		Order("p.code ASC").
		Pluck("p.code", &codes).Error
	return codes, err
}

// SetPermissionsByCode replaces the role's permission bindings, looking up IDs by code.
// Unknown codes are ignored (caller can check by comparing input vs ListPermissionCodes).
func (r *Repo) SetPermissionsByCode(ctx context.Context, roleID uint64, codes []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&models.RolePermission{}).Error; err != nil {
			return err
		}
		if len(codes) == 0 {
			return nil
		}
		var ids []uint64
		if err := tx.Model(&models.Permission{}).Where("code IN ?", codes).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		rows := make([]models.RolePermission, 0, len(ids))
		for _, pid := range ids {
			rows = append(rows, models.RolePermission{RoleID: roleID, PermissionID: pid})
		}
		return tx.Create(&rows).Error
	})
}

// UserIDsForRole returns user IDs currently bound to this role.
func (r *Repo) UserIDsForRole(ctx context.Context, roleID uint64) ([]uint64, error) {
	var ids []uint64
	err := r.db.WithContext(ctx).Model(&models.UserRole{}).
		Where("role_id = ?", roleID).
		Pluck("user_id", &ids).Error
	return ids, err
}
