package datasource

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

type serviceAPI interface {
	List(context.Context, ListQuery) (*ListResponse, error)
	Get(context.Context, uint64) (*Detail, error)
	Create(context.Context, uint64, string, string, CreateRequest) (*Detail, error)
	Update(context.Context, uint64, string, string, uint64, UpdateRequest) (*Detail, error)
	Delete(context.Context, uint64, string, string, uint64) error
	TestConnection(context.Context, uint64, string, string, uint64) (*TestResponse, error)
}
type Handler struct{ svc serviceAPI }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }
func (h *Handler) Mount(g *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	read := g.Group("", permission("observability:datasource:read"))
	read.GET("/datasources", h.List)
	read.GET("/datasources/:id", h.Get)
	test := g.Group("", permission("observability:datasource:test"))
	test.POST("/datasources/:id/test", h.Test)
	write := g.Group("", permission("observability:datasource:write"))
	write.POST("/datasources", h.Create)
	write.PUT("/datasources/:id", h.Update)
	del := g.Group("", permission("observability:datasource:delete"))
	del.DELETE("/datasources/:id", h.Delete)
}
func parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 63)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}

// List godoc
// @Summary List observability data sources
// @Tags observability
// @Security BearerAuth
// @Param q query string false "name search"
// @Param auth_type query string false "authentication type"
// @Param cluster_id query int false "cluster ID"
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} response.Envelope{data=ListResponse}
// @Router /observability/datasources [get]
func (h *Handler) List(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, validation("invalid query"))
		return
	}
	out, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// Get godoc
// @Summary Get observability data source
// @Tags observability
// @Security BearerAuth
// @Param id path int true "data source ID"
// @Success 200 {object} response.Envelope{data=Detail}
// @Router /observability/datasources/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// Create godoc
// @Summary Create observability data source
// @Tags observability
// @Security BearerAuth
// @Accept json
// @Param body body CreateRequest true "data source"
// @Success 200 {object} response.Envelope{data=Detail}
// @Router /observability/datasources [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, validation("invalid request"))
		return
	}
	out, err := h.svc.Create(c.Request.Context(), c.GetUint64(middleware.CtxKeyUserID), c.ClientIP(), c.Request.UserAgent(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// Update godoc
// @Summary Update observability data source
// @Tags observability
// @Security BearerAuth
// @Accept json
// @Param id path int true "data source ID"
// @Param body body UpdateRequest true "data source"
// @Success 200 {object} response.Envelope{data=Detail}
// @Router /observability/datasources/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, validation("invalid request"))
		return
	}
	out, err := h.svc.Update(c.Request.Context(), c.GetUint64(middleware.CtxKeyUserID), c.ClientIP(), c.Request.UserAgent(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// Delete godoc
// @Summary Delete observability data source
// @Tags observability
// @Security BearerAuth
// @Param id path int true "data source ID"
// @Success 200 {object} response.Envelope
// @Router /observability/datasources/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), c.GetUint64(middleware.CtxKeyUserID), c.ClientIP(), c.Request.UserAgent(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{})
}

// Test godoc
// @Summary Test observability data source
// @Tags observability
// @Security BearerAuth
// @Param id path int true "data source ID"
// @Success 200 {object} response.Envelope{data=TestResponse}
// @Router /observability/datasources/{id}/test [post]
func (h *Handler) Test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	out, err := h.svc.TestConnection(c.Request.Context(), c.GetUint64(middleware.CtxKeyUserID), c.ClientIP(), c.Request.UserAgent(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}
