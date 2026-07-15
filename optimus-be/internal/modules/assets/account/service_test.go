//go:build dbtest

package account

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/assets/errs"
	"optimus-be/internal/modules/audit"
	"optimus-be/tests/dbtest"
)

type fakeAudit struct {
	mu     sync.Mutex
	events []audit.Event
	err    error
}

func (f *fakeAudit) Record(_ context.Context, event audit.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
	return f.err
}

func (f *fakeAudit) snapshot() []audit.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]audit.Event(nil), f.events...)
}

type fakeCloudKeyExists struct {
	exists bool
	err    error
}

func (f fakeCloudKeyExists) Exists(context.Context, uint64) (bool, error) {
	return f.exists, f.err
}

func setupSvc(t *testing.T) (*Service, *Repo, *fakeAudit, *models.CredentialCloudKey) {
	t.Helper()
	repo, gdb := setupRepo(t)
	recorder := &fakeAudit{}
	cloudKey := dbtest.SeedCloudKey(t, gdb, "ck-"+t.Name())
	return NewService(repo, recorder, fakeCloudKeyExists{exists: true}), repo, recorder, cloudKey
}

func requireBizCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	var bizErr *apperr.BizError
	require.Error(t, err)
	require.True(t, errors.As(err, &bizErr))
	require.Equal(t, code, bizErr.Code)
}

func parseTargetID(t *testing.T, value string) uint64 {
	t.Helper()
	id, err := strconv.ParseUint(value, 10, 64)
	require.NoError(t, err)
	return id
}

func TestService_Create_ValidatesProvider(t *testing.T) {
	svc, _, _, cloudKey := setupSvc(t)
	_, err := svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "prod", Provider: "gcp", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"us-east-1"},
	})
	requireBizCode(t, err, errs.CodeAssetsProviderUnsupported)
}

func TestService_Create_ValidatesRegion(t *testing.T) {
	svc, _, _, cloudKey := setupSvc(t)
	_, err := svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"US_EAST_1"},
	})
	requireBizCode(t, err, errs.CodeAssetsRegionInvalid)
}

func TestService_Create_ChecksCloudKeyExists(t *testing.T) {
	repo, gdb := setupRepo(t)
	recorder := &fakeAudit{}
	svc := NewService(repo, recorder, fakeCloudKeyExists{exists: false})
	cloudKey := dbtest.SeedCloudKey(t, gdb, "missing-reference")
	_, err := svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"us-east-1"},
	})
	requireBizCode(t, err, errs.CodeAssetsCloudKeyNotFound)
}

func TestService_Create_NameConflict(t *testing.T) {
	svc, _, _, cloudKey := setupSvc(t)
	req := CreateRequest{Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"us-east-1"}}
	_, err := svc.Create(context.Background(), 7, "", "", req)
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), 7, "", "", req)
	requireBizCode(t, err, errs.CodeAssetsCloudAccountNameConflict)
}

func TestService_Create_WritesAudit(t *testing.T) {
	svc, _, recorder, cloudKey := setupSvc(t)
	detail, err := svc.Create(context.Background(), 7, "10.0.0.1", "test-agent", CreateRequest{
		Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	events := recorder.snapshot()
	require.Len(t, events, 1)
	require.Equal(t, "assets.cloud_account.create", events[0].Action)
	require.Equal(t, "cloud_account", events[0].TargetType)
	require.Equal(t, detail.ID, parseTargetID(t, events[0].TargetID))
	require.Equal(t, uint64(7), *events[0].UserID)
}

func TestService_Update_RegionShrinkageCascades(t *testing.T) {
	svc, repo, _, cloudKey := setupSvc(t)
	detail, err := svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID,
		EnabledRegions: []string{"us-east-1", "us-west-2"},
	})
	require.NoError(t, err)
	dbtest.SeedAWSInstance(t, repo.DB(), detail.ID, "us-east-1", "i-east")
	dbtest.SeedAWSInstance(t, repo.DB(), detail.ID, "us-west-2", "i-west")

	updated, err := svc.Update(context.Background(), 7, "", "", detail.ID, UpdateRequest{
		EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"us-east-1"}, updated.EnabledRegions)
	var alive []models.AWSInstance
	require.NoError(t, repo.DB().Where("cloud_account_id = ?", detail.ID).Find(&alive).Error)
	require.Len(t, alive, 1)
	require.Equal(t, "us-east-1", alive[0].Region)
}

func TestService_Delete_CascadesAllRegions(t *testing.T) {
	svc, repo, recorder, cloudKey := setupSvc(t)
	detail, err := svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID,
		EnabledRegions: []string{"us-east-1", "us-west-2"},
	})
	require.NoError(t, err)
	dbtest.SeedAWSInstance(t, repo.DB(), detail.ID, "us-east-1", "i-east")
	dbtest.SeedAWSInstance(t, repo.DB(), detail.ID, "us-west-2", "i-west")

	cascaded, err := svc.Delete(context.Background(), 7, "", "", detail.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, cascaded)
	_, err = repo.FindByID(context.Background(), detail.ID)
	requireBizCode(t, err, errs.CodeAssetsCloudAccountNotFound)
	events := recorder.snapshot()
	require.Equal(t, "assets.cloud_account.delete", events[len(events)-1].Action)
}
