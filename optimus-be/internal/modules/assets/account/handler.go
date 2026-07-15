package account

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

type Handler struct {
	svc         *Service
	triggerSync func(c *gin.Context, id uint64)
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) SetTriggerSync(trigger func(c *gin.Context, id uint64)) {
	h.triggerSync = trigger
}

func (h *Handler) Mount(group *gin.RouterGroup, requirePermission func(code string) gin.HandlerFunc) {
	read := group.Group("", requirePermission("assets:account:read"))
	{
		read.GET("/cloud-accounts", h.List)
		read.GET("/cloud-accounts/:id", h.Get)
	}
	write := group.Group("", requirePermission("assets:account:write"))
	{
		write.POST("/cloud-accounts", h.Create)
		write.PUT("/cloud-accounts/:id", h.Update)
		write.POST("/cloud-accounts/:id/sync", h.TriggerSync)
	}
	remove := group.Group("", requirePermission("assets:account:delete"))
	remove.DELETE("/cloud-accounts/:id", h.Delete)
}

func (h *Handler) parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 63)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}

// List returns a page of cloud accounts.
// @Summary  List cloud accounts
// @Tags     assets
// @Security BearerAuth
// @Produce  json
// @Param    q               query string false "search by name"
// @Param    provider        query string false "cloud provider"
// @Param    enabled         query bool   false "enabled filter"
// @Param    include_deleted query bool   false "include soft-deleted accounts"
// @Param    page            query int    false "page" default(1)
// @Param    size            query int    false "page size" default(20)
// @Success  200 {object} response.Envelope{data=ListResponse}
// @Router   /assets/cloud-accounts [get]
func (h *Handler) List(c *gin.Context) {
	var query ListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	result, err := h.svc.List(c.Request.Context(), query)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

// Get returns one cloud account.
// @Summary  Get cloud account
// @Tags     assets
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "cloud account ID"
// @Success  200 {object} response.Envelope{data=Detail}
// @Router   /assets/cloud-accounts/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	detail, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, detail)
}

// Create registers a cloud account.
// @Summary  Create cloud account
// @Tags     assets
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body CreateRequest true "cloud account payload"
// @Success  200 {object} response.Envelope{data=Detail}
// @Failure  422 {object} response.Envelope
// @Router   /assets/cloud-accounts [post]
func (h *Handler) Create(c *gin.Context) {
	var request CreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	detail, err := h.svc.Create(
		c.Request.Context(),
		c.GetUint64(middleware.CtxKeyUserID),
		c.ClientIP(),
		c.Request.UserAgent(),
		request,
	)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, detail)
}

// Update changes a cloud account.
// @Summary  Update cloud account
// @Tags     assets
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path int true "cloud account ID"
// @Param    body body UpdateRequest true "cloud account payload"
// @Success  200 {object} response.Envelope{data=Detail}
// @Failure  422 {object} response.Envelope
// @Router   /assets/cloud-accounts/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var request UpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	detail, err := h.svc.Update(
		c.Request.Context(),
		c.GetUint64(middleware.CtxKeyUserID),
		c.ClientIP(),
		c.Request.UserAgent(),
		id,
		request,
	)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, detail)
}

// Delete soft-deletes a cloud account and its discovered resources.
// @Summary  Delete cloud account
// @Tags     assets
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "cloud account ID"
// @Success  200 {object} response.Envelope
// @Router   /assets/cloud-accounts/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	cascaded, err := h.svc.Delete(
		c.Request.Context(),
		c.GetUint64(middleware.CtxKeyUserID),
		c.ClientIP(),
		c.Request.UserAgent(),
		id,
	)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"cascaded_resources_count": cascaded})
}

// TriggerSync starts a manual sync when the sync engine is wired.
// @Summary  Trigger cloud account sync
// @Tags     assets
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "cloud account ID"
// @Success  200 {object} response.Envelope
// @Failure  501 {object} response.Envelope
// @Router   /assets/cloud-accounts/{id}/sync [post]
func (h *Handler) TriggerSync(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	if h.triggerSync == nil {
		c.JSON(http.StatusNotImplemented, response.Envelope{
			Code:    int(apperr.CodeInternal),
			Data:    nil,
			Message: "not implemented",
		})
		return
	}
	h.triggerSync(c, id)
}
