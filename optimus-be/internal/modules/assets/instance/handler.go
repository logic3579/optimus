package instance

import (
	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

type Handler struct{ svc *Service }

func NewHandler(service *Service) *Handler { return &Handler{svc: service} }

func (h *Handler) Mount(group *gin.RouterGroup, requirePermission func(code string) gin.HandlerFunc) {
	read := group.Group("", requirePermission("assets:resource:read"))
	read.GET("/instances", h.List)
}

// List returns a page of discovered EC2 instances.
// @Summary  List EC2 instances
// @Tags     assets
// @Security BearerAuth
// @Produce  json
// @Param    account_id      query int    false "filter by cloud account"
// @Param    region          query string false "filter by AWS region"
// @Param    state           query string false "filter by instance state"
// @Param    vpc_id          query string false "filter by VPC ID"
// @Param    q               query string false "search name, instance ID, private IP, or tag values"
// @Param    include_deleted query bool   false "include soft-deleted instances" default(false)
// @Param    page            query int    false "page" default(1)
// @Param    size            query int    false "page size" default(20) maximum(200)
// @Success  200 {object} response.Envelope{data=ListResponse}
// @Failure  400 {object} response.Envelope
// @Failure  500 {object} response.Envelope
// @Router   /assets/instances [get]
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
