package vpc

import (
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

type Handler struct{ svc *Service }

func NewHandler(service *Service) *Handler { return &Handler{svc: service} }

func (h *Handler) Mount(group *gin.RouterGroup, requirePermission func(code string) gin.HandlerFunc) {
	read := group.Group("", requirePermission("assets:resource:read"))
	read.GET("/vpcs", h.List)
	read.GET("/vpcs/:id/subnets", h.ListSubnets)
}

// List returns a page of VPC snapshots.
// @Summary  List VPCs
// @Tags     assets
// @Security BearerAuth
// @Produce  json
// @Param    account_id      query int    false "filter by cloud account"
// @Param    region          query string false "filter by AWS region"
// @Param    q               query string false "match VPC name or AWS VPC ID"
// @Param    include_deleted query bool   false "include soft-deleted snapshots"
// @Param    page            query int    false "page" default(1)
// @Param    size            query int    false "page size" default(20) maximum(200)
// @Success  200 {object} response.Envelope{data=ListResponse}
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Failure  500 {object} response.Envelope
// @Router   /assets/vpcs [get]
func (h *Handler) List(c *gin.Context) {
	var query ListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(c, validationError("invalid query parameters"))
		return
	}
	result, err := h.svc.List(c.Request.Context(), query)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

// ListSubnets returns subnets belonging to one live VPC snapshot row.
// @Summary  List VPC subnets
// @Tags     assets
// @Security BearerAuth
// @Produce  json
// @Param    id              path  int    true  "VPC snapshot row ID"
// @Param    q               query string false "match subnet name or AWS subnet ID"
// @Param    include_deleted query bool   false "include soft-deleted snapshots"
// @Param    page            query int    false "page" default(1)
// @Param    size            query int    false "page size" default(20) maximum(200)
// @Success  200 {object} response.Envelope{data=SubnetListResponse}
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Failure  404 {object} response.Envelope
// @Failure  500 {object} response.Envelope
// @Router   /assets/vpcs/{id}/subnets [get]
func (h *Handler) ListSubnets(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid VPC id"))
		return
	}
	var query SubnetListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(c, validationError("invalid query parameters"))
		return
	}
	result, err := h.svc.ListSubnetsByVPCRowID(c.Request.Context(), id, query)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}
