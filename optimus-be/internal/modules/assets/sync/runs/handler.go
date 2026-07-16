package runs

import (
	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

type Handler struct{ svc *Service }

func NewHandler(service *Service) *Handler { return &Handler{svc: service} }

func (h *Handler) Mount(group *gin.RouterGroup, requirePermission func(code string) gin.HandlerFunc) {
	read := group.Group("", requirePermission("assets:sync:read"))
	read.GET("/sync-runs", h.List)
}

// List returns a page of asset synchronization runs.
// @Summary  List sync runs
// @Tags     assets
// @Security BearerAuth
// @Produce  json
// @Param    account_id    query int    false "filter by cloud account"
// @Param    resource_type query string false "instance|network|database"
// @Param    status        query string false "running|success|failed|skipped"
// @Param    started_after query string false "inclusive RFC3339 lower bound"
// @Param    page          query int    false "page" default(1)
// @Param    size          query int    false "page size" default(20) maximum(200)
// @Success  200 {object} response.Envelope{data=ListResponse}
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Failure  500 {object} response.Envelope
// @Router   /assets/sync-runs [get]
func (h *Handler) List(c *gin.Context) {
	var query ListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid query parameters"))
		return
	}
	result, err := h.svc.List(c.Request.Context(), query)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}
