package secret

import (
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

// Handler wires the secret Service into Gin. Route mounting lives in the k8s
// module file (Task 15) — this file only exposes the List/Get/Data
// HandlerFuncs. Data is mounted under a different permission than List/Get.
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

// List handles GET /k8s/clusters/:id/secrets. The optional ?namespace=
// query string narrows the listing; absent it lists across all namespaces.
// Values are not included in the response.
func (h *Handler) List() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseClusterID(c)
		if !ok {
			return
		}
		var q ListQuery
		_ = c.ShouldBindQuery(&q)
		out, err := h.svc.List(c.Request.Context(), id, q)
		if err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, out)
	}
}

// Get handles GET /k8s/clusters/:id/secrets/:ns/:name. Returns metadata and
// key names only — never values.
func (h *Handler) Get() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseClusterID(c)
		if !ok {
			return
		}
		out, err := h.svc.Get(c.Request.Context(), id, c.Param("ns"), c.Param("name"))
		if err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, out)
	}
}

// Data handles GET /k8s/clusters/:id/secrets/:ns/:name/data. Returns the
// base64-decoded payload; Task 15 wiring must gate this behind a separate
// k8s:secret:reveal permission distinct from the list/get k8s:secret:read.
func (h *Handler) Data() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseClusterID(c)
		if !ok {
			return
		}
		out, err := h.svc.Data(c.Request.Context(), id, c.Param("ns"), c.Param("name"))
		if err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, out)
	}
}
