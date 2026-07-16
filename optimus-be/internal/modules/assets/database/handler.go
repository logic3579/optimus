package database

import (
	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

type Handler struct{ svc *Service }

func NewHandler(service *Service) *Handler { return &Handler{svc: service} }

func (h *Handler) Mount(group *gin.RouterGroup, requirePermission func(code string) gin.HandlerFunc) {
	read := group.Group("", requirePermission("assets:resource:read"))
	read.GET("/databases", h.List)
}

// List returns a page of discovered RDS database instances.
// @Summary  List RDS databases
// @Tags     assets
// @Security BearerAuth
// @Produce  json
// @Param    account_id      query int    false "filter by cloud account"
// @Param    region          query string false "filter by AWS region"
// @Param    engine          query string false "filter by database engine"
// @Param    status          query string false "filter by database status"
// @Param    q               query string false "search database ID or endpoint"
// @Param    include_deleted query bool   false "include soft-deleted databases"
// @Param    page            query int    false "page" default(1)
// @Param    size            query int    false "page size" default(20) maximum(200)
// @Success  200 {object} response.Envelope{data=ListResponse}
// @Failure  400 {object} response.Envelope
// @Failure  500 {object} response.Envelope
// @Router   /assets/databases [get]
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
