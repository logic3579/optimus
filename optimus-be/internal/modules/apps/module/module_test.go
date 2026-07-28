//go:build dbtest

package module

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps/application"
	"optimus-be/internal/modules/apps/release"
	apprepo "optimus-be/internal/modules/apps/repo"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/delivery/orchestrator"
	"optimus-be/internal/modules/delivery/pipeline"
	"optimus-be/internal/modules/delivery/project"
	"optimus-be/internal/modules/delivery/run"
	"optimus-be/internal/modules/rbac"
)

// nopCipher is a never-called Cipher stub — the snapshot test exercises
// only route assembly, never reaches a handler that touches the cipher.
type nopCipher struct{}

func (nopCipher) Seal(b []byte) ([]byte, error) { return b, nil }
func (nopCipher) Open(b []byte) ([]byte, error) { return b, nil }

// nopFactory is a never-called release.Factory stub. release.NewService
// refuses a nil Factory at construction time, so we satisfy the interface
// without spinning up real helm wiring.
type nopFactory struct{}

func (nopFactory) NewForCluster(context.Context, uint64, string, string) (*action.Configuration, error) {
	return nil, nil
}

type operationInspectorStub struct {
	operation *release.Operation
	err       error
}

func (s operationInspectorStub) Inspect(context.Context, string) (*release.Operation, error) {
	return s.operation, s.err
}

type applicationLookupStub struct {
	application *models.AppsApplication
	err         error
}

func (s applicationLookupStub) GetModel(context.Context, uint64) (*models.AppsApplication, error) {
	return s.application, s.err
}

type releaseInspectorStub struct {
	evidence release.DeliveryInspection
	err      error
}

func (s releaseInspectorStub) InspectDeliveryRelease(context.Context, *models.AppsApplication) (release.DeliveryInspection, error) {
	return s.evidence, s.err
}

func TestDeliveryInspectorUsesOnlyCoordinatorOrLiveP3Evidence(t *testing.T) {
	now := time.Now().UTC()
	digest := "sha256:" + strings.Repeat("a", 64)
	app := &models.AppsApplication{ID: 7}
	t.Run("definite coordinator result", func(t *testing.T) {
		revision := int64(3)
		d := digest
		a := deliveryInspectorAdapter{coordinator: operationInspectorStub{operation: &release.Operation{ApplicationID: 7, State: models.AppsReleaseOperationSucceeded, ResultRevision: &revision, ResultDigest: &d}}, applications: applicationLookupStub{err: errors.New("must not call")}, releases: releaseInspectorStub{err: errors.New("must not call")}}
		got, err := a.Inspect(context.Background(), 7, "op")
		require.NoError(t, err)
		require.Equal(t, orchestrator.Inspection{Revision: 3, Digest: digest}, got)
	})
	t.Run("live target and conservative prior proof", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			observed time.Time
			created  time.Time
			prior    bool
		}{{"target", now.Add(time.Second), now, false}, {"prior", now.Add(-time.Second), now, true}, {"boundary", now, now, false}, {"zero operation time", now.Add(-time.Second), time.Time{}, false}} {
			t.Run(tc.name, func(t *testing.T) {
				a := deliveryInspectorAdapter{coordinator: operationInspectorStub{operation: &release.Operation{ApplicationID: 7, CreatedAt: tc.created}}, applications: applicationLookupStub{application: app}, releases: releaseInspectorStub{evidence: release.DeliveryInspection{Revision: 4, Digest: digest, ObservedAt: tc.observed}}}
				got, err := a.Inspect(context.Background(), 7, "op")
				require.NoError(t, err)
				require.Equal(t, tc.prior, got.PreviousDigestProven)
			})
		}
	})
	t.Run("mismatch and provider errors stay redacted", func(t *testing.T) {
		for _, a := range []deliveryInspectorAdapter{
			{coordinator: operationInspectorStub{operation: &release.Operation{ApplicationID: 8}}},
			{coordinator: operationInspectorStub{operation: &release.Operation{ApplicationID: 7}}, applications: applicationLookupStub{err: errors.New("authorization=secret")}},
			{coordinator: operationInspectorStub{operation: &release.Operation{ApplicationID: 7}}, applications: applicationLookupStub{application: app}, releases: releaseInspectorStub{err: errors.New("kubeconfig=secret")}},
		} {
			_, err := a.Inspect(context.Background(), 7, "op")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret")
		}
	})
}

// TestMountRoutes_Snapshot guards the /apps surface against silent drift.
// Adding, renaming, or removing a route forces the FE / docs / permission
// story to be re-checked.
//
// Uses dockertest Postgres because the apps_applications model's `tags`
// column declares `type:jsonb`, which SQLite's AutoMigrate cannot honour.
// The test only exercises route assembly; no DB writes happen.
func TestMountRoutes_Snapshot(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(td)

	rec := audit.NewRecorder(gdb)

	repoSvc := apprepo.NewService(apprepo.NewRepo(gdb), nopCipher{}, rec)
	appRepo := application.NewRepo(gdb)
	appSvc := application.NewService(appRepo, rec)
	relSvc := release.NewService(nopFactory{}, appSvc, &HelmChartLoader{Repo: repoSvc}, rec)

	m := New(repoSvc, appSvc, relSvc)

	cache := rbac.NewPermissionCache(gdb, time.Minute)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/api/v1")
	m.MountRoutes(grp, cache)

	paths := make([]string, 0, len(r.Routes()))
	for _, ri := range r.Routes() {
		paths = append(paths, ri.Method+" "+ri.Path)
	}
	sort.Strings(paths)

	want := []string{
		"DELETE /api/v1/apps/applications/:id",
		"DELETE /api/v1/apps/repos/:id",
		"GET /api/v1/apps/applications",
		"GET /api/v1/apps/applications/:id",
		"GET /api/v1/apps/applications/:id/release/history",
		"GET /api/v1/apps/applications/:id/release/status",
		"GET /api/v1/apps/repos",
		"GET /api/v1/apps/repos/:id",
		"GET /api/v1/apps/repos/:id/charts",
		"GET /api/v1/apps/repos/:id/charts/:chart/versions",
		"GET /api/v1/apps/repos/:id/charts/:chart/versions/:version/values",
		"POST /api/v1/apps/applications",
		"POST /api/v1/apps/applications/:id/release/install",
		"POST /api/v1/apps/applications/:id/release/rollback",
		"POST /api/v1/apps/applications/:id/release/uninstall",
		"POST /api/v1/apps/applications/:id/release/upgrade",
		"POST /api/v1/apps/repos",
		"PUT /api/v1/apps/applications/:id",
		"PUT /api/v1/apps/repos/:id",
	}
	require.Equal(t, want, paths, "route surface drifted:\n  got: %s", strings.Join(paths, "\n       "))
}

func TestDeliveryAdaptersExposeOnlyConsumerOwnedContracts(t *testing.T) {
	typ := reflect.TypeOf(DeliveryAdapters{})
	contracts := map[string]reflect.Type{
		"ProjectApplications": reflect.TypeOf((*project.ApplicationReader)(nil)).Elem(),
		"RunApplications":     reflect.TypeOf((*run.ApplicationReader)(nil)).Elem(),
		"Artifacts":           reflect.TypeOf((*run.ArtifactResolver)(nil)).Elem(),
		"ArtifactVersions":    reflect.TypeOf((*pipeline.VersionLister)(nil)).Elem(),
		"Executor":            reflect.TypeOf((*orchestrator.Executor)(nil)).Elem(),
		"Inspector":           reflect.TypeOf((*orchestrator.Inspector)(nil)).Elem(),
	}
	for name, contract := range contracts {
		field, ok := typ.FieldByName(name)
		require.True(t, ok)
		require.Equal(t, contract, field.Type)
	}
}
