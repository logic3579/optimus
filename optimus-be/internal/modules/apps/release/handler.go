package release

import (
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

// Handler exposes the apps/release HTTP surface: six endpoints hanging off
// /apps/applications/:id/release/...
type Handler struct {
	svc *Service
}

// NewHandler returns a Handler bound to the given Service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Public wrappers for main.go's nested permission groups. Each method maps
// 1:1 to a permission code (see internal/infra/permissions/codes.go):
//   - HandleStatus    -> apps:release:read
//   - HandleHistory   -> apps:release:read
//   - HandleInstall   -> apps:release:install
//   - HandleUpgrade   -> apps:release:upgrade
//   - HandleRollback  -> apps:release:rollback
//   - HandleUninstall -> apps:release:uninstall
func (h *Handler) HandleStatus() gin.HandlerFunc    { return h.status }
func (h *Handler) HandleHistory() gin.HandlerFunc   { return h.history }
func (h *Handler) HandleInstall() gin.HandlerFunc   { return h.install }
func (h *Handler) HandleUpgrade() gin.HandlerFunc   { return h.upgrade }
func (h *Handler) HandleRollback() gin.HandlerFunc  { return h.rollback }
func (h *Handler) HandleUninstall() gin.HandlerFunc { return h.uninstall }

func (h *Handler) actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }

// parseID parses the :id path param. Returns (id, true) on success or writes
// a 40001 BizError and returns (0, false) on failure.
func (h *Handler) parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}

// status returns the live helm status for the application's release.
// @Summary  Get release status
// @Tags     apps
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "application ID"
// @Success  200 {object} response.Envelope
// @Router   /apps/applications/{id}/release/status [get]
func (h *Handler) status(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	out, err := h.svc.Status(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// history returns helm history for the application's release.
// @Summary  Get release history
// @Tags     apps
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "application ID"
// @Success  200 {object} response.Envelope
// @Router   /apps/applications/{id}/release/history [get]
func (h *Handler) history(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	out, err := h.svc.History(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"items": out})
}

// install runs helm install for the application's release.
// @Summary  Install release
// @Tags     apps
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path int             true "application ID"
// @Param    body body InstallRequest  true "install payload"
// @Success  200 {object} response.Envelope
// @Router   /apps/applications/{id}/release/install [post]
func (h *Handler) install(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req InstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	out, err := h.svc.Install(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// upgrade runs helm upgrade. If body.chart_repo_id is set, the application's
// chart_repo_id is repointed atomically before the upgrade.
// @Summary  Upgrade release
// @Tags     apps
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path int             true "application ID"
// @Param    body body UpgradeRequest  true "upgrade payload"
// @Success  200 {object} response.Envelope
// @Router   /apps/applications/{id}/release/upgrade [post]
func (h *Handler) upgrade(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req UpgradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	out, err := h.svc.Upgrade(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// rollback runs helm rollback to the body-specified revision.
// @Summary  Rollback release
// @Tags     apps
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path int              true "application ID"
// @Param    body body RollbackRequest  true "rollback payload"
// @Success  200 {object} response.Envelope
// @Router   /apps/applications/{id}/release/rollback [post]
func (h *Handler) rollback(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req RollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	out, err := h.svc.Rollback(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// uninstall runs helm uninstall. Does NOT delete the Optimus application row.
// @Summary  Uninstall release
// @Tags     apps
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path int               true "application ID"
// @Param    body body UninstallRequest  false "uninstall payload"
// @Success  200 {object} response.Envelope
// @Router   /apps/applications/{id}/release/uninstall [post]
func (h *Handler) uninstall(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req UninstallRequest
	// Empty body is OK: keep_history defaults to false.
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
			return
		}
	}
	if err := h.svc.Uninstall(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id, req); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
