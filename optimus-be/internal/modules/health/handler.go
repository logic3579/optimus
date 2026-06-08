package health

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
)

type Handler struct {
	DB      *gorm.DB
	Version string
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/health", h.health)
}

func (h *Handler) health(c *gin.Context) {
	if err := db.Ping(context.Background(), h.DB); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"db":      "down",
			"version": h.Version,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"db": "ok", "version": h.Version})
}
