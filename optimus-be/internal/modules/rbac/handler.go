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
	g.GET("/me/menus", h.getMyMenus)
	g.GET("/me/permissions", h.getMyPermissions)
}

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
