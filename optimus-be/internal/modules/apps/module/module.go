// Package module is the composition root for the P3 application lifecycle
// surface. It wires the chart-repo CRUD service, the application CRUD
// service, and the helm-driven release service into a single Module value
// with one MountRoutes entrypoint.
//
// This package sits alongside (not inside) the root `apps` package because
// every apps sub-package (repo, application, release, helmclient) already
// imports the root for MapError; pulling release.Handler in from the root
// would create an import cycle.
//
// Cross-package seams (apps/repo.InUseCounter, application.HelmStatusProbe,
// application.HelmInstalledChecker, k8s/cluster.AppsApplicationCounter) are
// wired post-construction in cmd/server/main.go to keep this package free
// of cycles and free of any direct k8s import.
package module

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"

	"helm.sh/helm/v3/pkg/chart"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps/application"
	"optimus-be/internal/modules/apps/release"
	apprepo "optimus-be/internal/modules/apps/repo"
	"optimus-be/internal/modules/delivery/orchestrator"
	"optimus-be/internal/modules/delivery/pipeline"
	"optimus-be/internal/modules/delivery/project"
	"optimus-be/internal/modules/delivery/run"
	"optimus-be/internal/modules/rbac"
)

// Module bundles every apps sub-service + handler so cmd/server/main.go only
// needs to call New + MountRoutes. The Repo/Application/Release fields stay
// exported so main.go (or tests) can finish post-construction wiring.
type Module struct {
	Repo        *apprepo.Service
	Application *application.Service
	Release     *release.Service

	repoH    *apprepo.Handler
	appH     *application.Handler
	releaseH *release.Handler
}

type deliveryGovernanceReader interface {
	IsManaged(context.Context, uint64) (bool, error)
	ActiveOperationID(context.Context, uint64) (string, error)
}

// DeliveryAdapters are the only P3 capabilities exposed to delivery.
type DeliveryAdapters struct {
	ProjectApplications project.ApplicationReader
	RunApplications     run.ApplicationReader
	Artifacts           run.ArtifactResolver
	ArtifactVersions    pipeline.VersionLister
	Executor            orchestrator.Executor
	Inspector           orchestrator.Inspector
}

func (m *Module) DeliveryAdapters(coordinator *release.Coordinator) DeliveryAdapters {
	base := deliveryApplicationAdapter{applications: m.Application, releases: m.Release}
	return DeliveryAdapters{ProjectApplications: deliveryProjectApplicationAdapter{base}, RunApplications: deliveryRunApplicationAdapter{base}, Artifacts: deliveryArtifactAdapter{repo: m.Repo}, ArtifactVersions: deliveryVersionAdapter{repo: m.Repo}, Executor: deliveryExecutorAdapter{release: m.Release}, Inspector: deliveryInspectorAdapter{coordinator: coordinator, applications: m.Application, releases: m.Release}}
}

func (m *Module) SetDeliveryGovernance(reader deliveryGovernanceReader) {
	m.Release.SetGovernance(deliveryGovernanceAdapter{reader: reader})
}

type deliveryApplicationAdapter struct {
	applications *application.Service
	releases     *release.Service
}

func (a deliveryApplicationAdapter) application(ctx context.Context, id uint64) (*models.AppsApplication, bool, error) {
	row, err := a.applications.GetModel(ctx, id)
	if err != nil {
		return nil, false, err
	}
	installed, err := a.releases.IsReleaseInstalled(ctx, row)
	return row, installed, err
}

type deliveryProjectApplicationAdapter struct{ deliveryApplicationAdapter }

func (a deliveryProjectApplicationAdapter) GetApplication(ctx context.Context, id uint64) (*project.Application, error) {
	r, installed, err := a.application(ctx, id)
	if err != nil {
		return nil, err
	}
	return &project.Application{ID: r.ID, Name: r.Name, ChartRepoID: r.ChartRepoID, ChartName: r.ChartName, Installed: installed, ClusterID: r.ClusterID, Namespace: r.Namespace, ReleaseName: r.ReleaseName}, nil
}

type deliveryRunApplicationAdapter struct{ deliveryApplicationAdapter }

func (a deliveryRunApplicationAdapter) GetApplication(ctx context.Context, id uint64) (*run.Application, error) {
	r, installed, err := a.application(ctx, id)
	if err != nil {
		return nil, err
	}
	return &run.Application{ID: r.ID, ChartRepoID: r.ChartRepoID, ChartName: r.ChartName, Installed: installed, ClusterID: r.ClusterID, Namespace: r.Namespace, ReleaseName: r.ReleaseName}, nil
}

type deliveryArtifactAdapter struct{ repo *apprepo.Service }

func (a deliveryArtifactAdapter) ResolveArtifact(ctx context.Context, repoID uint64, name, version string) (*run.Artifact, error) {
	x, err := a.repo.ResolveArtifact(ctx, repoID, name, version)
	if err != nil {
		return nil, err
	}
	return &run.Artifact{RepoID: x.RepoID, ChartName: x.ChartName, Version: x.Version, Digest: x.Digest}, nil
}

type deliveryVersionAdapter struct{ repo *apprepo.Service }

func (a deliveryVersionAdapter) ListVersions(ctx context.Context, repoID uint64, name string) ([]pipeline.ArtifactVersion, error) {
	xs, err := a.repo.ListVersions(ctx, repoID, name)
	if err != nil {
		return nil, err
	}
	out := make([]pipeline.ArtifactVersion, 0, len(xs))
	for _, x := range xs {
		out = append(out, pipeline.ArtifactVersion{ChartRepoID: repoID, ChartName: name, Version: x.Version})
	}
	return out, nil
}

type deliveryExecutorAdapter struct{ release *release.Service }

func (a deliveryExecutorAdapter) UpgradeExisting(ctx context.Context, req orchestrator.UpgradeRequest) (orchestrator.UpgradeResult, error) {
	x, err := a.release.UpgradeForDelivery(ctx, release.DeliveryUpgradeRequest{ApplicationID: req.ApplicationID, OperationID: req.OperationID, RepoID: req.RepoID, ChartName: req.ChartName, ChartVersion: req.ChartVersion, Digest: req.Digest, InitiatorID: req.InitiatorID, Purpose: req.Purpose})
	if err != nil {
		return orchestrator.UpgradeResult{}, err
	}
	return orchestrator.UpgradeResult{Revision: int64(x.Revision), Digest: x.Digest}, nil
}

type deliveryOperationInspector interface {
	Inspect(context.Context, string) (*release.Operation, error)
}
type deliveryApplicationLookup interface {
	GetModel(context.Context, uint64) (*models.AppsApplication, error)
}
type deliveryReleaseInspector interface {
	InspectDeliveryRelease(context.Context, *models.AppsApplication) (release.DeliveryInspection, error)
}
type deliveryInspectorAdapter struct {
	coordinator  deliveryOperationInspector
	applications deliveryApplicationLookup
	releases     deliveryReleaseInspector
}

func (a deliveryInspectorAdapter) Inspect(ctx context.Context, applicationID uint64, operationID string) (orchestrator.Inspection, error) {
	x, err := a.coordinator.Inspect(ctx, operationID)
	if err != nil {
		return orchestrator.Inspection{}, err
	}
	if x.ApplicationID != applicationID {
		return orchestrator.Inspection{}, errors.New("operation application mismatch")
	}
	if x.State == models.AppsReleaseOperationSucceeded && x.ResultRevision != nil && x.ResultDigest != nil {
		return orchestrator.Inspection{Revision: *x.ResultRevision, Digest: *x.ResultDigest}, nil
	}
	app, err := a.applications.GetModel(ctx, applicationID)
	if err != nil {
		return orchestrator.Inspection{}, errors.New("application inspection unavailable")
	}
	evidence, err := a.releases.InspectDeliveryRelease(ctx, app)
	if err != nil {
		return orchestrator.Inspection{}, errors.New("release artifact inspection unavailable")
	}
	return orchestrator.Inspection{Revision: evidence.Revision, Digest: evidence.Digest, PreviousDigestProven: !x.CreatedAt.IsZero() && evidence.ObservedAt.Before(x.CreatedAt)}, nil
}

type deliveryGovernanceAdapter struct{ reader deliveryGovernanceReader }

func (a deliveryGovernanceAdapter) AuthorizeMutation(ctx context.Context, applicationID uint64, action release.MutationAction) error {
	managed, err := a.reader.IsManaged(ctx, applicationID)
	if err != nil {
		return err
	}
	if !managed || action == release.MutationActionRollback {
		return nil
	}
	operationID, err := a.reader.ActiveOperationID(ctx, applicationID)
	if err == nil && release.DeliveryUpgradeAuthorized(ctx, applicationID, operationID, action) {
		return nil
	}
	return apperr.New(apperr.CodeDeliveryApplicationUnavailable, "delivery.application.unavailable", "delivery-managed application requires an authorized operation")
}

// New wires the apps Module from already-constructed services. The caller is
// responsible for cross-service post-construction wiring (so this package
// never reaches back into main.go for the seam values).
func New(repo *apprepo.Service, app *application.Service, rel *release.Service) *Module {
	return &Module{
		Repo:        repo,
		Application: app,
		Release:     rel,
		repoH:       apprepo.NewHandler(repo),
		appH:        application.NewHandler(app),
		releaseH:    release.NewHandler(rel),
	}
}

// HelmChartLoader satisfies release.ChartLoader by delegating to the
// apps/repo.Service.LoadChart method. release.Service requires a non-nil
// ChartLoader at construction time, so main.go builds this adapter before
// calling release.NewService — there is no construction-order cycle since
// apps/repo has no reference to release.
type HelmChartLoader struct {
	Repo *apprepo.Service
}

// LoadChart fetches a chart tarball through apps/repo.Service and returns
// the parsed *chart.Chart. All errors are already normalised to apperr.BizError
// by apps/repo (which calls apps.MapError internally).
func (l *HelmChartLoader) LoadChart(ctx context.Context, repoID uint64, chartName, version string) (*chart.Chart, error) {
	return l.Repo.LoadChart(ctx, repoID, chartName, version)
}

// LoadVerifiedChart preserves the immutable P6 artifact boundary by delegating
// download, digest verification, parsing, and byte wiping to apps/repo.
func (l *HelmChartLoader) LoadVerifiedChart(ctx context.Context, artifact apprepo.Artifact) (*chart.Chart, error) {
	return l.Repo.LoadVerifiedChart(ctx, artifact)
}

// MountRoutes registers every /apps route under `protected` (which must
// already be JWT-gated). Permission gating is per-route via nested sub-groups
// with middleware.RequirePermission — matching the pattern used by the k8s
// and credentials modules. See cmd/server/main.go's mountUserRoutes for the
// rationale (variadic args to GET/POST do NOT guarantee middleware ordering
// when handlers are registered separately; only Group("", mw) does).
func (m *Module) MountRoutes(protected *gin.RouterGroup, cache *rbac.PermissionCache) {
	g := protected.Group("/apps")

	// ---- chart repos ------------------------------------------------------
	repos := g.Group("/repos")
	repord := repos.Group("", middleware.RequirePermission(cache, "apps:repo:read"))
	repord.GET("", m.repoH.HandleList())
	repord.GET("/:id", m.repoH.HandleGet())
	repord.GET("/:id/charts", m.repoH.HandleListCharts())
	repord.GET("/:id/charts/:chart/versions", m.repoH.HandleListVersions())
	repord.GET("/:id/charts/:chart/versions/:version/values", m.repoH.HandleGetDefaultValues())

	repowr := repos.Group("", middleware.RequirePermission(cache, "apps:repo:write"))
	repowr.POST("", m.repoH.HandleCreate())
	repowr.PUT("/:id", m.repoH.HandleUpdate())

	repodel := repos.Group("", middleware.RequirePermission(cache, "apps:repo:delete"))
	repodel.DELETE("/:id", m.repoH.HandleDelete())

	// ---- applications -----------------------------------------------------
	apps := g.Group("/applications")
	apprd := apps.Group("", middleware.RequirePermission(cache, "apps:application:read"))
	apprd.GET("", m.appH.HandleList())
	apprd.GET("/:id", m.appH.HandleGet())

	appwr := apps.Group("", middleware.RequirePermission(cache, "apps:application:write"))
	appwr.POST("", m.appH.HandleCreate())
	appwr.PUT("/:id", m.appH.HandleUpdate())

	appdel := apps.Group("", middleware.RequirePermission(cache, "apps:application:delete"))
	appdel.DELETE("/:id", m.appH.HandleDelete())

	// ---- release lifecycle (hangs off /apps/applications/:id/release) -----
	rel := apps.Group("/:id/release")

	// status + history are gated by apps:application:read — they read live
	// helm state for an application the caller can already see. The 10 P3
	// permission codes (3 app + 4 release verbs + 3 repo) deliberately have
	// no dedicated "release:read" code; that would force every viewer to
	// receive a second grant just to see what /apps/applications/:id already
	// surfaces via the HelmStatusProbe decorator.
	relrd := rel.Group("", middleware.RequirePermission(cache, "apps:application:read"))
	relrd.GET("/status", m.releaseH.HandleStatus())
	relrd.GET("/history", m.releaseH.HandleHistory())

	relinst := rel.Group("", middleware.RequirePermission(cache, "apps:release:install"))
	relinst.POST("/install", m.releaseH.HandleInstall())

	relup := rel.Group("", middleware.RequirePermission(cache, "apps:release:upgrade"))
	relup.POST("/upgrade", m.releaseH.HandleUpgrade())

	relrb := rel.Group("", middleware.RequirePermission(cache, "apps:release:rollback"))
	relrb.POST("/rollback", m.releaseH.HandleRollback())

	reluni := rel.Group("", middleware.RequirePermission(cache, "apps:release:uninstall"))
	reluni.POST("/uninstall", m.releaseH.HandleUninstall())
}
