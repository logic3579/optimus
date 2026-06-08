package role

import (
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register attaches routes to a group already mounted under /api/v1/roles.
// Per-route RequirePermission middleware is applied by the caller in main.go.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("", h.list)
	g.POST("", h.create)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.delete)
	g.PUT("/:id/permissions", h.setPermissions)
}

// Public wrappers used by main.go to register handlers under nested groups
// gated by middleware.RequirePermission.
func (h *Handler) HandleList() gin.HandlerFunc           { return h.list }
func (h *Handler) HandleGet() gin.HandlerFunc            { return h.get }
func (h *Handler) HandleCreate() gin.HandlerFunc         { return h.create }
func (h *Handler) HandleUpdate() gin.HandlerFunc         { return h.update }
func (h *Handler) HandleDelete() gin.HandlerFunc         { return h.delete }
func (h *Handler) HandleSetPermissions() gin.HandlerFunc { return h.setPermissions }

func (h *Handler) actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }

func (h *Handler) parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}

// list returns all roles with their bound permissions.
// @Summary  List roles
// @Tags     roles
// @Security BearerAuth
// @Produce  json
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Router   /roles [get]
func (h *Handler) list(c *gin.Context) {
	out, err := h.svc.List(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// get returns a single role by ID.
// @Summary  Get role
// @Tags     roles
// @Security BearerAuth
// @Produce  json
// @Param    id  path     int true "role ID"
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Failure  404 {object} response.Envelope
// @Router   /roles/{id} [get]
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

// create creates a new role (without permissions; bind via setPermissions).
// @Summary  Create role
// @Tags     roles
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body     CreateRequest true "role payload"
// @Success  200  {object} response.Envelope
// @Failure  400  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Failure  403  {object} response.Envelope
// @Failure  409  {object} response.Envelope
// @Router   /roles [post]
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

// update mutates role name/description fields.
// @Summary  Update role
// @Tags     roles
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path     int           true "role ID"
// @Param    body body     UpdateRequest true "role payload"
// @Success  200  {object} response.Envelope
// @Failure  400  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Failure  403  {object} response.Envelope
// @Failure  404  {object} response.Envelope
// @Router   /roles/{id} [put]
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

// delete removes a role (and its bindings).
// @Summary  Delete role
// @Tags     roles
// @Security BearerAuth
// @Produce  json
// @Param    id  path     int true "role ID"
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Failure  404 {object} response.Envelope
// @Router   /roles/{id} [delete]
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

// setPermissions replaces a role's permission bindings with the supplied set of codes.
// @Summary  Set role permissions
// @Tags     roles
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path     int                   true "role ID"
// @Param    body body     SetPermissionsRequest true "permission codes"
// @Success  200  {object} response.Envelope
// @Failure  400  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Failure  403  {object} response.Envelope
// @Failure  404  {object} response.Envelope
// @Router   /roles/{id}/permissions [put]
func (h *Handler) setPermissions(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req SetPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	if err := h.svc.SetPermissions(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id, req.PermissionCodes); err != nil {
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
