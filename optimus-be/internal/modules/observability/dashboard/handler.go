package dashboard

import (
	"context"
	"github.com/gin-gonic/gin"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
	"strconv"
)

type serviceAPI interface {
	List(context.Context, ListQuery) (*ListResponse, error)
	Get(context.Context, uint64) (*Detail, error)
	Create(context.Context, uint64, string, string, SaveRequest) (*Detail, error)
	Update(context.Context, uint64, string, string, uint64, SaveRequest) (*Detail, error)
	Delete(context.Context, uint64, string, string, uint64) error
}
type Handler struct{ svc serviceAPI }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }
func (h *Handler) Mount(g *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	read := g.Group("", permission("observability:dashboard:read"))
	read.GET("/dashboards", h.List)
	read.GET("/dashboards/:id", h.Get)
	write := g.Group("", permission("observability:dashboard:write"))
	write.POST("/dashboards", h.Create)
	write.PUT("/dashboards/:id", h.Update)
	del := g.Group("", permission("observability:dashboard:delete"))
	del.DELETE("/dashboards/:id", h.Delete)
}
func dashID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 63)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}

// List godoc
// @Summary List custom metric dashboards
// @Tags observability
// @Security BearerAuth
// @Param q query string false "name search"
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} response.Envelope{data=ListResponse}
// @Router /observability/dashboards [get]
func (h *Handler) List(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, invalidPanel("invalid query"))
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
// @Summary Get a custom metric dashboard
// @Tags observability
// @Security BearerAuth
// @Param id path int true "dashboard ID"
// @Success 200 {object} response.Envelope{data=Detail}
// @Router /observability/dashboards/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := dashID(c)
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
// @Summary Create a custom metric dashboard
// @Tags observability
// @Security BearerAuth
// @Accept json
// @Param body body SaveRequest true "dashboard aggregate"
// @Success 200 {object} response.Envelope{data=Detail}
// @Router /observability/dashboards [post]
func (h *Handler) Create(c *gin.Context) {
	var req SaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, invalidPanel("invalid request"))
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
// @Summary Replace a custom metric dashboard
// @Tags observability
// @Security BearerAuth
// @Accept json
// @Param id path int true "dashboard ID"
// @Param body body SaveRequest true "dashboard aggregate"
// @Success 200 {object} response.Envelope{data=Detail}
// @Router /observability/dashboards/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, ok := dashID(c)
	if !ok {
		return
	}
	var req SaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, invalidPanel("invalid request"))
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
// @Summary Delete a custom metric dashboard
// @Tags observability
// @Security BearerAuth
// @Param id path int true "dashboard ID"
// @Success 200 {object} response.Envelope
// @Router /observability/dashboards/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := dashID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), c.GetUint64(middleware.CtxKeyUserID), c.ClientIP(), c.Request.UserAgent(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{})
}
