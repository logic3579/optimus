//go:build dbtest

package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps/application"
	"optimus-be/internal/modules/audit"
	deliveryproject "optimus-be/internal/modules/delivery/project"
)

// setupSvc returns a Service + Repo + (clusterID, chartRepoID) for use by
// service-level tests. Adds a permissions-registered admin row so the audit
// log FK (user_id → users.id) is satisfied when actorID != 0 — service tests
// in this suite pass actorID=0 so audit rows have UserID = nil, but the
// helper is here for the full HTTP suite.
func setupSvc(t *testing.T) (*application.Service, *application.Repo, uint64, uint64) {
	t.Helper()
	r, td := newRepo(t)
	t.Cleanup(td)
	clID, crID := seedFKs(t, r)
	rec := audit.NewRecorder(r.DB())
	svc := application.NewService(r, rec)
	return svc, r, clID, crID
}

// fakeProbe implements application.HelmStatusProbe deterministically.
type fakeProbe struct {
	status string
	rev    *int
	cv     string
	av     string
	ldp    string
	err    error
}

func (f *fakeProbe) StatusForApplication(_ context.Context, _ *models.AppsApplication) (string, *int, string, string, string, error) {
	if f.err != nil {
		return "", nil, "", "", "", f.err
	}
	return f.status, f.rev, f.cv, f.av, f.ldp, nil
}

// fakeChecker implements application.HelmInstalledChecker.
type fakeChecker struct {
	installed bool
	err       error
	calls     int
}

func (f *fakeChecker) IsReleaseInstalled(_ context.Context, _ *models.AppsApplication) (bool, error) {
	f.calls++
	return f.installed, f.err
}

type fakeDeliveryApplicationCounter struct {
	count int64
	err   error
	calls int
}

func (f *fakeDeliveryApplicationCounter) CountByApplicationID(_ context.Context, _ uint64) (int64, error) {
	f.calls++
	return f.count, f.err
}

type databaseApplicationReader struct{ repo *application.Repo }

func (r databaseApplicationReader) GetApplication(ctx context.Context, id uint64) (*deliveryproject.Application, error) {
	row, err := r.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &deliveryproject.Application{
		ID: row.ID, Name: row.Name, ChartRepoID: row.ChartRepoID, ChartName: row.ChartName,
		Installed: true, ClusterID: row.ClusterID, Namespace: row.Namespace, ReleaseName: row.ReleaseName,
	}, nil
}

func TestService_Create_NameConflict(t *testing.T) {
	svc, _, clID, crID := setupSvc(t)
	ctx := context.Background()
	_, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "dup", ClusterID: clID, Namespace: "default", ReleaseName: "a",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)
	_, err = svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "dup", ClusterID: clID, Namespace: "default", ReleaseName: "b",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.Error(t, err)
	var be *apperr.BizError
	require.True(t, errors.As(err, &be))
	require.Equal(t, apperr.CodeConflict, be.Code)
	require.Equal(t, "apps.application.name_taken", be.MessageKey)
}

func TestService_Create_ReleaseTupleConflict(t *testing.T) {
	svc, _, clID, crID := setupSvc(t)
	ctx := context.Background()
	_, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "a", ClusterID: clID, Namespace: "default", ReleaseName: "rel",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)
	_, err = svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "b", ClusterID: clID, Namespace: "default", ReleaseName: "rel",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.Error(t, err)
	var be *apperr.BizError
	require.True(t, errors.As(err, &be))
	require.Equal(t, apperr.CodeAppsReleaseNameDuplicate, be.Code)
}

func TestService_Get_DecoratesStatusWhenProbeSet(t *testing.T) {
	svc, _, clID, crID := setupSvc(t)
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "p", ClusterID: clID, Namespace: "default", ReleaseName: "p",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)

	// Without a probe wired in, Status fields are empty.
	got, err := svc.Get(ctx, d.ID)
	require.NoError(t, err)
	require.Equal(t, "", got.Status)
	require.Nil(t, got.Revision)

	// With a probe wired in, Status fields are populated.
	rev := 3
	svc.SetHelmStatusProbe(&fakeProbe{
		status: "deployed", rev: &rev, cv: "1.2.3", av: "v1", ldp: "2026-01-01T00:00:00Z",
	})
	got, err = svc.Get(ctx, d.ID)
	require.NoError(t, err)
	require.Equal(t, "deployed", got.Status)
	require.NotNil(t, got.Revision)
	require.Equal(t, 3, *got.Revision)
	require.Equal(t, "1.2.3", got.ChartVersion)
	require.Equal(t, "v1", got.AppVersion)
	require.Equal(t, "2026-01-01T00:00:00Z", got.LastDeployedAt)
}

func TestService_Get_ProbeErrorLeavesFieldsEmpty(t *testing.T) {
	svc, _, clID, crID := setupSvc(t)
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "p", ClusterID: clID, Namespace: "default", ReleaseName: "p",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)
	svc.SetHelmStatusProbe(&fakeProbe{err: errors.New("upstream down")})
	got, err := svc.Get(ctx, d.ID)
	require.NoError(t, err)
	require.Equal(t, "", got.Status, "probe error should not leak; status stays empty")
}

func TestService_Delete_RefusedWhenReleaseStillInstalled(t *testing.T) {
	svc, _, clID, crID := setupSvc(t)
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "d", ClusterID: clID, Namespace: "default", ReleaseName: "d",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)
	svc.SetHelmInstalledChecker(&fakeChecker{installed: true})
	err = svc.Delete(ctx, 0, "", "", d.ID)
	require.Error(t, err)
	var be *apperr.BizError
	require.True(t, errors.As(err, &be))
	require.Equal(t, apperr.CodeAppsReleaseStillPresent, be.Code)
	require.Equal(t, "apps.application.release_still_installed", be.MessageKey)
}

func TestService_Delete_AllowedWhenNoCheckerOrNotInstalled(t *testing.T) {
	svc, _, clID, crID := setupSvc(t)
	ctx := context.Background()

	// no checker -> allowed.
	d, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "x1", ClusterID: clID, Namespace: "default", ReleaseName: "x1",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, 0, "", "", d.ID))

	// checker set, installed=false -> allowed.
	svc.SetHelmInstalledChecker(&fakeChecker{installed: false})
	d, err = svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "x2", ClusterID: clID, Namespace: "default", ReleaseName: "x2",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, 0, "", "", d.ID))
}

func TestService_Delete_RefusedWhenApplicationIsDeliveryManaged(t *testing.T) {
	svc, r, clID, crID := setupSvc(t)
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "delivery-managed", ClusterID: clID, Namespace: "default", ReleaseName: "delivery-managed",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)
	counter := &fakeDeliveryApplicationCounter{count: 1}
	checker := &fakeChecker{installed: false}
	svc.SetDeliveryApplicationCounter(counter)
	svc.SetHelmInstalledChecker(checker)

	err = svc.Delete(ctx, 0, "", "", d.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeDeliveryEnvironmentInUse, be.Code)
	require.Equal(t, "delivery.environment.in_use", be.MessageKey)
	require.Equal(t, 1, counter.calls)
	require.Zero(t, checker.calls, "delivery binding check must precede the Helm lookup")
	_, getErr := r.Get(ctx, d.ID)
	require.NoError(t, getErr, "blocked application must remain active")
}

func TestService_Delete_DeliveryCounterLookupFailureFailsClosed(t *testing.T) {
	svc, r, clID, crID := setupSvc(t)
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "counter-error", ClusterID: clID, Namespace: "default", ReleaseName: "counter-error",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)
	wantErr := errors.New("postgres exposed sensitive topology")
	svc.SetDeliveryApplicationCounter(&fakeDeliveryApplicationCounter{err: wantErr})

	err = svc.Delete(ctx, 0, "", "", d.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeDeliveryApplicationUnavailable, be.Code)
	require.Equal(t, "delivery.application.unavailable", be.MessageKey)
	require.NotContains(t, be.Message, "sensitive topology")
	require.ErrorIs(t, err, wantErr)
	_, getErr := r.Get(ctx, d.ID)
	require.NoError(t, getErr, "lookup failure must fail closed")
}

func TestService_Delete_DeliveryCounterNilSafeAndZeroAllowsExistingBehavior(t *testing.T) {
	svc, _, clID, crID := setupSvc(t)
	ctx := context.Background()

	d, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "nil-counter", ClusterID: clID, Namespace: "default", ReleaseName: "nil-counter",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)
	svc.SetDeliveryApplicationCounter(nil)
	require.NoError(t, svc.Delete(ctx, 0, "", "", d.ID))

	d, err = svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "zero-counter", ClusterID: clID, Namespace: "default", ReleaseName: "zero-counter",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)
	counter := &fakeDeliveryApplicationCounter{}
	svc.SetDeliveryApplicationCounter(counter)
	require.NoError(t, svc.Delete(ctx, 0, "", "", d.ID))
	require.Equal(t, 1, counter.calls)
}

func TestService_DeleteAndDeliveryBindSerializeWithoutDanglingBinding(t *testing.T) {
	svc, appRepo, clID, crID := setupSvc(t)
	ctx := context.Background()
	app, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "bind-delete-race", ClusterID: clID, Namespace: "default", ReleaseName: "bind-delete-race",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)

	deliveryRepo := deliveryproject.NewRepo(appRepo.DB())
	deliverySvc := deliveryproject.NewService(deliveryRepo, databaseApplicationReader{repo: appRepo}, nil, nil)
	project, err := deliverySvc.CreateProject(ctx, 0, "", "", deliveryproject.CreateProjectRequest{Name: "race-project"})
	require.NoError(t, err)
	svc.SetDeliveryApplicationCounter(deliveryRepo)

	start := make(chan struct{})
	bindDone := make(chan error, 1)
	deleteDone := make(chan error, 1)
	go func() {
		<-start
		_, bindErr := deliverySvc.BindEnvironment(ctx, 0, "", "", project.ID, deliveryproject.BindEnvironmentRequest{
			EnvironmentKey: "prod", DisplayName: "Production", ApplicationID: app.ID,
		})
		bindDone <- bindErr
	}()
	go func() {
		<-start
		deleteDone <- svc.Delete(ctx, 0, "", "", app.ID)
	}()
	close(start)
	bindErr := <-bindDone
	deleteErr := <-deleteDone

	_, appErr := appRepo.Get(ctx, app.ID)
	bindings, countErr := deliveryRepo.CountByApplicationID(ctx, app.ID)
	require.NoError(t, countErr)
	require.False(t, errors.Is(appErr, gorm.ErrRecordNotFound) && bindings > 0,
		"active delivery binding must never reference a deleted application")
	if bindErr == nil {
		be, ok := apperr.AsBiz(deleteErr)
		require.True(t, ok)
		require.Equal(t, apperr.CodeDeliveryEnvironmentInUse, be.Code)
		require.NoError(t, appErr)
		require.EqualValues(t, 1, bindings)
		return
	}
	require.NoError(t, deleteErr)
	be, ok := apperr.AsBiz(bindErr)
	require.True(t, ok)
	require.Equal(t, apperr.CodeDeliveryApplicationUnavailable, be.Code)
	require.ErrorIs(t, appErr, gorm.ErrRecordNotFound)
	require.Zero(t, bindings)
}

func TestService_Update_OnlyAllowedFields(t *testing.T) {
	svc, r, clID, crID := setupSvc(t)
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "u", ClusterID: clID, Namespace: "default", ReleaseName: "u",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)

	desc := "new description"
	newTags := []string{"prod", "us-east"}
	uid := uint64(42)
	// Pretend a user row exists so the FK ON DELETE SET NULL accepts the owner_user_id.
	require.NoError(t, r.DB().Exec(
		`INSERT INTO users (id, username, password_hash, email, display_name, status, created_at, updated_at) `+
			`VALUES (?, ?, '', '', '', 'enabled', NOW(), NOW())`, uid, "user42").Error)

	out, err := svc.Update(ctx, 0, "", "", d.ID, application.UpdateRequest{
		Description: &desc,
		Tags:        newTags,
		OwnerUserID: &uid,
	})
	require.NoError(t, err)
	require.Equal(t, "new description", out.Description)
	require.Equal(t, []string{"prod", "us-east"}, out.Tags)
	require.NotNil(t, out.OwnerUserID)
	require.Equal(t, uid, *out.OwnerUserID)

	// Re-read raw model: name, cluster_id, namespace, release_name, chart_name,
	// chart_repo_id all must still equal their original values.
	raw, err := r.Get(ctx, d.ID)
	require.NoError(t, err)
	require.Equal(t, "u", raw.Name)
	require.Equal(t, clID, raw.ClusterID)
	require.Equal(t, "default", raw.Namespace)
	require.Equal(t, "u", raw.ReleaseName)
	require.Equal(t, crID, raw.ChartRepoID)
	require.Equal(t, "nginx", raw.ChartName)
}

func TestService_SetChartRepo_PatchesField(t *testing.T) {
	svc, r, clID, crID := setupSvc(t)
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", application.CreateRequest{
		Name: "rc", ClusterID: clID, Namespace: "default", ReleaseName: "rc",
		ChartRepoID: crID, ChartName: "nginx",
	})
	require.NoError(t, err)

	// Add a second chart repo.
	cr2 := &models.AppsChartRepo{Name: "cr2", Type: "oci", URL: "oci://x"}
	require.NoError(t, r.DB().Create(cr2).Error)

	require.NoError(t, svc.SetChartRepo(ctx, d.ID, cr2.ID))
	raw, err := r.Get(ctx, d.ID)
	require.NoError(t, err)
	require.Equal(t, cr2.ID, raw.ChartRepoID)
}

func TestService_Get_NotFound(t *testing.T) {
	svc, _, _, _ := setupSvc(t)
	_, err := svc.Get(context.Background(), 99999)
	require.Error(t, err)
	var be *apperr.BizError
	require.True(t, errors.As(err, &be))
	require.Equal(t, apperr.CodeNotFound, be.Code)
}
