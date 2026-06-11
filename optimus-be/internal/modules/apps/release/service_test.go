package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
)

// --- fakes -----------------------------------------------------------------

// inMemoryRecorder captures audit.Event for assertions. Safe for parallel
// goroutines in case anything ever fans out.
type inMemoryRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *inMemoryRecorder) Record(_ context.Context, e audit.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}

func (r *inMemoryRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

// fakeFactory returns the same in-memory helm action.Configuration for every
// call. That works for unit tests because helm's release storage lives in
// memory anyway — install/upgrade/rollback all see the same release history.
type fakeFactory struct {
	cfg *action.Configuration
}

func newFakeFactory() *fakeFactory {
	cfg := &action.Configuration{
		Releases:     storage.Init(driver.NewMemory()),
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(_ string, _ ...interface{}) {},
	}
	return &fakeFactory{cfg: cfg}
}

func (f *fakeFactory) NewForCluster(_ context.Context, _ uint64, _, _ string) (*action.Configuration, error) {
	return f.cfg, nil
}

// fakeChartLoader returns a minimal helm chart whose metadata version mirrors
// the requested version.
type fakeChartLoader struct{}

func (fakeChartLoader) LoadChart(_ context.Context, _ uint64, name, version string) (*chart.Chart, error) {
	return loader.LoadArchive(buildMinimalChartTgz(name, version))
}

// stubAppService implements AppLookup. Holds a single AppsApplication row
// in memory; SetChartRepo mutates ChartRepoID in place.
type stubAppService struct {
	app *models.AppsApplication
}

func newStubAppService() *stubAppService {
	return &stubAppService{
		app: &models.AppsApplication{
			ID:          42,
			Name:        "demo",
			ClusterID:   1,
			Namespace:   "default",
			ReleaseName: "demo",
			ChartRepoID: 7,
			ChartName:   "mychart",
		},
	}
}

func (s *stubAppService) GetModel(_ context.Context, id uint64) (*models.AppsApplication, error) {
	if id != s.app.ID {
		return nil, apperr.New(apperr.CodeNotFound, "apps.application.not_found", "stub: not found")
	}
	return s.app, nil
}

func (s *stubAppService) SetChartRepo(_ context.Context, _, newRepoID uint64) error {
	s.app.ChartRepoID = newRepoID
	return nil
}

// failingChartLoader always errors. Used to verify error-flow.
type failingChartLoader struct{ err error }

func (f failingChartLoader) LoadChart(_ context.Context, _ uint64, _, _ string) (*chart.Chart, error) {
	return nil, f.err
}

// --- chart tgz builder -----------------------------------------------------

// buildMinimalChartTgz produces an in-memory chart tarball with the layout
// helm's loader.LoadArchive expects:
//
//	<root>/Chart.yaml
//	<root>/values.yaml
//	<root>/templates/configmap.yaml  (no-op ConfigMap)
//
// `root` is hard-coded to "mychart" so the chart name in Chart.yaml matches
// the directory name (helm rejects mismatches).
func buildMinimalChartTgz(_ /*name*/, version string) io.Reader {
	const root = "mychart"
	chartYAML := "apiVersion: v2\nname: " + root + "\nversion: " + version + "\nappVersion: \"1.0\"\n"
	const valuesYAML = ""
	const templateYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: noop
  namespace: {{ .Release.Namespace }}
data:
  marker: "release-unit-test"
`
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := []struct {
		name string
		body string
	}{
		{root + "/Chart.yaml", chartYAML},
		{root + "/values.yaml", valuesYAML},
		{root + "/templates/configmap.yaml", templateYAML},
	}
	now := time.Now()
	for _, f := range files {
		hdr := &tar.Header{
			Name:     f.name,
			Mode:     0o644,
			Size:     int64(len(f.body)),
			ModTime:  now,
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			panic(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			panic(err)
		}
	}
	if err := tw.Close(); err != nil {
		panic(err)
	}
	if err := gz.Close(); err != nil {
		panic(err)
	}
	return bytes.NewReader(buf.Bytes())
}

// --- helpers ---------------------------------------------------------------

func newTestService() (*Service, *stubAppService, *inMemoryRecorder, *fakeFactory) {
	rec := &inMemoryRecorder{}
	factory := newFakeFactory()
	apps := newStubAppService()
	s := NewService(factory, apps, fakeChartLoader{}, rec)
	return s, apps, rec, factory
}

// --- tests -----------------------------------------------------------------

func TestNewService_PanicsOnNilSeams(t *testing.T) {
	rec := &inMemoryRecorder{}
	f := newFakeFactory()
	apps := newStubAppService()
	loader := fakeChartLoader{}

	cases := []func(){
		func() { _ = NewService(nil, apps, loader, rec) },
		func() { _ = NewService(f, nil, loader, rec) },
		func() { _ = NewService(f, apps, nil, rec) },
		func() { _ = NewService(f, apps, loader, nil) },
	}
	for i, c := range cases {
		i, c := i, c
		t.Run("", func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatalf("case %d: expected panic, got none", i)
				}
			}()
			c()
		})
	}
}

func TestService_Install_Then_Upgrade_Then_Rollback_Then_Uninstall(t *testing.T) {
	s, apps, rec, _ := newTestService()
	ctx := context.Background()
	id := apps.app.ID

	// install
	res, err := s.Install(ctx, 1, "1.1.1.1", "ua/1", id, InstallRequest{ChartVersion: "1.0.0"})
	require.NoError(t, err)
	require.Equal(t, 1, res.Revision)
	require.Equal(t, "deployed", res.Status)
	require.Equal(t, "1.0.0", res.ChartVersion)

	// upgrade
	res, err = s.Upgrade(ctx, 1, "1.1.1.1", "ua/1", id, UpgradeRequest{ChartVersion: "1.1.0"})
	require.NoError(t, err)
	require.Equal(t, 2, res.Revision)
	require.Equal(t, "deployed", res.Status)

	// status reflects revision 2
	st, err := s.Status(ctx, id)
	require.NoError(t, err)
	require.Equal(t, 2, st.Revision)
	require.Equal(t, "deployed", st.Status)

	// history has 2 entries
	hist, err := s.History(ctx, id)
	require.NoError(t, err)
	require.Len(t, hist, 2)

	// rollback to v1 -> new revision 3
	rb, err := s.Rollback(ctx, 1, "1.1.1.1", "ua/1", id, RollbackRequest{Revision: 1})
	require.NoError(t, err)
	require.Equal(t, 3, rb.Revision)

	// uninstall (no keep_history)
	require.NoError(t, s.Uninstall(ctx, 1, "1.1.1.1", "ua/1", id, UninstallRequest{}))

	// audit captured 4 writes
	events := rec.snapshot()
	require.Len(t, events, 4)
	require.Equal(t, "apps.release.install", events[0].Action)
	require.Equal(t, "apps.release.upgrade", events[1].Action)
	require.Equal(t, "apps.release.rollback", events[2].Action)
	require.Equal(t, "apps.release.uninstall", events[3].Action)
	require.NotNil(t, events[0].UserID)
	require.Equal(t, uint64(1), *events[0].UserID)
	require.Equal(t, "42", events[0].TargetID)
	require.Equal(t, "apps_application", events[0].TargetType)
}

func TestService_Install_DuplicateRelease(t *testing.T) {
	s, apps, _, _ := newTestService()
	ctx := context.Background()
	id := apps.app.ID

	_, err := s.Install(ctx, 1, "", "", id, InstallRequest{ChartVersion: "1.0.0"})
	require.NoError(t, err)

	_, err = s.Install(ctx, 1, "", "", id, InstallRequest{ChartVersion: "1.0.0"})
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok, "expected BizError, got %T: %v", err, err)
	require.Equal(t, apperr.CodeAppsReleaseAlreadyExists, be.Code)
}

func TestService_Status_NotFound(t *testing.T) {
	s, apps, _, _ := newTestService()
	_, err := s.Status(context.Background(), apps.app.ID)
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAppsReleaseNotFound, be.Code)
}

func TestService_History_NotFound(t *testing.T) {
	s, apps, _, _ := newTestService()
	_, err := s.History(context.Background(), apps.app.ID)
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAppsReleaseNotFound, be.Code)
}

func TestService_Rollback_RevisionMissing(t *testing.T) {
	s, apps, _, _ := newTestService()
	ctx := context.Background()
	id := apps.app.ID

	_, err := s.Install(ctx, 1, "", "", id, InstallRequest{ChartVersion: "1.0.0"})
	require.NoError(t, err)

	_, err = s.Rollback(ctx, 1, "", "", id, RollbackRequest{Revision: 999})
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok, "expected BizError, got %T: %v", err, err)
	require.Equal(t, apperr.CodeAppsReleaseHistoryTooShort, be.Code)
}

func TestService_Upgrade_RepointsChartRepo(t *testing.T) {
	s, apps, _, _ := newTestService()
	ctx := context.Background()
	id := apps.app.ID

	_, err := s.Install(ctx, 1, "", "", id, InstallRequest{ChartVersion: "1.0.0"})
	require.NoError(t, err)

	newRepo := uint64(99)
	_, err = s.Upgrade(ctx, 1, "", "", id, UpgradeRequest{
		ChartRepoID: &newRepo, ChartVersion: "1.1.0",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(99), apps.app.ChartRepoID, "ChartRepoID should have been repointed")
}

func TestService_Upgrade_NotFound(t *testing.T) {
	// Upgrade against a release that was never installed.
	s, apps, _, _ := newTestService()
	_, err := s.Upgrade(context.Background(), 1, "", "", apps.app.ID, UpgradeRequest{ChartVersion: "1.0.0"})
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	// helm's ErrNoDeployedReleases sentinel → 42202 via MapError.
	require.Equal(t, apperr.CodeAppsReleaseNotFound, be.Code)
}

func TestService_AppLookup_NotFound(t *testing.T) {
	s, _, _, _ := newTestService()
	_, err := s.Status(context.Background(), 9999)
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_Install_ChartLoaderError(t *testing.T) {
	rec := &inMemoryRecorder{}
	factory := newFakeFactory()
	apps := newStubAppService()
	want := errors.New("repo down")
	s := NewService(factory, apps, failingChartLoader{err: want}, rec)
	_, err := s.Install(context.Background(), 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
	require.ErrorIs(t, err, want)
}

func TestService_Install_InvalidValues(t *testing.T) {
	s, apps, _, _ := newTestService()
	// A scalar at the root is not a valid helm values document.
	_, err := s.Install(context.Background(), 1, "", "", apps.app.ID, InstallRequest{
		ChartVersion: "1.0.0", ValuesYAML: "just-a-string",
	})
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAppsReleaseInvalidValues, be.Code)
}

func TestParseValues(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty", "", false},
		{"whitespace only", "   \n\t  ", false},
		{"valid map", "foo: bar\nbaz: 1\n", false},
		{"null root", "null\n", false},
		{"scalar root", "42", true},
		{"sequence root", "- one\n- two\n", true},
		{"bad yaml", "[unclosed", true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			out, err := parseValues(c.in)
			if c.wantErr {
				require.Error(t, err)
				be, ok := apperr.AsBiz(err)
				require.True(t, ok)
				require.Equal(t, apperr.CodeAppsReleaseInvalidValues, be.Code)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, out)
		})
	}
}

func TestService_Probe_IsReleaseInstalled_FalseWhenAbsent(t *testing.T) {
	s, apps, _, _ := newTestService()
	installed, err := s.IsReleaseInstalled(context.Background(), apps.app)
	require.NoError(t, err)
	require.False(t, installed)
}

func TestService_Probe_IsReleaseInstalled_TrueWhenDeployed(t *testing.T) {
	s, apps, _, _ := newTestService()
	_, err := s.Install(context.Background(), 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
	require.NoError(t, err)
	installed, err := s.IsReleaseInstalled(context.Background(), apps.app)
	require.NoError(t, err)
	require.True(t, installed)
}

func TestService_Probe_StatusForApplication_ReturnsUnknownOnAbsence(t *testing.T) {
	s, apps, _, _ := newTestService()
	status, rev, cv, av, ldp, err := s.StatusForApplication(context.Background(), apps.app)
	require.NoError(t, err) // probe swallows helm errors and surfaces "unknown"
	require.Equal(t, "unknown", status)
	require.Nil(t, rev)
	require.Empty(t, cv)
	require.Empty(t, av)
	require.Empty(t, ldp)
}

func TestService_Probe_StatusForApplication_DecoratesWhenDeployed(t *testing.T) {
	s, apps, _, _ := newTestService()
	_, err := s.Install(context.Background(), 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
	require.NoError(t, err)
	status, rev, cv, _, ldp, err := s.StatusForApplication(context.Background(), apps.app)
	require.NoError(t, err)
	require.Equal(t, "deployed", status)
	require.NotNil(t, rev)
	require.Equal(t, 1, *rev)
	require.Equal(t, "1.0.0", cv)
	require.NotEmpty(t, ldp)
}

func TestService_Uninstall_NotFound(t *testing.T) {
	s, apps, _, _ := newTestService()
	err := s.Uninstall(context.Background(), 1, "", "", apps.app.ID, UninstallRequest{})
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAppsReleaseNotFound, be.Code)
}
