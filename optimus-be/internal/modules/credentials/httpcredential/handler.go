package httpcredential

import (
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler             { return &Handler{s} }
func (h *Handler) HandleList() gin.HandlerFunc   { return h.list }
func (h *Handler) HandleGet() gin.HandlerFunc    { return h.get }
func (h *Handler) HandleCreate() gin.HandlerFunc { return h.create }
func (h *Handler) HandleUpdate() gin.HandlerFunc { return h.update }
func (h *Handler) HandleDelete() gin.HandlerFunc { return h.delete }
func (h *Handler) Mount(g *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	read := g.Group("", permission("credentials:http:read"))
	read.GET("", h.list)
	read.GET("/:id", h.get)
	write := g.Group("", permission("credentials:http:write"))
	write.POST("", h.create)
	write.PUT("/:id", h.update)
	del := g.Group("", permission("credentials:http:delete"))
	del.DELETE("/:id", h.delete)
}
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
// @Produce json
// @Param page query int false "page (default 1)"
// @Param page_size query int false "page size (default 20)"
// @Param q query string false "search by name"
// @Param auth_type query string false "filter by auth type (basic|bearer)"
// @Success 200 {object} response.Envelope{data=ListResponse}
// @Failure 400,401,403 {object} response.Envelope
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
// @Produce json
// @Param id path int true "credential ID"
// @Success 200 {object} response.Envelope{data=Detail}
// @Failure 400,401,403,404 {object} response.Envelope
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
// @Accept json
// @Produce json
// @Param body body CreateRequest true "credential payload"
// @Success 200 {object} response.Envelope{data=Detail}
// @Failure 400,401,403,409 {object} response.Envelope
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
// @Accept json
// @Produce json
// @Param id path int true "credential ID"
// @Param body body UpdateRequest true "credential payload"
// @Success 200 {object} response.Envelope{data=Detail}
// @Failure 400,401,403,404,409 {object} response.Envelope
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
// @Produce json
// @Param id path int true "credential ID"
// @Success 200 {object} response.Envelope
// @Failure 400,401,403,404,409 {object} response.Envelope
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
