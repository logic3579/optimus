package health

import (
	"log/slog"
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
	if err := db.Ping(c.Request.Context(), h.DB); err != nil {
		// Log internally but never expose error text to unauthenticated callers.
		slog.Error("health check db ping failed", "err", err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"db":      "down",
			"version": h.Version,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"db": "ok", "version": h.Version})
}
