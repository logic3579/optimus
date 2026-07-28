package project

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

const maxRequestBodyBytes int64 = 1 << 20

type handlerService interface {
	List(context.Context, ListQuery) (*ListResponse, error)
	Get(context.Context, uint64) (*ProjectDetail, error)
	ListEnvironments(context.Context, uint64) ([]Environment, error)
	CreateProject(context.Context, uint64, string, string, CreateProjectRequest) (*ProjectDetail, error)
	UpdateProject(context.Context, uint64, string, string, uint64, UpdateProjectRequest) (*ProjectDetail, error)
	DeleteProject(context.Context, uint64, string, string, uint64) error
	BindEnvironment(context.Context, uint64, string, string, uint64, BindEnvironmentRequest) (*Environment, error)
	UpdateEnvironment(context.Context, uint64, string, string, uint64, uint64, UpdateEnvironmentRequest) (*Environment, error)
	UnbindEnvironment(context.Context, uint64, string, string, uint64, uint64) error
}

type Handler struct{ svc handlerService }

func NewHandler(svc handlerService) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(g *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	read := g.Group("", permission("delivery:project:read"))
	read.GET("/projects", h.List)
	read.GET("/projects/:id", h.Get)
	read.GET("/projects/:id/environments", h.ListEnvironments)
	write := g.Group("", permission("delivery:project:write"))
	write.POST("/projects", h.Create)
	write.PUT("/projects/:id", h.Update)
	write.POST("/projects/:id/environments", h.BindEnvironment)
	write.PUT("/projects/:id/environments/:environmentId", h.UpdateEnvironment)
	write.DELETE("/projects/:id/environments/:environmentId", h.UnbindEnvironment)
	del := g.Group("", permission("delivery:project:delete"))
	del.DELETE("/projects/:id", h.Delete)
}

func parseID(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}

func bindJSON(c *gin.Context, dst any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	if err := c.ShouldBindJSON(dst); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid request"))
		return false
	}
	return true
}

func actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }

// List godoc
// @Summary List delivery projects
// @Tags delivery
// @Security BearerAuth
// @Param q query string false "name search"
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} response.Envelope{data=ListResponse}
// @Router /delivery/projects [get]
func (h *Handler) List(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid query"))
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
// @Summary Get a delivery project
// @Tags delivery
// @Security BearerAuth
// @Param id path int true "project ID"
// @Success 200 {object} response.Envelope{data=ProjectDetail}
// @Router /delivery/projects/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := parseID(c, "id")
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

// ListEnvironments godoc
// @Summary List delivery project environments
// @Tags delivery
// @Security BearerAuth
// @Param id path int true "project ID"
// @Success 200 {object} response.Envelope{data=[]Environment}
// @Router /delivery/projects/{id}/environments [get]
func (h *Handler) ListEnvironments(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	out, err := h.svc.ListEnvironments(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// Create godoc
// @Summary Create a delivery project
// @Tags delivery
// @Security BearerAuth
// @Accept json
// @Param body body CreateProjectRequest true "project"
// @Success 200 {object} response.Envelope{data=ProjectDetail}
// @Router /delivery/projects [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateProjectRequest
	if !bindJSON(c, &req) {
		return
	}
	out, err := h.svc.CreateProject(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// Update godoc
// @Summary Update a delivery project
// @Tags delivery
// @Security BearerAuth
// @Accept json
// @Param id path int true "project ID"
// @Param body body UpdateProjectRequest true "project"
// @Success 200 {object} response.Envelope{data=ProjectDetail}
// @Router /delivery/projects/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req UpdateProjectRequest
	if !bindJSON(c, &req) {
		return
	}
	out, err := h.svc.UpdateProject(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// Delete godoc
// @Summary Delete a delivery project
// @Tags delivery
// @Security BearerAuth
// @Param id path int true "project ID"
// @Success 200 {object} response.Envelope
// @Router /delivery/projects/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteProject(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{})
}

// BindEnvironment godoc
// @Summary Bind an application as a delivery environment
// @Tags delivery
// @Security BearerAuth
// @Accept json
// @Param id path int true "project ID"
// @Param body body BindEnvironmentRequest true "environment"
// @Success 200 {object} response.Envelope{data=Environment}
// @Router /delivery/projects/{id}/environments [post]
func (h *Handler) BindEnvironment(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req BindEnvironmentRequest
	if !bindJSON(c, &req) {
		return
	}
	out, err := h.svc.BindEnvironment(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// UpdateEnvironment godoc
// @Summary Update delivery environment metadata
// @Tags delivery
// @Security BearerAuth
// @Accept json
// @Param id path int true "project ID"
// @Param environmentId path int true "environment ID"
// @Param body body UpdateEnvironmentRequest true "environment"
// @Success 200 {object} response.Envelope{data=Environment}
// @Router /delivery/projects/{id}/environments/{environmentId} [put]
func (h *Handler) UpdateEnvironment(c *gin.Context) {
	pid, ok := parseID(c, "id")
	if !ok {
		return
	}
	eid, ok := parseID(c, "environmentId")
	if !ok {
		return
	}
	var req UpdateEnvironmentRequest
	if !bindJSON(c, &req) {
		return
	}
	out, err := h.svc.UpdateEnvironment(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), pid, eid, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// UnbindEnvironment godoc
// @Summary Unbind a delivery environment
// @Tags delivery
// @Security BearerAuth
// @Param id path int true "project ID"
// @Param environmentId path int true "environment ID"
// @Success 200 {object} response.Envelope
// @Router /delivery/projects/{id}/environments/{environmentId} [delete]
func (h *Handler) UnbindEnvironment(c *gin.Context) {
	pid, ok := parseID(c, "id")
	if !ok {
		return
	}
	eid, ok := parseID(c, "environmentId")
	if !ok {
		return
	}
	if err := h.svc.UnbindEnvironment(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), pid, eid); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{})
}
