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

// Public wrappers used by main.go to register handlers under nested groups
// gated by middleware.RequirePermission. Each returns the corresponding
// private handler method so the middleware runs before the handler executes.
func (h *Handler) HandleList() gin.HandlerFunc        { return h.list }
func (h *Handler) HandleGet() gin.HandlerFunc         { return h.get }
func (h *Handler) HandleCreate() gin.HandlerFunc      { return h.create }
func (h *Handler) HandleUpdate() gin.HandlerFunc      { return h.update }
func (h *Handler) HandleDelete() gin.HandlerFunc      { return h.delete }
func (h *Handler) HandleSetRoles() gin.HandlerFunc    { return h.setRoles }
func (h *Handler) HandleSetStatus() gin.HandlerFunc   { return h.setStatus }
func (h *Handler) HandleSetPassword() gin.HandlerFunc { return h.setPassword }

func (h *Handler) actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }

func (h *Handler) parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}

// list returns a paginated list of users.
// @Summary  List users
// @Tags     users
// @Security BearerAuth
// @Produce  json
// @Param    page      query int    false "page (default 1)"
// @Param    page_size query int    false "page_size (default 20, max 100)"
// @Param    search    query string false "search by username/email"
// @Param    status    query string false "enabled | disabled"
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Router   /users [get]
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

// create creates a new user.
// @Summary  Create user
// @Tags     users
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body     CreateRequest true "user payload"
// @Success  200  {object} response.Envelope
// @Failure  400  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Failure  403  {object} response.Envelope
// @Failure  409  {object} response.Envelope
// @Router   /users [post]
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

// get returns a single user by ID.
// @Summary  Get user
// @Tags     users
// @Security BearerAuth
// @Produce  json
// @Param    id   path     int true "user ID"
// @Success  200  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Failure  403  {object} response.Envelope
// @Failure  404  {object} response.Envelope
// @Router   /users/{id} [get]
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

// update mutates user profile fields (does not change roles, status, or password).
// @Summary  Update user
// @Tags     users
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path     int           true "user ID"
// @Param    body body     UpdateRequest true "profile fields"
// @Success  200  {object} response.Envelope
// @Failure  400  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Failure  403  {object} response.Envelope
// @Failure  404  {object} response.Envelope
// @Router   /users/{id} [put]
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

// delete soft-deletes a user.
// @Summary  Delete user
// @Tags     users
// @Security BearerAuth
// @Produce  json
// @Param    id   path     int true "user ID"
// @Success  200  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Failure  403  {object} response.Envelope
// @Failure  404  {object} response.Envelope
// @Router   /users/{id} [delete]
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

// setRoles replaces a user's role bindings with the supplied set.
// @Summary  Set user roles
// @Tags     users
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path     int             true "user ID"
// @Param    body body     SetRolesRequest true "role IDs"
// @Success  200  {object} response.Envelope
// @Failure  400  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Failure  403  {object} response.Envelope
// @Failure  404  {object} response.Envelope
// @Router   /users/{id}/roles [put]
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

// setStatus toggles a user between enabled and disabled.
// @Summary  Set user status
// @Tags     users
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path     int              true "user ID"
// @Param    body body     SetStatusRequest true "status"
// @Success  200  {object} response.Envelope
// @Failure  400  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Failure  403  {object} response.Envelope
// @Failure  404  {object} response.Envelope
// @Router   /users/{id}/status [put]
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

// setPassword resets a user's password (admin action).
// @Summary  Set user password
// @Tags     users
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path     int                true "user ID"
// @Param    body body     SetPasswordRequest true "new password"
// @Success  200  {object} response.Envelope
// @Failure  400  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Failure  403  {object} response.Envelope
// @Failure  404  {object} response.Envelope
// @Router   /users/{id}/password [put]
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
