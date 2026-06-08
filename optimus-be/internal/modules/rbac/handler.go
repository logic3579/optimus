package rbac

import (
	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

// ctxUserID is the gin context key set by JWTAuth middleware.
// Duplicated here to avoid an import cycle with the middleware package.
const ctxUserID = "x-user-id"

type Handler struct {
	svc *MeService
}

func NewHandler(svc *MeService) *Handler { return &Handler{svc: svc} }

// RegisterMe attaches the /me family of routes.
// The supplied group should already have JWTAuth middleware applied.
func (h *Handler) RegisterMe(g *gin.RouterGroup) {
	g.GET("/me", h.getMe)
	g.PUT("/me", h.updateMe)
	g.PUT("/me/password", h.changeMyPassword)
	g.GET("/me/menus", h.getMyMenus)
	g.GET("/me/permissions", h.getMyPermissions)
}

// getMe returns the authenticated user's profile.
// @Summary  Current user profile
// @Tags     me
// @Security BearerAuth
// @Produce  json
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  404 {object} response.Envelope
// @Router   /me [get]
func (h *Handler) getMe(c *gin.Context) {
	uid := c.GetUint64(ctxUserID)
	if uid == 0 {
		response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.unauthenticated", "authentication required"))
		return
	}
	dto, err := h.svc.GetUser(c.Request.Context(), uid)
	if err != nil {
		response.Error(c, apperr.Wrap(err, apperr.CodeNotFound, "common.not_found", "user not found"))
		return
	}
	response.Success(c, dto)
}

// updateMe applies the authenticated user's profile edits.
// @Summary  Update current user profile
// @Tags     me
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body UpdateMeRequest true "profile patch"
// @Success  200 {object} response.Envelope
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Router   /me [put]
func (h *Handler) updateMe(c *gin.Context) {
	uid := c.GetUint64(ctxUserID)
	if uid == 0 {
		response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.unauthenticated", "authentication required"))
		return
	}
	var req UpdateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	dto, err := h.svc.UpdateMe(c.Request.Context(), uid, c.ClientIP(), c.Request.UserAgent(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, dto)
}

// changeMyPassword changes the authenticated user's password (requires old).
// @Summary  Change current user password
// @Tags     me
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body ChangePasswordRequest true "old/new password"
// @Success  200 {object} response.Envelope
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Router   /me/password [put]
func (h *Handler) changeMyPassword(c *gin.Context) {
	uid := c.GetUint64(ctxUserID)
	if uid == 0 {
		response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.unauthenticated", "authentication required"))
		return
	}
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	if err := h.svc.ChangeMyPassword(c.Request.Context(), uid, c.ClientIP(), c.Request.UserAgent(), req.OldPassword, req.NewPassword); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, nil)
}

// getMyMenus returns the menu tree visible to the authenticated user.
// @Summary  Current user menu tree
// @Tags     me
// @Security BearerAuth
// @Produce  json
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Router   /me/menus [get]
func (h *Handler) getMyMenus(c *gin.Context) {
	uid := c.GetUint64(ctxUserID)
	if uid == 0 {
		response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.unauthenticated", "authentication required"))
		return
	}
	tree, err := h.svc.ListMenus(c.Request.Context(), uid)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, tree)
}

// getMyPermissions returns the flat list of permission codes the authenticated user holds.
// @Summary  Current user permission codes
// @Tags     me
// @Security BearerAuth
// @Produce  json
// @Success  200 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Router   /me/permissions [get]
func (h *Handler) getMyPermissions(c *gin.Context) {
	uid := c.GetUint64(ctxUserID)
	if uid == 0 {
		response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.unauthenticated", "authentication required"))
		return
	}
	codes, err := h.svc.ListPermissions(c.Request.Context(), uid)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, codes)
}
