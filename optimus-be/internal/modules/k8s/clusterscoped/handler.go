package clusterscoped

import (
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

// Handler wires the clusterscoped Service into Gin. Route mounting lives in
// the k8s module file (Task 15) — this file only exposes the four
// HandlerFuncs.
type Handler struct{ svc *Service }

// NewHandler constructs a Handler bound to the given Service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// parseClusterID extracts the :id path param. Writes a BadRequest envelope
// and returns ok=false on a malformed value so the caller can early-return.
func parseClusterID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid cluster id"))
		return 0, false
	}
	return id, true
}

// ListNamespaces handles GET /k8s/clusters/:id/namespaces.
func (h *Handler) ListNamespaces() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseClusterID(c)
		if !ok {
			return
		}
		var q ListQuery
		_ = c.ShouldBindQuery(&q)
		out, err := h.svc.ListNamespaces(c.Request.Context(), id, q)
		if err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, out)
	}
}

// ListNodes handles GET /k8s/clusters/:id/nodes.
func (h *Handler) ListNodes() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseClusterID(c)
		if !ok {
			return
		}
		var q ListQuery
		_ = c.ShouldBindQuery(&q)
		out, err := h.svc.ListNodes(c.Request.Context(), id, q)
		if err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, out)
	}
}

// GetNode handles GET /k8s/clusters/:id/nodes/:name.
func (h *Handler) GetNode() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseClusterID(c)
		if !ok {
			return
		}
		out, err := h.svc.GetNode(c.Request.Context(), id, c.Param("name"))
		if err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, out)
	}
}

// ListEvents handles GET /k8s/clusters/:id/events.
func (h *Handler) ListEvents() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseClusterID(c)
		if !ok {
			return
		}
		var q ListQuery
		_ = c.ShouldBindQuery(&q)
		out, err := h.svc.ListEvents(c.Request.Context(), id, q)
		if err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, out)
	}
}
