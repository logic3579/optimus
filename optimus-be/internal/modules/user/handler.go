package user

import (
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/pagination"
	"optimus-be/internal/infra/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register attaches routes to a group already mounted under /api/v1/users.
// Per-route RequirePermission middleware is applied by the caller in main.go.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("", h.list)
	g.POST("", h.create)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.delete)
	g.PUT("/:id/roles", h.setRoles)
	g.PUT("/:id/status", h.setStatus)
	g.PUT("/:id/password", h.setPassword)
}

func (h *Handler) actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }

func (h *Handler) parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}

func (h *Handler) list(c *gin.Context) {
	p := pagination.Parse(c)
	q := ListQuery{Search: c.Query("search"), Status: c.Query("status")}
	page, err := h.svc.List(c.Request.Context(), q, p)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, page)
}

func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	out, err := h.svc.Create(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func (h *Handler) get(c *gin.Context) {
	id, ok := h.parseID(c)
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

func (h *Handler) update(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	out, err := h.svc.Update(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func (h *Handler) delete(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

func (h *Handler) setRoles(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req SetRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	if err := h.svc.SetRoles(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id, req.RoleIDs); err != nil {
		response.Error(c, err)
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func (h *Handler) setStatus(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req SetStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	if err := h.svc.SetStatus(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id, req.Status); err != nil {
		response.Error(c, err)
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

func (h *Handler) setPassword(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req SetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	if err := h.svc.SetPassword(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id, req.Password); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
