//go:build dbtest

package kubeconfig_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/kubeconfig"
)

const validKubeconfigYAML = `apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c1
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: ctx
  context:
    cluster: c1
    user: u1
    namespace: default
users:
- name: u1
  user:
    token: abc
`

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

func newSvc(t *testing.T) (*kubeconfig.Service, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	rec := audit.NewRecorder(gdb)
	svc := kubeconfig.NewService(kubeconfig.NewRepo(gdb), passthroughCipher{}, rec)
	return svc, td
}

func TestService_Create_RoundTrip(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	d, err := svc.Create(context.Background(), 0, "", "", kubeconfig.CreateRequest{
		Name: "n1", DefaultNamespace: "default", Kubeconfig: validKubeconfigYAML,
	})
	require.NoError(t, err)
	require.NotZero(t, d.ID)
}

func TestService_Create_InvalidYAML(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	_, err := svc.Create(context.Background(), 0, "", "", kubeconfig.CreateRequest{
		Name: "bad", Kubeconfig: "{not yaml",
	})
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, "credentials.invalid_key_format", be.MessageKey)
}

func TestService_Create_NoContexts(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	_, err := svc.Create(context.Background(), 0, "", "", kubeconfig.CreateRequest{
		Name: "nc", Kubeconfig: "apiVersion: v1\nkind: Config\n",
	})
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, "credentials.invalid_key_format", be.MessageKey)
}

func TestService_Create_NameTaken(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	_, err := svc.Create(ctx, 0, "", "", kubeconfig.CreateRequest{Name: "dup", Kubeconfig: validKubeconfigYAML})
	require.NoError(t, err)
	_, err = svc.Create(ctx, 0, "", "", kubeconfig.CreateRequest{Name: "dup", Kubeconfig: validKubeconfigYAML})
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, be.Code)
}

func TestService_Update_RotateAndPartial(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", kubeconfig.CreateRequest{
		Name: "r1", DefaultNamespace: "default", Kubeconfig: validKubeconfigYAML,
	})
	require.NoError(t, err)

	newNS := "kube-system"
	_, err = svc.Update(ctx, 0, "", "", d.ID, kubeconfig.UpdateRequest{DefaultNamespace: &newNS})
	require.NoError(t, err)
	got, _ := svc.Get(ctx, d.ID)
	require.Equal(t, "kube-system", got.DefaultNamespace)

	// Rotate YAML.
	otherYAML := validKubeconfigYAML // any valid kubeconfig
	_, err = svc.Update(ctx, 0, "", "", d.ID, kubeconfig.UpdateRequest{Kubeconfig: &otherYAML})
	require.NoError(t, err)
}

func TestService_Delete(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", kubeconfig.CreateRequest{Name: "del", Kubeconfig: validKubeconfigYAML})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, 0, "", "", d.ID))
	_, err = svc.Get(ctx, d.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_Consume_System(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	d, err := svc.Create(ctx, 0, "", "", kubeconfig.CreateRequest{Name: "c1", DefaultNamespace: "default", Kubeconfig: validKubeconfigYAML})
	require.NoError(t, err)

	rec, err := svc.Consume(ctx, nil, d.ID, "system:smoke")
	require.NoError(t, err)
	require.Equal(t, validKubeconfigYAML, string(rec.YAML))
	require.Equal(t, "default", rec.DefaultNamespace)
}

func TestService_List_Pagination(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_, err := svc.Create(ctx, 0, "", "", kubeconfig.CreateRequest{
			Name: "l" + string(rune('a'+i)), DefaultNamespace: "default", Kubeconfig: validKubeconfigYAML,
		})
		require.NoError(t, err)
	}
	res, err := svc.List(ctx, kubeconfig.ListQuery{Page: 1, PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, int64(4), res.Total)
	require.Len(t, res.Items, 2)
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	err := svc.Delete(context.Background(), 0, "", "", 99999)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_Consume_RejectsSystemWithoutPrefix(t *testing.T) {
	svc, td := newSvc(t)
	defer td()
	ctx := context.Background()
	d, _ := svc.Create(ctx, 0, "", "", kubeconfig.CreateRequest{Name: "c2", Kubeconfig: validKubeconfigYAML})
	_, err := svc.Consume(ctx, nil, d.ID, "no-prefix")
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, "credentials.system_purpose_required", be.MessageKey)
}
