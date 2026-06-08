package permission

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"optimus-be/internal/infra/response"
	"optimus-be/internal/models"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler { return &Handler{db: db} }

// Register attaches the read-only permission route to a group already mounted
// under /api/v1/permissions. Per-route RequirePermission middleware is applied
// by the caller in main.go.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("", h.list)
}

// HandleList is the public wrapper used by main.go to mount the list handler
// under a group gated by middleware.RequirePermission.
func (h *Handler) HandleList() gin.HandlerFunc { return h.list }

func (h *Handler) list(c *gin.Context) {
	var rows []models.Permission
	if err := h.db.WithContext(c.Request.Context()).Order("category ASC, code ASC").Find(&rows).Error; err != nil {
		response.Error(c, err)
		return
	}
	out := make([]Permission, 0, len(rows))
	for _, r := range rows {
		out = append(out, Permission{
			ID: r.ID, Code: r.Code, Name: r.Name, Category: r.Category, Description: r.Description,
		})
	}
	response.Success(c, out)
}
