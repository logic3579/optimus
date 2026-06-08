package audit

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/pagination"
	"optimus-be/internal/infra/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("", h.list)
}

// HandleList is the public wrapper used by main.go to mount the list handler
// under a group gated by middleware.RequirePermission.
func (h *Handler) HandleList() gin.HandlerFunc { return h.list }

// list returns paginated audit log entries with optional filters.
// @Summary  List audit logs
// @Tags     audit
// @Security BearerAuth
// @Produce  json
// @Param    page      query int    false "page (default 1)"
// @Param    page_size query int    false "page_size (default 20, max 100)"
// @Param    action    query string false "filter by action code"
// @Param    user_id   query int    false "filter by actor user ID"
// @Param    start     query string false "start time (RFC3339)"
// @Param    end       query string false "end time (RFC3339)"
// @Success  200 {object} response.Envelope
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Router   /audit-logs [get]
func (h *Handler) list(c *gin.Context) {
	p := pagination.Parse(c)
	q := ListQuery{Action: c.Query("action")}
	if v := c.Query("user_id"); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid user_id"))
			return
		}
		q.UserID = &id
	}
	if v := c.Query("start"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid start (RFC3339)"))
			return
		}
		q.Start = &t
	}
	if v := c.Query("end"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid end (RFC3339)"))
			return
		}
		q.End = &t
	}
	out, err := h.svc.List(c.Request.Context(), q, p)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}
