package auth

import (
	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register attaches routes to the supplied auth group (typically /api/v1/auth).
func (h *Handler) Register(g *gin.RouterGroup) {
	g.POST("/login", h.login)
	g.POST("/refresh", h.refresh)
	g.POST("/logout", h.logout)
}

// login authenticates a user and issues an access/refresh token pair.
// @Summary  Login
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body body     LoginRequest true "credentials"
// @Success  200  {object} response.Envelope
// @Failure  400  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Failure  429  {object} response.Envelope
// @Router   /auth/login [post]
func (h *Handler) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", err.Error()))
		return
	}
	pair, err := h.svc.Login(c.Request.Context(), req, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, pair)
}

// refresh exchanges a valid refresh token for a new token pair (rotation).
// @Summary  Refresh tokens
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body body     RefreshRequest true "refresh token"
// @Success  200  {object} response.Envelope
// @Failure  400  {object} response.Envelope
// @Failure  401  {object} response.Envelope
// @Router   /auth/refresh [post]
func (h *Handler) refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", err.Error()))
		return
	}
	pair, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, pair)
}

// logout revokes the supplied refresh token (best-effort; always 200).
// @Summary  Logout
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body body     LogoutRequest true "refresh token to revoke"
// @Success  200  {object} response.Envelope
// @Router   /auth/logout [post]
func (h *Handler) logout(c *gin.Context) {
	var req LogoutRequest
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
