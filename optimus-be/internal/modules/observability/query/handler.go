package query

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

type serviceAPI interface {
	Instant(context.Context, uint64, InstantRequest) (*BatchResult, error)
	Range(context.Context, uint64, RangeRequest) (*BatchResult, error)
	Labels(context.Context, uint64, uint64) ([]string, error)
	LabelValues(context.Context, uint64, uint64, string) ([]string, error)
}
type Source struct {
	ID        uint64  `json:"id"`
	Name      string  `json:"name"`
	ClusterID *uint64 `json:"cluster_id"`
}
type SourceLister interface {
	ListQuerySources(context.Context) ([]Source, error)
}
type SourceListerFunc func(context.Context) ([]Source, error)

func (f SourceListerFunc) ListQuerySources(ctx context.Context) ([]Source, error) { return f(ctx) }

type Handler struct {
	svc     serviceAPI
	sources SourceLister
}

func NewHandler(s serviceAPI, sources ...SourceLister) *Handler {
	h := &Handler{svc: s}
	if len(sources) > 0 {
		h.sources = sources[0]
	}
	return h
}
func (h *Handler) Mount(g *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	read := g.Group("", permission("observability:metric:read"))
	read.GET("/query-sources", h.QuerySources)
	read.POST("/query", h.Instant)
	read.POST("/query-range", h.Range)
	read.GET("/datasources/:id/labels", h.Labels)
	read.GET("/datasources/:id/label-values", h.LabelValues)
}

// QuerySources godoc
// @Summary List minimal data sources available for metric queries
// @Tags observability
// @Security BearerAuth
// @Success 200 {object} response.Envelope{data=[]Source}
// @Router /observability/query-sources [get]
func (h *Handler) QuerySources(c *gin.Context) {
	if h.sources == nil {
		response.Error(c, apperr.New(apperr.CodeInternal, "common.internal", "query source lister is unavailable"))
		return
	}
	out, err := h.sources.ListQuerySources(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}
func actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }
func id(c *gin.Context) (uint64, bool) {
	v, e := strconv.ParseUint(c.Param("id"), 10, 64)
	if e != nil || v == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return v, true
}

// Instant godoc
// @Summary Run an instant Prometheus query batch
// @Tags observability
// @Security BearerAuth
// @Accept json
// @Param body body InstantRequest true "query batch"
// @Success 200 {object} response.Envelope{data=BatchResult}
// @Router /observability/query [post]
func (h *Handler) Instant(c *gin.Context) {
	var req InstantRequest
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, invalid())
		return
	}
	v, e := h.svc.Instant(c.Request.Context(), actor(c), req)
	if e != nil {
		response.Error(c, e)
		return
	}
	response.Success(c, v)
}

// Range godoc
// @Summary Run a range Prometheus query batch
// @Tags observability
// @Security BearerAuth
// @Accept json
// @Param body body RangeRequest true "range query batch"
// @Success 200 {object} response.Envelope{data=BatchResult}
// @Router /observability/query-range [post]
func (h *Handler) Range(c *gin.Context) {
	var req RangeRequest
	if c.ShouldBindJSON(&req) != nil {
		response.Error(c, invalid())
		return
	}
	v, e := h.svc.Range(c.Request.Context(), actor(c), req)
	if e != nil {
		response.Error(c, e)
		return
	}
	response.Success(c, v)
}

// Labels godoc
// @Summary List Prometheus label names
// @Tags observability
// @Security BearerAuth
// @Param id path int true "data source ID"
// @Success 200 {object} response.Envelope{data=[]string}
// @Router /observability/datasources/{id}/labels [get]
func (h *Handler) Labels(c *gin.Context) {
	v, ok := id(c)
	if !ok {
		return
	}
	out, e := h.svc.Labels(c.Request.Context(), actor(c), v)
	if e != nil {
		response.Error(c, e)
		return
	}
	response.Success(c, out)
}

// LabelValues godoc
// @Summary List values for a Prometheus label
// @Tags observability
// @Security BearerAuth
// @Param id path int true "data source ID"
// @Param label query string true "label name"
// @Success 200 {object} response.Envelope{data=[]string}
// @Router /observability/datasources/{id}/label-values [get]
func (h *Handler) LabelValues(c *gin.Context) {
	v, ok := id(c)
	if !ok {
		return
	}
	out, e := h.svc.LabelValues(c.Request.Context(), actor(c), v, c.Query("label"))
	if e != nil {
		response.Error(c, e)
		return
	}
	response.Success(c, out)
}
