package sshkey

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

// Public wrappers used by main.go to register handlers under nested groups
// gated by middleware.RequirePermission.
func (h *Handler) HandleList() gin.HandlerFunc   { return h.list }
func (h *Handler) HandleGet() gin.HandlerFunc    { return h.get }
func (h *Handler) HandleCreate() gin.HandlerFunc { return h.create }
func (h *Handler) HandleUpdate() gin.HandlerFunc { return h.update }
func (h *Handler) HandleDelete() gin.HandlerFunc { return h.delete }

func (h *Handler) actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }

func (h *Handler) parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}

// list returns a paginated set of SSH credentials. Secret material is never
// included.
// @Summary  List SSH credentials
// @Tags     credentials
// @Security BearerAuth
// @Produce  json
// @Param    page      query    int    false "page (default 1)"
// @Param    page_size query    int    false "page size (default 20)"
// @Param    q         query    string false "search by name or description"
// @Param    username  query    string false "filter by username (exact)"
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Router   /credentials/ssh-keys [get]
func (h *Handler) list(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	out, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// get returns one SSH credential's metadata (no secrets).
// @Summary  Get SSH credential
// @Tags     credentials
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "credential ID"
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Failure  404 {object} response.Envelope
// @Router   /credentials/ssh-keys/{id} [get]
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

// create stores a new SSH credential. Secret material is sealed with the vault
// cipher before being persisted.
// @Summary  Create SSH credential
// @Tags     credentials
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body     CreateRequest true "credential payload"
// @Success  200 {object} response.Envelope
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Failure  409 {object} response.Envelope
// @Router   /credentials/ssh-keys [post]
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

// update mutates metadata and/or rotates secret material. Omitted fields are
// left as-is; a non-empty private_key value rotates the stored key (and emits
// a credentials.rotate audit row).
// @Summary  Update SSH credential
// @Tags     credentials
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path     int           true "credential ID"
// @Param    body body     UpdateRequest true "credential payload"
// @Success  200 {object} response.Envelope
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Failure  404 {object} response.Envelope
// @Failure  409 {object} response.Envelope
// @Router   /credentials/ssh-keys/{id} [put]
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

// delete removes the credential. Audit row survives.
// @Summary  Delete SSH credential
// @Tags     credentials
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "credential ID"
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Failure  404 {object} response.Envelope
// @Router   /credentials/ssh-keys/{id} [delete]
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
