package module

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"optimus-be/internal/infra/config"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/delivery/approval"
	deliveryerrs "optimus-be/internal/modules/delivery/errs"
	"optimus-be/internal/modules/delivery/event"
	"optimus-be/internal/modules/delivery/pipeline"
	"optimus-be/internal/modules/delivery/project"
	"optimus-be/internal/modules/delivery/run"
	"optimus-be/internal/modules/rbac"
)

// Input contains only explicit cross-module seams. Delivery never reaches a
// P1/P3 private repository or table through this assembly boundary.
type Input struct {
	DB                  *gorm.DB
	Audit               *audit.Recorder
	Config              config.DeliveryConfig
	ProjectApplications project.ApplicationReader
	RunApplications     run.ApplicationReader
	Artifacts           run.ArtifactResolver
	ArtifactVersions    pipeline.VersionLister
	ApprovalPermissions approval.PermissionChecker
}
type Module struct {
	project  *project.Handler
	pipeline *pipeline.Handler
	run      *run.Handler
	approval *approval.Handler
	event    *event.Handler
}

func Wire(in Input) (*Module, error) {
	if nilValue(in.DB) || nilValue(in.Audit) || nilValue(in.ProjectApplications) || nilValue(in.RunApplications) || nilValue(in.Artifacts) || nilValue(in.ArtifactVersions) || nilValue(in.ApprovalPermissions) {
		return nil, errors.New("delivery database, audit, application, artifact, and permission seams are required")
	}
	if in.Config.MaxStageTimeout <= 0 || in.Config.SSEHeartbeat <= 0 || in.Config.SSEMaxConnections <= 0 {
		return nil, errors.New("invalid delivery HTTP limits")
	}
	projectRepo := project.NewRepo(in.DB)
	runRepo := run.NewRepo(in.DB)
	pipelineRepo := pipeline.NewRepo(in.DB)
	projectSvc := project.NewService(projectRepo, in.ProjectApplications, activityReader{db: in.DB}, in.Audit)
	pipelineSvc := pipeline.NewService(pipelineRepo, in.Audit, in.Config.MaxStageTimeout)
	runSvc := run.NewService(runRepo, in.RunApplications, in.Artifacts, in.Audit)
	approvalSvc := approval.NewService(approval.NewRepo(in.DB), in.ApprovalPermissions, in.Audit)
	eventSvc := event.NewService(event.NewRepo(in.DB))
	return &Module{project: project.NewHandler(projectSvc), pipeline: pipeline.NewHandler(pipeline.NewHTTPService(pipelineSvc, pipelineRepo, artifactCatalog{projects: projectSvc, versions: in.ArtifactVersions})), run: run.NewHandler(run.NewHTTPService(runSvc, runRepo)), approval: approval.NewHandler(approvalSvc), event: event.NewHandler(eventSvc, in.Config.SSEHeartbeat, in.Config.SSEMaxConnections)}, nil
}
func nilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Interface, reflect.Chan:
		return rv.IsNil()
	}
	return false
}
func (m *Module) MountRoutes(protected *gin.RouterGroup, cache *rbac.PermissionCache) {
	permission := func(code string) gin.HandlerFunc { return middleware.RequirePermission(cache, code) }
	m.mountRoutesWithPermission(protected, permission)
}
func (m *Module) mountRoutesWithPermission(protected *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	g := protected.Group("/delivery")
	m.project.Mount(g, permission)
	m.pipeline.Mount(g, permission)
	m.run.Mount(g, permission)
	m.approval.Mount(g, permission)
	m.event.Mount(g, permission)
}

type activityReader struct{ db *gorm.DB }

type artifactCatalog struct {
	projects interface {
		ListEnvironments(context.Context, uint64) ([]project.Environment, error)
	}
	versions pipeline.VersionLister
}

func (a artifactCatalog) ListArtifacts(ctx context.Context, projectID uint64) ([]pipeline.ArtifactVersion, error) {
	environments, err := a.projects.ListEnvironments(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if len(environments) == 0 {
		return []pipeline.ArtifactVersion{}, nil
	}
	items, err := a.versions.ListVersions(ctx, environments[0].ChartRepoID, environments[0].ChartName)
	if err != nil {
		return nil, apperr.Wrap(err, deliveryerrs.CodeExecutionUnavailable, deliveryerrs.KeyExecutionUnavailable, "artifact versions unavailable")
	}
	for i := range items {
		items[i].Version = strings.TrimSpace(items[i].Version)
		if items[i].Version == "" || len(items[i].Version) > 128 {
			return nil, apperr.New(deliveryerrs.CodeExecutionUnavailable, deliveryerrs.KeyExecutionUnavailable, "artifact versions unavailable")
		}
		items[i].ChartRepoID = environments[0].ChartRepoID
		items[i].ChartName = environments[0].ChartName
	}
	return items, nil
}

func (a activityReader) ProjectActivity(ctx context.Context, projectID uint64) (project.ProjectActivity, error) {
	var rows []models.DeliveryRunState
	err := a.db.WithContext(ctx).Model(&models.DeliveryRun{}).Where("project_id = ? AND state IN ?", projectID, []models.DeliveryRunState{models.DeliveryRunQueued, models.DeliveryRunRunning, models.DeliveryRunWaitingApproval, models.DeliveryRunCancelRequested, models.DeliveryRunReconciling, models.DeliveryRunOutcomeUnknown}).Pluck("state", &rows).Error
	if err != nil {
		return project.ProjectActivity{}, err
	}
	out := project.ProjectActivity{Active: len(rows) > 0}
	for _, state := range rows {
		if state == models.DeliveryRunOutcomeUnknown {
			out.OutcomeUnknown = true
		}
	}
	return out, nil
}
