package permissions

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

type RegisterResult struct {
	Inserted int
	Updated  int
	Stale    []string // codes present in DB but not in registry
}

// Register upserts all permissions in `defs` into the DB and reports rows in DB
// that are no longer declared (stale). It does NOT delete stale rows — that
// must be a deliberate decision (e.g. a separate housekeeping command).
func Register(ctx context.Context, gdb *gorm.DB, defs []Permission) (*RegisterResult, error) {
	result := &RegisterResult{}

	existing := map[string]models.Permission{}
	var rows []models.Permission
	if err := gdb.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load permissions: %w", err)
	}
	for _, r := range rows {
		existing[r.Code] = r
	}

	declared := map[string]struct{}{}
	for _, d := range defs {
		declared[d.Code] = struct{}{}
		cur, ok := existing[d.Code]
		if !ok {
			row := models.Permission{
				Code: d.Code, Name: d.Name, Category: d.Category, Description: d.Description,
			}
			if err := gdb.WithContext(ctx).Create(&row).Error; err != nil {
				return nil, fmt.Errorf("insert %s: %w", d.Code, err)
			}
			result.Inserted++
			continue
		}
		if cur.Name != d.Name || cur.Category != d.Category || cur.Description != d.Description {
			if err := gdb.WithContext(ctx).Model(&models.Permission{}).
				Where("id = ?", cur.ID).
				Updates(map[string]any{
					"name":        d.Name,
					"category":    d.Category,
					"description": d.Description,
				}).Error; err != nil {
				return nil, fmt.Errorf("update %s: %w", d.Code, err)
			}
			result.Updated++
		}
	}

	for code := range existing {
		if _, ok := declared[code]; !ok {
			result.Stale = append(result.Stale, code)
		}
	}
	return result, nil
}
