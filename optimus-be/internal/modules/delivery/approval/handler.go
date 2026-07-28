package approval

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
	ListPending(context.Context, uint64) ([]PendingApproval, error)
	Approve(context.Context, uint64, string, string, uint64, DecisionRequest) (*Decision, error)
	Reject(context.Context, uint64, string, string, uint64, DecisionRequest) (*Decision, error)
}
type Handler struct{ svc handlerService }

func NewHandler(svc handlerService) *Handler { return &Handler{svc: svc} }
func (h *Handler) Mount(g *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	read := g.Group("", permission("delivery:approval:read"))
	read.GET("/approvals/pending", h.ListPending)
	decide := g.Group("", permission("delivery:approval:decide"))
	decide.POST("/run-stages/:id/approve", h.Approve)
	decide.POST("/run-stages/:id/reject", h.Reject)
}
func stageID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid stage id"))
		return 0, false
	}
	return id, true
}
func actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }

// ListPending godoc
// @Summary List actionable delivery approvals
// @Tags delivery
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=[]PendingApproval}
// @Router /delivery/approvals/pending [get]
func (h *Handler) ListPending(c *gin.Context) {
	out, err := h.svc.ListPending(c.Request.Context(), actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// Approve godoc
// @Summary Approve a delivery run stage
// @Tags delivery
// @Security BearerAuth
// @Accept json
// @Param id path int true "run stage ID"
// @Param body body DecisionRequest true "decision comment"
// @Success 200 {object} response.Envelope{data=Decision}
// @Router /delivery/run-stages/{id}/approve [post]
func (h *Handler) Approve(c *gin.Context) { h.decide(c, true) }

// Reject godoc
// @Summary Reject a delivery run stage
// @Tags delivery
// @Security BearerAuth
// @Accept json
// @Param id path int true "run stage ID"
// @Param body body DecisionRequest true "decision comment"
// @Success 200 {object} response.Envelope{data=Decision}
// @Router /delivery/run-stages/{id}/reject [post]
func (h *Handler) Reject(c *gin.Context) { h.decide(c, false) }
func (h *Handler) decide(c *gin.Context, approve bool) {
	id, ok := stageID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	var req DecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid request"))
		return
	}
	var out *Decision
	var err error
	if approve {
		out, err = h.svc.Approve(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id, req)
	} else {
		out, err = h.svc.Reject(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id, req)
	}
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}
