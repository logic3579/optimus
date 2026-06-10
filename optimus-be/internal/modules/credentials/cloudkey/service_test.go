//go:build dbtest

package cloudkey_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
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
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	return cloudkey.NewService(cloudkey.NewRepo(gdb), passthroughCipher{}, audit.NewRecorder(gdb)), td
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
