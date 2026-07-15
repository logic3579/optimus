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
	_, err = svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{},
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
	require.Equal(t, "10.0.0.1", events[0].IP)
	require.Equal(t, "test-agent", events[0].UserAgent)
	payload := events[0].Payload.(map[string]any)
	require.Equal(t, "prod", payload["name"])
	require.Equal(t, "aws", payload["provider"])
	require.Equal(t, cloudKey.ID, payload["cloudkey_id"])
	require.Equal(t, []string{"us-east-1"}, payload["regions"])
	require.True(t, detail.Enabled)
}

func TestService_Update_RegionShrinkageCascades(t *testing.T) {
	svc, repo, _, cloudKey := setupSvc(t)
	detail, err := svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID,
		EnabledRegions: []string{"us-east-1", "us-west-2"},
	})
	require.NoError(t, err)
	seedResourceSet(t, repo.DB(), detail.ID, "us-east-1", "service-east")
	seedResourceSet(t, repo.DB(), detail.ID, "us-west-2", "service-west")

	updated, err := svc.Update(context.Background(), 7, "", "", detail.ID, UpdateRequest{
		EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"us-east-1"}, updated.EnabledRegions)
	var alive []models.AWSInstance
	require.NoError(t, repo.DB().Where("cloud_account_id = ?", detail.ID).Find(&alive).Error)
	require.Len(t, alive, 1)
	require.Equal(t, "us-east-1", alive[0].Region)
	for _, model := range []any{&models.AWSVPC{}, &models.AWSSubnet{}, &models.AWSDatabase{}} {
		var count int64
		require.NoError(t, repo.DB().Model(model).Where("cloud_account_id = ?", detail.ID).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
}

func TestService_Delete_CascadesAllRegions(t *testing.T) {
	svc, repo, recorder, cloudKey := setupSvc(t)
	detail, err := svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID,
		EnabledRegions: []string{"us-east-1", "us-west-2"},
	})
	require.NoError(t, err)
	seedResourceSet(t, repo.DB(), detail.ID, "us-east-1", "delete-east")
	seedResourceSet(t, repo.DB(), detail.ID, "us-west-2", "delete-west")

	cascaded, err := svc.Delete(context.Background(), 7, "delete-ip", "delete-ua", detail.ID)
	require.NoError(t, err)
	require.EqualValues(t, 8, cascaded)
	_, err = repo.FindByID(context.Background(), detail.ID)
	requireBizCode(t, err, errs.CodeAssetsCloudAccountNotFound)
	events := recorder.snapshot()
	event := events[len(events)-1]
	require.Equal(t, "assets.cloud_account.delete", event.Action)
	require.Equal(t, "cloud_account", event.TargetType)
	require.Equal(t, detail.ID, parseTargetID(t, event.TargetID))
	require.Equal(t, uint64(7), *event.UserID)
	require.Equal(t, "delete-ip", event.IP)
	require.Equal(t, "delete-ua", event.UserAgent)
	payload := event.Payload.(map[string]any)
	require.Equal(t, "prod", payload["name"])
	require.EqualValues(t, 8, payload["cascaded_resources_count"])
}

func TestService_GetListUpdateAndNoop(t *testing.T) {
	svc, _, recorder, cloudKey := setupSvc(t)
	detail, err := svc.Create(context.Background(), 9, "ip", "ua", CreateRequest{
		Name: "before", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	got, err := svc.Get(context.Background(), detail.ID)
	require.NoError(t, err)
	require.Equal(t, "before", got.Name)
	listed, err := svc.List(context.Background(), ListQuery{})
	require.NoError(t, err)
	require.Len(t, listed.Items, 1)
	require.Equal(t, cloudKey.Name, listed.Items[0].CloudKeyName)

	name, description, enabled := "after", "desc", false
	updated, err := svc.Update(context.Background(), 9, "update-ip", "update-ua", detail.ID, UpdateRequest{
		Name: &name, Description: &description, Enabled: &enabled, EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	require.Equal(t, "after", updated.Name)
	require.False(t, updated.Enabled)
	events := recorder.snapshot()
	updateEvent := events[len(events)-1]
	require.Equal(t, "assets.cloud_account.update", updateEvent.Action)
	require.Equal(t, "cloud_account", updateEvent.TargetType)
	require.Equal(t, detail.ID, parseTargetID(t, updateEvent.TargetID))
	require.Equal(t, uint64(9), *updateEvent.UserID)
	require.Equal(t, "update-ip", updateEvent.IP)
	require.Equal(t, "update-ua", updateEvent.UserAgent)
	require.Equal(t, []string{"description", "enabled", "name"}, updateEvent.Payload.(map[string]any)["changed_fields"])

	before := len(events)
	_, err = svc.Update(context.Background(), 9, "ip", "ua", detail.ID, UpdateRequest{EnabledRegions: []string{"us-east-1"}})
	require.NoError(t, err)
	require.Len(t, recorder.snapshot(), before, "equal regions must be a no-op")
}

func TestService_Create_PropagatesCloudKeyCheckerError(t *testing.T) {
	repo, _ := setupRepo(t)
	wantErr := errors.New("checker failed")
	svc := NewService(repo, &fakeAudit{}, fakeCloudKeyExists{err: wantErr})
	_, err := svc.Create(context.Background(), 1, "", "", CreateRequest{
		Name: "prod", Provider: "aws", CloudKeyID: 42, EnabledRegions: []string{"us-east-1"},
	})
	require.ErrorIs(t, err, wantErr)
}

func TestService_RecordSyncTriggerAudit(t *testing.T) {
	svc, _, recorder, cloudKey := setupSvc(t)
	detail, err := svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	actor := uint64(11)
	svc.RecordSyncTrigger(context.Background(), &actor, "sync-ip", "sync-ua", detail.ID)
	event := recorder.snapshot()[1]
	require.Equal(t, "assets.cloud_account.sync_trigger", event.Action)
	require.Equal(t, "cloud_account", event.TargetType)
	require.Equal(t, detail.ID, parseTargetID(t, event.TargetID))
	require.Equal(t, actor, *event.UserID)
	require.Equal(t, "sync-ip", event.IP)
	require.Equal(t, "sync-ua", event.UserAgent)
	require.Equal(t, "manual", event.Payload.(map[string]any)["trigger"])
}

func TestService_Delete_RollsBackWhenCascadeFails(t *testing.T) {
	svc, repo, _, cloudKey := setupSvc(t)
	detail, err := svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "rollback", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	dbtest.SeedAWSInstance(t, repo.DB(), detail.ID, "us-east-1", "i-rollback")
	require.NoError(t, repo.DB().Migrator().DropTable(&models.AWSVPC{}))
	_, err = svc.Delete(context.Background(), 7, "", "", detail.ID)
	require.Error(t, err)
	_, err = repo.FindByID(context.Background(), detail.ID)
	require.NoError(t, err)
	var alive int64
	require.NoError(t, repo.DB().Model(&models.AWSInstance{}).Where("cloud_account_id = ?", detail.ID).Count(&alive).Error)
	require.EqualValues(t, 1, alive)
}

func TestService_PropagatesNameAndCloudKeyNameLookupErrors(t *testing.T) {
	t.Run("name lookup", func(t *testing.T) {
		repo, _ := setupRepo(t)
		require.NoError(t, repo.DB().Migrator().DropTable(&models.CloudAccount{}))
		svc := NewService(repo, &fakeAudit{}, fakeCloudKeyExists{exists: true})
		_, err := svc.Create(context.Background(), 1, "", "", CreateRequest{
			Name: "prod", Provider: "aws", CloudKeyID: 1, EnabledRegions: []string{"us-east-1"},
		})
		require.Error(t, err)
	})
	t.Run("cloud key names lookup", func(t *testing.T) {
		svc, repo, _, cloudKey := setupSvc(t)
		_, err := svc.Create(context.Background(), 1, "", "", CreateRequest{
			Name: "prod", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"us-east-1"},
		})
		require.NoError(t, err)
		require.NoError(t, repo.DB().Migrator().DropTable(&models.CredentialCloudKey{}))
		_, err = svc.List(context.Background(), ListQuery{})
		require.Error(t, err)
	})
}

func TestService_UpdateDelete_DeletedAccountReturnsNotFoundWithoutAudit(t *testing.T) {
	svc, repo, recorder, cloudKey := setupSvc(t)
	detail, err := svc.Create(context.Background(), 7, "", "", CreateRequest{
		Name: "stale", Provider: "aws", CloudKeyID: cloudKey.ID, EnabledRegions: []string{"us-east-1"},
	})
	require.NoError(t, err)
	require.NoError(t, repo.SoftDelete(context.Background(), detail.ID))
	name := "renamed"
	_, err = svc.Update(context.Background(), 7, "", "", detail.ID, UpdateRequest{Name: &name})
	requireBizCode(t, err, errs.CodeAssetsCloudAccountNotFound)
	_, err = svc.Delete(context.Background(), 7, "", "", detail.ID)
	requireBizCode(t, err, errs.CodeAssetsCloudAccountNotFound)
	require.Len(t, recorder.snapshot(), 1, "failed stale writes must not audit")
}
