//go:build dbtest

package cloudkey_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/cloudkey"
)

type passthroughCipher struct{}

func (passthroughCipher) Seal(b []byte) ([]byte, error) {
	out := make([]byte, 0, len(b)+4)
	out = append(out, []byte("SEAL")...)
	out = append(out, b...)
	return out, nil
}

func (passthroughCipher) Open(b []byte) ([]byte, error) {
	if len(b) < 4 || string(b[:4]) != "SEAL" {
		return nil, errors.New("bad ciphertext")
	}
	return b[4:], nil
}

func newSvc(t *testing.T) (*cloudkey.Service, func()) {
	svc, _, td := newSvcWithDB(t)
	return svc, td
}

func newSvcWithDB(t *testing.T) (*cloudkey.Service, *gorm.DB, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	return cloudkey.NewService(cloudkey.NewRepo(gdb), passthroughCipher{}, audit.NewRecorder(gdb)), gdb, td
}

type fakeAccountsInUseCounter struct {
	n     int64
	err   error
	gotID uint64
	calls int
}

func (f *fakeAccountsInUseCounter) CountByCloudKeyID(_ context.Context, id uint64) (int64, error) {
	f.gotID = id
	f.calls++
	return f.n, f.err
}

func newReq() cloudkey.CreateRequest {
	return cloudkey.CreateRequest{
		Name: "k", Provider: "aws", Region: "us-east-1",
		AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret-value-123",
	}
}

func TestService_Create_RoundTrip(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	d, err := svc.Create(context.Background(), 0, "", "", newReq())
	require.NoError(t, err)
	require.NotZero(t, d.ID)
	require.Equal(t, "aws", d.Provider)
}

func TestService_Create_NameTaken(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	_, err := svc.Create(ctx, 0, "", "", newReq())
	require.NoError(t, err)
	_, err = svc.Create(ctx, 0, "", "", newReq())
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, be.Code)
}

func TestService_Get_NotFound(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	_, err := svc.Get(context.Background(), 99999)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_Exists(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	detail, err := svc.Create(context.Background(), 0, "", "", newReq())
	require.NoError(t, err)
	exists, err := svc.Exists(context.Background(), detail.ID)
	require.NoError(t, err)
	require.True(t, exists)
	exists, err = svc.Exists(context.Background(), detail.ID+1)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestService_Update_PartialAndRotate(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", newReq())
	require.NoError(t, err)

	newRegion := "eu-west-1"
	_, err = svc.Update(ctx, 0, "", "", d.ID, cloudkey.UpdateRequest{Region: &newRegion})
	require.NoError(t, err)
	got, _ := svc.Get(ctx, d.ID)
	require.Equal(t, "eu-west-1", got.Region)
	require.Equal(t, "aws", got.Provider) // unchanged

	// Rotate secret only.
	newSecret := "rotated-secret"
	_, err = svc.Update(ctx, 0, "", "", d.ID, cloudkey.UpdateRequest{SecretAccessKey: &newSecret})
	require.NoError(t, err)

	rec, err := svc.Consume(ctx, nil, d.ID, "system:test")
	require.NoError(t, err)
	require.Equal(t, "rotated-secret", rec.SecretAccessKey)
	require.Equal(t, "AKIAEXAMPLE", rec.AccessKeyID) // unrotated
}

func TestService_Delete(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", newReq())
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, 0, "", "", d.ID))
	_, err = svc.Get(ctx, d.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	err := svc.Delete(context.Background(), 0, "", "", 99999)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_Delete_RefusesWhenCloudAccountReferencesKey(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	detail, err := svc.Create(ctx, 0, "", "", newReq())
	require.NoError(t, err)
	counter := &fakeAccountsInUseCounter{n: 2}
	svc.SetAccountsInUseCounter(counter)

	err = svc.Delete(ctx, 0, "", "", detail.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAssetsCloudAccountInUse, be.Code)
	require.Equal(t, "assets.cloud_account.in_use", be.MessageKey)
	require.Equal(t, detail.ID, counter.gotID)
	require.Equal(t, 1, counter.calls)
	_, err = svc.Get(ctx, detail.ID)
	require.NoError(t, err)
}

func TestService_Delete_AllowsWhenCloudAccountCounterIsZero(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	detail, err := svc.Create(ctx, 0, "", "", newReq())
	require.NoError(t, err)
	counter := &fakeAccountsInUseCounter{}
	svc.SetAccountsInUseCounter(counter)

	require.NoError(t, svc.Delete(ctx, 0, "", "", detail.ID))
	require.Equal(t, detail.ID, counter.gotID)
	_, err = svc.Get(ctx, detail.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_Delete_AllowsNilCloudAccountCounter(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	detail, err := svc.Create(ctx, 0, "", "", newReq())
	require.NoError(t, err)
	svc.SetAccountsInUseCounter(nil)

	require.NoError(t, svc.Delete(ctx, 0, "", "", detail.ID))
}

func TestService_Delete_PropagatesCloudAccountCounterErrorWithoutDeleteOrAudit(t *testing.T) {
	svc, gdb, td := newSvcWithDB(t)
	defer td()
	ctx := context.Background()
	detail, err := svc.Create(ctx, 0, "", "", newReq())
	require.NoError(t, err)
	counterErr := errors.New("count cloud account references")
	counter := &fakeAccountsInUseCounter{err: counterErr}
	svc.SetAccountsInUseCounter(counter)

	err = svc.Delete(ctx, 0, "", "", detail.ID)
	require.ErrorIs(t, err, counterErr)
	require.Equal(t, detail.ID, counter.gotID)
	require.Equal(t, 1, counter.calls)
	_, err = svc.Get(ctx, detail.ID)
	require.NoError(t, err)
	var deleteAudits int64
	require.NoError(t, gdb.Model(&models.AuditLog{}).
		Where("action = ? AND target_type = ?", "credentials.delete", "credentials.cloud_key").
		Count(&deleteAudits).Error)
	require.Zero(t, deleteAudits)
}

func TestService_Consume_System(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", newReq())
	require.NoError(t, err)
	rec, err := svc.Consume(ctx, nil, d.ID, "system:smoke")
	require.NoError(t, err)
	require.Equal(t, "AKIAEXAMPLE", rec.AccessKeyID)
	require.Equal(t, "secret-value-123", rec.SecretAccessKey)
}

func TestService_Consume_SystemRequiresPrefix(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	d, _ := svc.Create(ctx, 0, "", "", newReq())
	_, err := svc.Consume(ctx, nil, d.ID, "no-prefix")
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, "credentials.system_purpose_required", be.MessageKey)
}

func TestService_List(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		req := newReq()
		req.Name = "l" + string(rune('a'+i))
		_, err := svc.Create(ctx, 0, "", "", req)
		require.NoError(t, err)
	}
	res, err := svc.List(ctx, cloudkey.ListQuery{Page: 1, PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, int64(3), res.Total)
	require.Len(t, res.Items, 2)
}
