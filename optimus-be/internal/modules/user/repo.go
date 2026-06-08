package user

import (
	"context"

	"gorm.io/gorm"

	"optimus-be/internal/infra/pagination"
	"optimus-be/internal/models"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) Create(ctx context.Context, u *models.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *Repo) Get(ctx context.Context, id uint64) (*models.User, error) {
	var u models.User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repo) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	var u models.User
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repo) List(ctx context.Context, q ListQuery, p pagination.Params) ([]models.User, int64, error) {
	tx := r.db.WithContext(ctx).Model(&models.User{})
	if q.Search != "" {
		s := "%" + q.Search + "%"
		tx = tx.Where("username ILIKE ? OR email ILIKE ?", s, s)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.User
	if err := tx.Order("id DESC").Offset(p.Offset()).Limit(p.Limit()).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Updates(fields).Error
}

// SoftDelete soft-deletes the user AND hard-deletes associations (user_roles, refresh_tokens)
// in a single transaction (spec §6.2: cascading FKs don't fire on soft delete).
func (r *Repo) SoftDelete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&models.UserRole{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&models.RefreshToken{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.User{}, id).Error
	})
}

// SetRoles replaces the user's role bindings atomically.
func (r *Repo) SetRoles(ctx context.Context, userID uint64, roleIDs []uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&models.UserRole{}).Error; err != nil {
			return err
		}
		if len(roleIDs) == 0 {
			return nil
		}
		rows := make([]models.UserRole, 0, len(roleIDs))
		for _, rid := range roleIDs {
			rows = append(rows, models.UserRole{UserID: userID, RoleID: rid})
		}
		return tx.Create(&rows).Error
	})
}

func (r *Repo) ListRoleIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	var ids []uint64
	err := r.db.WithContext(ctx).Model(&models.UserRole{}).
		Where("user_id = ?", userID).
		Order("role_id ASC").
		Pluck("role_id", &ids).Error
	return ids, err
}

func (r *Repo) ListRolesForUser(ctx context.Context, userID uint64) ([]models.Role, error) {
	var roles []models.Role
	err := r.db.WithContext(ctx).
		Joins("JOIN user_roles ur ON ur.role_id = roles.id").
		Where("ur.user_id = ? AND roles.deleted_at IS NULL", userID).
		Order("roles.id ASC").
		Find(&roles).Error
	return roles, err
}
