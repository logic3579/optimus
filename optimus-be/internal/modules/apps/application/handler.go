package application

import (
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

// safeTagPattern restricts the `tag` query parameter to a charset that's safe
// for the JSONB containment filter the repo builds via string concatenation.
// Tags themselves can contain any chars on Create/Update — this only applies
// to the search filter at list time.
var safeTagPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)

// Handler exposes the apps/application HTTP surface.
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

// list returns a paginated set of applications.
// @Summary  List applications
// @Tags     apps
// @Security BearerAuth
// @Produce  json
// @Param    page          query int    false "page (default 1)"
// @Param    page_size     query int    false "page size (default 20)"
// @Param    name          query string false "name substring (ILIKE)"
// @Param    cluster_id    query int    false "filter by cluster id"
// @Param    namespace     query string false "filter by namespace"
// @Param    owner_user_id query int    false "filter by owner user id"
// @Param    tag           query string false "tag containment filter (charset [a-zA-Z0-9_.-])"
// @Success  200 {object} response.Envelope
// @Router   /apps/applications [get]
func (h *Handler) list(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	if q.Tag != "" && !safeTagPattern.MatchString(q.Tag) {
		response.Error(c, apperr.New(apperr.CodeValidation, "apps.application.tag_filter_charset",
			"tag filter accepts only [a-zA-Z0-9_.-] characters"))
		return
	}
	out, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// get returns one application, decorated with live helm status if the probe
// seam is wired.
// @Summary  Get application
// @Tags     apps
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "application ID"
// @Success  200 {object} response.Envelope
// @Router   /apps/applications/{id} [get]
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

// create registers a new application. Helm is NOT installed by this endpoint;
// the application row is the durable Optimus record that release.Install
// later associates with a live helm release.
// @Summary  Create application
// @Tags     apps
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body CreateRequest true "application payload"
// @Success  200 {object} response.Envelope
// @Router   /apps/applications [post]
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

// update mutates description / tags / owner_user_id only. Immutable fields
// (name, cluster, namespace, release name, chart name) and chart_repo_id
// cannot be changed through this path — chart_repo_id is only patched as
// part of release.Upgrade.
// @Summary  Update application
// @Tags     apps
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path int           true "application ID"
// @Param    body body UpdateRequest true "application payload"
// @Success  200 {object} response.Envelope
// @Router   /apps/applications/{id} [put]
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

// delete removes the application row. Refused if the underlying helm release
// is still installed on the cluster (the wired-in HelmInstalledChecker is the
// source of truth).
// @Summary  Delete application
// @Tags     apps
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "application ID"
// @Success  200 {object} response.Envelope
// @Router   /apps/applications/{id} [delete]
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
