//go:build dbtest

package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps/application"
	"optimus-be/internal/modules/audit"
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
}

func (f *fakeChecker) IsReleaseInstalled(_ context.Context, _ *models.AppsApplication) (bool, error) {
	return f.installed, f.err
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
