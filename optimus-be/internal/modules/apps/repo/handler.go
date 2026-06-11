package repo

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

// Handler exposes the apps/repo HTTP surface.
type Handler struct {
	svc *Service
}

// NewHandler returns a Handler bound to the given Service.
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

// list returns a paginated set of chart repos. encrypted_password is never
// included; has_password is the only signal callers see.
// @Summary  List chart repos
// @Tags     apps
// @Security BearerAuth
// @Produce  json
// @Param    page      query int    false "page (default 1)"
// @Param    page_size query int    false "page size (default 20)"
// @Param    name      query string false "name substring (ILIKE)"
// @Param    type      query string false "filter by type (oci|http)"
// @Success  200 {object} response.Envelope
// @Router   /apps/repos [get]
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

// get returns one chart repo's metadata (no plaintext password).
// @Summary  Get chart repo
// @Tags     apps
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "chart repo ID"
// @Success  200 {object} response.Envelope
// @Router   /apps/repos/{id} [get]
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

// create registers a new chart repo. Password (if any) is sealed by the
// vault cipher before being persisted.
// @Summary  Create chart repo
// @Tags     apps
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body CreateRequest true "chart repo payload"
// @Success  200 {object} response.Envelope
// @Router   /apps/repos [post]
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

// update mutates chart-repo fields. Password tri-state:
//   - field absent / empty string -> keep current ciphertext.
//   - field explicit null         -> clear ciphertext (handler converts null
//     into a sentinel so the service sees the difference).
//   - field non-empty string      -> re-seal under the vault cipher.
//
// Type is silently ignored.
// @Summary  Update chart repo
// @Tags     apps
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path int           true "chart repo ID"
// @Param    body body UpdateRequest true "chart repo payload"
// @Success  200 {object} response.Envelope
// @Router   /apps/repos/{id} [put]
func (h *Handler) update(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	raw, _ := c.GetRawData()
	var req UpdateRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
			return
		}
	}
	if hasExplicitNull(raw, "password") {
		s := passwordClearSentinel
		req.Password = &s
	}
	out, err := h.svc.Update(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// delete removes a chart repo. Refused if any application still references it.
// @Summary  Delete chart repo
// @Tags     apps
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "chart repo ID"
// @Success  200 {object} response.Envelope
// @Router   /apps/repos/{id} [delete]
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

// hasExplicitNull reports whether the JSON object has the field set to
// literal null at the top level. Used by update() to distinguish
// "clear password" (null) from "keep password" (omitted / empty string).
func hasExplicitNull(raw []byte, field string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	v, ok := m[field]
	return ok && bytes.Equal(bytes.TrimSpace(v), []byte("null"))
}
