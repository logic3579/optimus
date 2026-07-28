package pipeline

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
	"optimus-be/internal/modules/delivery/errs"
)

const maxRequestBodyBytes int64 = 1 << 20

// ArtifactVersion is the safe deployable-version projection returned by P3.
// It intentionally contains no repository credential, values, or chart bytes.
type ArtifactVersion struct {
	ChartRepoID uint64 `json:"chart_repo_id"`
	ChartName   string `json:"chart_name"`
	Version     string `json:"version"`
}

type handlerService interface {
	GetCurrent(context.Context, uint64) (*Pipeline, error)
	Publish(context.Context, uint64, string, string, uint64, PublishRequest) (*Pipeline, error)
	ListArtifacts(context.Context, uint64) ([]ArtifactVersion, error)
}

type Handler struct{ svc handlerService }

func NewHandler(svc handlerService) *Handler { return &Handler{svc: svc} }

// VersionLister is the P3-facing, consumer-owned seam used by the artifact
// picker. Implementations return only safe version identities.
type VersionLister interface {
	ListVersions(context.Context, uint64, string) ([]ArtifactVersion, error)
}
type projectArtifactLister interface {
	ListArtifacts(context.Context, uint64) ([]ArtifactVersion, error)
}
type httpService struct {
	service   *Service
	repo      *Repo
	artifacts projectArtifactLister
}

func NewHTTPService(service *Service, repo *Repo, artifacts projectArtifactLister) handlerService {
	return &httpService{service: service, repo: repo, artifacts: artifacts}
}
func (s *httpService) GetCurrent(ctx context.Context, projectID uint64) (*Pipeline, error) {
	row, stages, err := s.repo.GetCurrent(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, apperr.New(errs.CodePipelineMissing, errs.KeyPipelineMissing, "delivery project has no current pipeline")
	}
	return pipelineFrom(row, stages), nil
}
func (s *httpService) Publish(ctx context.Context, a uint64, ip, ua string, p uint64, r PublishRequest) (*Pipeline, error) {
	return s.service.Publish(ctx, a, ip, ua, p, r)
}
func (s *httpService) ListArtifacts(ctx context.Context, projectID uint64) ([]ArtifactVersion, error) {
	if s.artifacts == nil {
		return nil, apperr.New(apperr.CodeInternal, "common.internal", "artifact catalog unavailable")
	}
	items, err := s.artifacts.ListArtifacts(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (h *Handler) Mount(g *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	read := g.Group("", permission("delivery:pipeline:read"))
	read.GET("/projects/:id/pipeline", h.Get)
	write := g.Group("", permission("delivery:pipeline:write"))
	write.PUT("/projects/:id/pipeline", h.Publish)
	artifacts := g.Group("", permission("delivery:run:create"))
	artifacts.GET("/projects/:id/artifacts", h.ListArtifacts)
}

func projectID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid project id"))
		return 0, false
	}
	return id, true
}

// Get godoc
// @Summary Get the current delivery pipeline
// @Tags delivery
// @Security BearerAuth
// @Param id path int true "project ID"
// @Success 200 {object} response.Envelope{data=Pipeline}
// @Router /delivery/projects/{id}/pipeline [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	out, err := h.svc.GetCurrent(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// Publish godoc
// @Summary Publish a new immutable delivery pipeline version
// @Tags delivery
// @Security BearerAuth
// @Accept json
// @Param id path int true "project ID"
// @Param body body PublishRequest true "pipeline"
// @Success 200 {object} response.Envelope{data=Pipeline}
// @Router /delivery/projects/{id}/pipeline [put]
func (h *Handler) Publish(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	var req PublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid request"))
		return
	}
	out, err := h.svc.Publish(c.Request.Context(), c.GetUint64(middleware.CtxKeyUserID), c.ClientIP(), c.Request.UserAgent(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// ListArtifacts godoc
// @Summary List deployable artifact versions for a delivery project
// @Tags delivery
// @Security BearerAuth
// @Param id path int true "project ID"
// @Success 200 {object} response.Envelope{data=[]ArtifactVersion}
// @Router /delivery/projects/{id}/artifacts [get]
func (h *Handler) ListArtifacts(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	out, err := h.svc.ListArtifacts(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}
