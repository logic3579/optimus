package seed

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type Options struct {
	AdminUsername string
	AdminEmail    string
	BcryptCost    int // 0 → bcrypt.DefaultCost
}

type Result struct {
	AdminInitialPassword string // populated only on first creation
}

func Run(ctx context.Context, gdb *gorm.DB, opts Options) (*Result, error) {
	if opts.BcryptCost == 0 {
		opts.BcryptCost = bcrypt.DefaultCost
	}
	result := &Result{}

	if err := gdb.Transaction(func(tx *gorm.DB) error {
		if err := ensureBuiltinRoles(ctx, tx); err != nil {
			return err
		}
		if err := bindAdminPermissions(ctx, tx); err != nil {
			return err
		}
		if err := bindViewerPermissions(ctx, tx); err != nil {
			return err
		}
		if err := ensureInitialMenus(ctx, tx); err != nil {
			return err
		}
		pw, err := ensureAdminUser(ctx, tx, opts)
		if err != nil {
			return err
		}
		result.AdminInitialPassword = pw
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func ensureBuiltinRoles(ctx context.Context, tx *gorm.DB) error {
	roles := []models.Role{
		{Code: "admin", Name: "role.admin", Description: "Full access", IsBuiltin: true},
		{Code: "viewer", Name: "role.viewer", Description: "Read-only", IsBuiltin: true},
	}
	for i := range roles {
		var existing models.Role
		err := tx.WithContext(ctx).Where("code = ?", roles[i].Code).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&roles[i]).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func bindAdminPermissions(ctx context.Context, tx *gorm.DB) error {
	var role models.Role
	if err := tx.WithContext(ctx).Where("code = ?", "admin").First(&role).Error; err != nil {
		return err
	}
	var perms []models.Permission
	if err := tx.WithContext(ctx).Find(&perms).Error; err != nil {
		return err
	}
	if len(perms) == 0 {
		return errors.New("no permissions found in DB; run permissions.Register before seed.Run")
	}
	return bindPermsToRole(ctx, tx, role.ID, perms)
}

func bindViewerPermissions(ctx context.Context, tx *gorm.DB) error {
	var role models.Role
	if err := tx.WithContext(ctx).Where("code = ?", "viewer").First(&role).Error; err != nil {
		return err
	}
	var perms []models.Permission
	if err := tx.WithContext(ctx).Where("code LIKE ?", "%:read").Find(&perms).Error; err != nil {
		return err
	}
	return bindPermsToRole(ctx, tx, role.ID, perms)
}

func bindPermsToRole(ctx context.Context, tx *gorm.DB, roleID uint64, perms []models.Permission) error {
	for _, p := range perms {
		rp := models.RolePermission{RoleID: roleID, PermissionID: p.ID}
		if err := tx.WithContext(ctx).Where("role_id = ? AND permission_id = ?", roleID, p.ID).
			FirstOrCreate(&rp).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureInitialMenus(ctx context.Context, tx *gorm.DB) error {
	type spec struct {
		Code, Name, Path, Component, Icon string
		PermissionCode                    *string
		Children                          []spec
	}
	sp := func(s string) *string { return &s }
	tree := []spec{
		{Code: "dashboard", Name: "menu.dashboard", Path: "/dashboard", Component: "dashboard/Index", Icon: "dashboard"},
		{Code: "system", Name: "menu.system_group", Path: "/system", Component: "", Icon: "setting", Children: []spec{
			{Code: "system.users", Name: "menu.system.users", Path: "/system/users", Component: "system/users/List", PermissionCode: sp("system:user:read")},
			{Code: "system.roles", Name: "menu.system.roles", Path: "/system/roles", Component: "system/roles/List", PermissionCode: sp("system:role:read")},
			{Code: "system.permissions", Name: "menu.system.permissions", Path: "/system/permissions", Component: "system/permissions/List", PermissionCode: sp("system:permission:read")},
			{Code: "system.menus", Name: "menu.system.menus", Path: "/system/menus", Component: "system/menus/List", PermissionCode: sp("system:menu:read")},
			{Code: "system.audit_logs", Name: "menu.system.audit_logs", Path: "/system/audit-logs", Component: "system/audit-logs/List", PermissionCode: sp("system:audit:read")},
		}},
	}
	var insert func(parentID *uint64, nodes []spec) error
	insert = func(parentID *uint64, nodes []spec) error {
		for i, n := range nodes {
			var existing models.Menu
			err := tx.WithContext(ctx).Where("code = ?", n.Code).First(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				m := models.Menu{
					ParentID:       parentID,
					Code:           n.Code,
					Name:           n.Name,
					Path:           n.Path,
					Component:      n.Component,
					Icon:           n.Icon,
					PermissionCode: n.PermissionCode,
					SortOrder:      i,
				}
				if err := tx.Create(&m).Error; err != nil {
					return err
				}
				existing = m
			} else if err != nil {
				return err
			}
			if len(n.Children) > 0 {
				id := existing.ID
				if err := insert(&id, n.Children); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return insert(nil, tree)
}

func ensureAdminUser(ctx context.Context, tx *gorm.DB, opts Options) (string, error) {
	var existing models.User
	err := tx.WithContext(ctx).Where("username = ?", opts.AdminUsername).First(&existing).Error
	if err == nil {
		return "", nil // already exists; do not print a password
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	pw, err := randomPassword(24)
	if err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), opts.BcryptCost)
	if err != nil {
		return "", err
	}
	u := models.User{
		Username:     opts.AdminUsername,
		Email:        opts.AdminEmail,
		PasswordHash: string(hash),
		DisplayName:  "Administrator",
		Status:       "enabled",
	}
	if err := tx.Create(&u).Error; err != nil {
		return "", err
	}

	var adminRole models.Role
	if err := tx.WithContext(ctx).Where("code = ?", "admin").First(&adminRole).Error; err != nil {
		return "", err
	}
	if err := tx.Create(&models.UserRole{UserID: u.ID, RoleID: adminRole.ID}).Error; err != nil {
		return "", err
	}
	return pw, nil
}

func randomPassword(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	s := base64.RawURLEncoding.EncodeToString(buf)
	return strings.TrimRight(s, "="), nil
}
