package network

import (
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

// Handler wires the network Service into Gin. Route mounting lives in the
// k8s module file (Task 15) — this file only exposes List/Get HandlerFuncs.
// Both forward the `kind` path parameter to the Service which dispatches
// internally; no per-kind handler glue lives here.
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

// List handles GET /k8s/clusters/:id/network/:kind. The kind path param
// selects the underlying typed clientset call — see Service.List.
func (h *Handler) List() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseClusterID(c)
		if !ok {
			return
		}
		var q ListQuery
		_ = c.ShouldBindQuery(&q)
		out, err := h.svc.List(c.Request.Context(), id, c.Param("kind"), q)
		if err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, out)
	}
}

// Get handles GET /k8s/clusters/:id/network/:kind/:ns/:name.
func (h *Handler) Get() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseClusterID(c)
		if !ok {
			return
		}
		out, err := h.svc.Get(c.Request.Context(), id, c.Param("kind"), c.Param("ns"), c.Param("name"))
		if err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, out)
	}
}
