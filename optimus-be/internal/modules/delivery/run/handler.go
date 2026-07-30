package run

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/delivery/errs"
)

const maxRequestBodyBytes int64 = 1 << 20

type ListQuery struct {
	Page     int `form:"page,default=1" binding:"min=1"`
	PageSize int `form:"page_size,default=20" binding:"min=1,max=100"`
}
type ListResponse struct {
	Items    []RunView `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

// StageView is the client-safe stage projection. Lease ownership and other
// executor internals are intentionally absent.
type StageView struct {
	ID               uint64                    `json:"id"`
	EnvironmentID    uint64                    `json:"environment_id"`
	EnvironmentKey   string                    `json:"environment_key"`
	EnvironmentName  string                    `json:"environment_name"`
	ApplicationID    uint64                    `json:"application_id"`
	ClusterID        uint64                    `json:"cluster_id"`
	Namespace        string                    `json:"namespace"`
	ReleaseName      string                    `json:"release_name"`
	Order            int                       `json:"order"`
	Executor         models.DeliveryExecutor   `json:"executor"`
	ApprovalRequired bool                      `json:"approval_required"`
	Timeout          string                    `json:"timeout" example:"10m0s"`
	State            models.DeliveryStageState `json:"state"`
	ResultRevision   *int64                    `json:"result_revision,omitempty"`
	ResultDigest     *string                   `json:"result_digest,omitempty"`
	StartedAt        *time.Time                `json:"started_at,omitempty"`
	FinishedAt       *time.Time                `json:"finished_at,omitempty"`
	ErrorCode        *int                      `json:"error_code,omitempty"`
	ErrorMessageKey  *string                   `json:"error_message_key,omitempty"`
	CorrelationID    *string                   `json:"correlation_id,omitempty"`
}

// RunView omits the internal request fingerprint and exposes only stable,
// localized failure metadata suitable for recovery UI.
//
//nolint:revive // Explicit DTO name distinguishes it from the internal Run type.
type RunView struct {
	ID              uint64                  `json:"id"`
	ProjectID       uint64                  `json:"project_id"`
	PipelineID      uint64                  `json:"pipeline_id"`
	PipelineVersion int                     `json:"pipeline_version"`
	ChartRepoID     uint64                  `json:"chart_repo_id"`
	ChartName       string                  `json:"chart_name"`
	ChartVersion    string                  `json:"chart_version"`
	ChartDigest     string                  `json:"chart_digest"`
	InitiatorUserID uint64                  `json:"initiator_user_id"`
	State           models.DeliveryRunState `json:"state"`
	RetryOfRunID    *uint64                 `json:"retry_of_run_id,omitempty"`
	StartedAt       *time.Time              `json:"started_at,omitempty"`
	FinishedAt      *time.Time              `json:"finished_at,omitempty"`
	ErrorCode       *int                    `json:"error_code,omitempty"`
	ErrorMessageKey *string                 `json:"error_message_key,omitempty"`
	CorrelationID   *string                 `json:"correlation_id,omitempty"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
	Stages          []StageView             `json:"stages"`
}

type handlerService interface {
	List(context.Context, uint64, ListQuery) (*ListResponse, error)
	Get(context.Context, uint64) (*RunView, error)
	Create(context.Context, uint64, string, string, uint64, string, CreateRequest) (*Run, error)
	Cancel(context.Context, uint64, string, string, uint64) (*Run, error)
	RequestReconcile(context.Context, uint64, string, string, uint64) (*Run, error)
	Retry(context.Context, uint64, string, string, uint64, string) (*Run, error)
}
type Handler struct{ svc handlerService }

func NewHandler(svc handlerService) *Handler { return &Handler{svc: svc} }

type httpService struct {
	service *Service
	repo    *Repo
}

// NewHTTPService adds read-only HTTP queries to the orchestration service.
// It remains inside the run package and accesses only delivery-owned tables.
func NewHTTPService(service *Service, repo *Repo) handlerService {
	return &httpService{service: service, repo: repo}
}
func (s *httpService) Create(ctx context.Context, a uint64, ip, ua string, p uint64, k string, r CreateRequest) (*Run, error) {
	return s.service.Create(ctx, a, ip, ua, p, k, r)
}
func (s *httpService) Cancel(ctx context.Context, a uint64, ip, ua string, id uint64) (*Run, error) {
	return s.service.Cancel(ctx, a, ip, ua, id)
}
func (s *httpService) RequestReconcile(ctx context.Context, a uint64, ip, ua string, id uint64) (*Run, error) {
	return s.service.RequestReconcile(ctx, a, ip, ua, id)
}
func (s *httpService) Retry(ctx context.Context, a uint64, ip, ua string, id uint64, k string) (*Run, error) {
	return s.service.Retry(ctx, a, ip, ua, id, k)
}
func (s *httpService) Get(ctx context.Context, id uint64) (*RunView, error) {
	row, err := s.repo.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	stages, err := s.repo.ListRunStages(ctx, id)
	if err != nil {
		return nil, err
	}
	return runViewFromRows(row, stages), nil
}
func (s *httpService) List(ctx context.Context, projectID uint64, q ListQuery) (*ListResponse, error) {
	page, size := q.Page, q.PageSize
	if page == 0 {
		page = 1
	}
	if size == 0 {
		size = 20
	}
	if page < 1 || size < 1 || size > 100 || page-1 > math.MaxInt/size {
		return nil, apperr.New(apperr.CodeValidation, "common.validation", "invalid pagination")
	}
	var projects int64
	if err := s.repo.db.WithContext(ctx).Model(&models.DeliveryProject{}).Where("id = ?", projectID).Count(&projects).Error; err != nil {
		return nil, err
	}
	if projects != 1 {
		return nil, apperr.New(errs.CodeProjectNotFound, errs.KeyProjectNotFound, "delivery project not found")
	}
	db := s.repo.db.WithContext(ctx).Model(&models.DeliveryRun{}).Where("project_id = ?", projectID)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []models.DeliveryRun
	if err := db.Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]RunView, 0, len(rows))
	for i := range rows {
		stages, err := s.repo.ListRunStages(ctx, rows[i].ID)
		if err != nil {
			return nil, err
		}
		items = append(items, *runViewFromRows(&rows[i], stages))
	}
	return &ListResponse{Items: items, Total: total, Page: page, PageSize: size}, nil
}

func (h *Handler) Mount(g *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	read := g.Group("", permission("delivery:run:read"))
	read.GET("/projects/:id/runs", h.List)
	read.GET("/runs/:id", h.Get)
	create := g.Group("", permission("delivery:run:create"))
	create.POST("/projects/:id/runs", h.Create)
	create.POST("/runs/:id/reconcile", h.Reconcile)
	create.POST("/runs/:id/retry", h.Retry)
	cancel := g.Group("", permission("delivery:run:cancel"))
	cancel.POST("/runs/:id/cancel", h.Cancel)
}
func parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}
func actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }
func key(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		response.Error(c, apperr.New(errs.CodeIdempotencyMissing, errs.KeyIdempotencyMissing, "Idempotency-Key is required"))
		return "", false
	}
	if len(key) > 128 {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid Idempotency-Key"))
		return "", false
	}
	return key, true
}

// List godoc
// @Summary List delivery runs for a project
// @Tags delivery
// @Security BearerAuth
// @Param id path int true "project ID"
// @Param page query int false "page" default(1)
// @Param page_size query int false "page size" default(20)
// @Success 200 {object} response.Envelope{data=ListResponse}
// @Router /delivery/projects/{id}/runs [get]
func (h *Handler) List(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid query"))
		return
	}
	out, err := h.svc.List(c.Request.Context(), id, q)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// Get godoc
// @Summary Get delivery run detail
// @Tags delivery
// @Security BearerAuth
// @Param id path int true "run ID"
// @Success 200 {object} response.Envelope{data=RunView}
// @Router /delivery/runs/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := parseID(c)
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

// Create godoc
// @Summary Create an idempotent delivery run
// @Tags delivery
// @Security BearerAuth
// @Accept json
// @Param id path int true "project ID"
// @Param Idempotency-Key header string true "idempotency key"
// @Param body body CreateRequest true "artifact"
// @Success 200 {object} response.Envelope{data=RunView}
// @Router /delivery/projects/{id}/runs [post]
func (h *Handler) Create(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	k, ok := key(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid request"))
		return
	}
	out, err := h.svc.Create(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id, k, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, runViewFromRun(out))
}

// Cancel godoc
// @Summary Request delivery run cancellation
// @Tags delivery
// @Security BearerAuth
// @Param id path int true "run ID"
// @Success 200 {object} response.Envelope{data=RunView}
// @Router /delivery/runs/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	out, err := h.svc.Cancel(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, runViewFromRun(out))
}

// Reconcile godoc
// @Summary Request safe delivery run reconciliation
// @Tags delivery
// @Security BearerAuth
// @Param id path int true "run ID"
// @Success 200 {object} response.Envelope{data=RunView}
// @Router /delivery/runs/{id}/reconcile [post]
func (h *Handler) Reconcile(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	out, err := h.svc.RequestReconcile(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, runViewFromRun(out))
}

// Retry godoc
// @Summary Create a linked retry delivery run
// @Tags delivery
// @Security BearerAuth
// @Param id path int true "run ID"
// @Param Idempotency-Key header string true "idempotency key"
// @Success 200 {object} response.Envelope{data=RunView}
// @Router /delivery/runs/{id}/retry [post]
func (h *Handler) Retry(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	k, ok := key(c)
	if !ok {
		return
	}
	out, err := h.svc.Retry(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id, k)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, runViewFromRun(out))
}

func runViewFromRun(row *Run) *RunView {
	if row == nil {
		return nil
	}
	view := &RunView{ID: row.ID, ProjectID: row.ProjectID, PipelineID: row.PipelineID, PipelineVersion: row.PipelineVersion, ChartRepoID: row.ChartRepoID, ChartName: row.ChartName, ChartVersion: row.ChartVersion, ChartDigest: row.ChartDigest, InitiatorUserID: row.InitiatorUserID, State: row.State, RetryOfRunID: row.RetryOfRunID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Stages: make([]StageView, len(row.Stages))}
	for i := range row.Stages {
		stage := row.Stages[i]
		view.Stages[i] = StageView{ID: stage.ID, EnvironmentID: stage.EnvironmentID, EnvironmentKey: stage.EnvironmentKey,
			EnvironmentName: stage.EnvironmentName, ApplicationID: stage.ApplicationID, ClusterID: stage.ClusterID,
			Namespace: stage.Namespace, ReleaseName: stage.ReleaseName, Order: stage.Order, Executor: stage.Executor,
			ApprovalRequired: stage.ApprovalRequired, Timeout: stage.Timeout.String(), State: stage.State}
	}
	return view
}

func runViewFromRows(row *models.DeliveryRun, stages []models.DeliveryRunStage) *RunView {
	view := runViewFromRun(runFrom(row, stages))
	if view == nil {
		return nil
	}
	view.StartedAt, view.FinishedAt, view.ErrorCode, view.ErrorMessageKey, view.CorrelationID = row.StartedAt, row.FinishedAt, row.ErrorCode, row.ErrorMessageKey, row.CorrelationID
	for i := range stages {
		view.Stages[i].ResultRevision = stages[i].ResultRevision
		view.Stages[i].ResultDigest = stages[i].ResultDigest
		view.Stages[i].StartedAt = stages[i].StartedAt
		view.Stages[i].FinishedAt = stages[i].FinishedAt
		view.Stages[i].ErrorCode = stages[i].ErrorCode
		view.Stages[i].ErrorMessageKey = stages[i].ErrorMessageKey
		view.Stages[i].CorrelationID = stages[i].CorrelationID
	}
	return view
}
