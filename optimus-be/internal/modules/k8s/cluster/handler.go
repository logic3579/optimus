package cluster

import (
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
)

// safeTagPattern restricts the `tag` query parameter to a charset that's safe
// for the JSONB containment filter the repo builds via string concatenation.
// Tags themselves can contain any chars on Create/Update — this only applies
// to the search filter at list time.
var safeTagPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) HandleList() gin.HandlerFunc   { return h.list }
func (h *Handler) HandleGet() gin.HandlerFunc    { return h.get }
func (h *Handler) HandleCreate() gin.HandlerFunc { return h.create }
func (h *Handler) HandleUpdate() gin.HandlerFunc { return h.update }
func (h *Handler) HandleDelete() gin.HandlerFunc { return h.delete }
func (h *Handler) HandlePing() gin.HandlerFunc   { return h.ping }

func (h *Handler) actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }

func (h *Handler) parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}

// list returns a paginated set of clusters.
// @Summary  List clusters
// @Tags     k8s
// @Security BearerAuth
// @Produce  json
// @Param    page          query int    false "page (default 1)"
// @Param    page_size     query int    false "page size (default 20)"
// @Param    search        query string false "name / description substring"
// @Param    tag           query string false "tag containment filter (charset [a-zA-Z0-9_.-])"
// @Param    kubeconfig_id query int    false "filter by kubeconfig id"
// @Success  200 {object} response.Envelope
// @Router   /k8s/clusters [get]
func (h *Handler) list(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	if q.Tag != "" && !safeTagPattern.MatchString(q.Tag) {
		response.Error(c, apperr.New(apperr.CodeValidation, "k8s.cluster.tag_filter_charset",
			"tag filter accepts only [a-zA-Z0-9_.-] characters"))
		return
	}
	out, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// get returns one cluster.
// @Summary  Get cluster
// @Tags     k8s
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "cluster ID"
// @Success  200 {object} response.Envelope
// @Router   /k8s/clusters/{id} [get]
func (h *Handler) get(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// create stores a new cluster. The referenced kubeconfig is fetched through
// the Consumer seam and the requested context is validated.
// @Summary  Create cluster
// @Tags     k8s
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body CreateRequest true "cluster payload"
// @Success  200 {object} response.Envelope
// @Router   /k8s/clusters [post]
func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	out, err := h.svc.Create(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// update mutates cluster fields. If kubeconfig_id or context changes the
// referenced kubeconfig is re-validated.
// @Summary  Update cluster
// @Tags     k8s
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path int           true "cluster ID"
// @Param    body body UpdateRequest true "cluster payload"
// @Success  200 {object} response.Envelope
// @Router   /k8s/clusters/{id} [put]
func (h *Handler) update(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	out, err := h.svc.Update(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// delete removes the cluster. Audit row survives.
// @Summary  Delete cluster
// @Tags     k8s
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "cluster ID"
// @Success  200 {object} response.Envelope
// @Router   /k8s/clusters/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// ping probes the cluster apiserver and persists a best-effort health snapshot.
// @Summary  Ping cluster (apiserver health probe)
// @Tags     k8s
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "cluster ID"
// @Success  200 {object} response.Envelope
// @Router   /k8s/clusters/{id}/ping [post]
func (h *Handler) ping(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	out, err := h.svc.Ping(c.Request.Context(), h.actor(c), c.ClientIP(), c.Request.UserAgent(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}
