package httpcredential

import (
	"github.com/gin-gonic/gin"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
	"strconv"
)

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler             { return &Handler{s} }
func (h *Handler) HandleList() gin.HandlerFunc   { return h.list }
func (h *Handler) HandleGet() gin.HandlerFunc    { return h.get }
func (h *Handler) HandleCreate() gin.HandlerFunc { return h.create }
func (h *Handler) HandleUpdate() gin.HandlerFunc { return h.update }
func (h *Handler) HandleDelete() gin.HandlerFunc { return h.delete }
func id(c *gin.Context) (uint64, bool) {
	v, e := strconv.ParseUint(c.Param("id"), 10, 64)
	if e != nil || v == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return v, true
}

// @Summary List HTTP credentials
// @Tags credentials
// @Security BearerAuth
// @Success 200 {object} response.Envelope
// @Router /credentials/http-credentials [get]
func (h *Handler) list(c *gin.Context) {
	var q ListQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", e.Error()))
		return
	}
	o, e := h.svc.List(c.Request.Context(), q)
	if e != nil {
		response.Error(c, e)
		return
	}
	response.Success(c, o)
}

// @Summary Get HTTP credential
// @Tags credentials
// @Security BearerAuth
// @Success 200 {object} response.Envelope
// @Router /credentials/http-credentials/{id} [get]
func (h *Handler) get(c *gin.Context) {
	v, ok := id(c)
	if !ok {
		return
	}
	o, e := h.svc.Get(c.Request.Context(), v)
	if e != nil {
		response.Error(c, e)
		return
	}
	response.Success(c, o)
}

// @Summary Create HTTP credential
// @Tags credentials
// @Security BearerAuth
// @Param body body CreateRequest true "credential payload"
// @Success 200 {object} response.Envelope
// @Router /credentials/http-credentials [post]
func (h *Handler) create(c *gin.Context) {
	var r CreateRequest
	if e := c.ShouldBindJSON(&r); e != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", e.Error()))
		return
	}
	o, e := h.svc.Create(c.Request.Context(), c.GetUint64(middleware.CtxKeyUserID), c.ClientIP(), c.Request.UserAgent(), r)
	if e != nil {
		response.Error(c, e)
		return
	}
	response.Success(c, o)
}

// @Summary Update HTTP credential
// @Tags credentials
// @Security BearerAuth
// @Param body body UpdateRequest true "credential payload"
// @Success 200 {object} response.Envelope
// @Router /credentials/http-credentials/{id} [put]
func (h *Handler) update(c *gin.Context) {
	v, ok := id(c)
	if !ok {
		return
	}
	var r UpdateRequest
	if e := c.ShouldBindJSON(&r); e != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", e.Error()))
		return
	}
	o, e := h.svc.Update(c.Request.Context(), c.GetUint64(middleware.CtxKeyUserID), c.ClientIP(), c.Request.UserAgent(), v, r)
	if e != nil {
		response.Error(c, e)
		return
	}
	response.Success(c, o)
}

// @Summary Delete HTTP credential
// @Tags credentials
// @Security BearerAuth
// @Success 200 {object} response.Envelope
// @Router /credentials/http-credentials/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	v, ok := id(c)
	if !ok {
		return
	}
	if e := h.svc.Delete(c.Request.Context(), c.GetUint64(middleware.CtxKeyUserID), c.ClientIP(), c.Request.UserAgent(), v); e != nil {
		response.Error(c, e)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
