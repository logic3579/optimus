//go:build dbtest

package cluster_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/version"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/k8s/cluster"
)

// goodYAML is a minimal valid kubeconfig with one context ("ctx") and a
// token-based user (no exec / no auth-provider).
const goodYAML = `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: https://x:6443
    insecure-skip-tls-verify: true
users:
- name: u
  user:
    token: t
contexts:
- name: ctx
  context:
    cluster: c
    user: u
current-context: ctx
`

const execYAML = `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: https://x:6443
    insecure-skip-tls-verify: true
users:
- name: u
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: /bin/echo
      args: [pwned]
contexts:
- name: ctx
  context:
    cluster: c
    user: u
current-context: ctx
`

// fakeConsumer implements credentials.Consumer in-memory. Only GetKubeconfig
// is exercised; the other two methods are stubs.
type fakeConsumer struct {
	yaml []byte
	err  error
}

func (f *fakeConsumer) GetSSHKey(context.Context, uint64, string) (*credentials.SSHKey, error) {
	return nil, errors.New("not used")
}
func (f *fakeConsumer) GetKubeconfig(_ context.Context, _ uint64, _ string) (*credentials.Kubeconfig, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &credentials.Kubeconfig{Name: "kc", YAML: f.yaml}, nil
}
func (f *fakeConsumer) GetCloudKey(context.Context, uint64, string) (*credentials.CloudKey, error) {
	return nil, errors.New("not used")
}

// fakeVersionProbe satisfies cluster.VersionProbe.
type fakeVersionProbe struct {
	v   *version.Info
	err error
}

func (f *fakeVersionProbe) ServerVersion() (*version.Info, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.v, nil
}

// fakeProber satisfies cluster.Prober and returns a constant VersionProbe.
type fakeProber struct {
	p   cluster.VersionProbe
	err error
}

func (p *fakeProber) Discover(context.Context, uint64, string) (cluster.VersionProbe, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.p, nil
}

func newSvc(t *testing.T, yaml []byte, prober cluster.Prober) (*cluster.Service, uint64, func()) {
	t.Helper()
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	repo := cluster.NewRepo(gdb)
	rec := audit.NewRecorder(gdb)
	kc := &models.CredentialKubeconfig{Name: "kc", KubeconfigEnc: []byte{1}}
	require.NoError(t, gdb.Create(kc).Error)
	svc := cluster.NewService(repo, &fakeConsumer{yaml: yaml}, prober, rec)
	return svc, kc.ID, td
}

func TestService_Create_Validates_ContextMissing(t *testing.T) {
	svc, kcID, td := newSvc(t, []byte(goodYAML), nil)
	defer td()

	_, err := svc.Create(context.Background(), 0, "", "", cluster.CreateRequest{
		Name: "x", KubeconfigID: kcID, Context: "missing",
	})
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeValidation, be.Code)
	require.Equal(t, "k8s.kubeconfig.context_not_found", be.MessageKey)
}

func TestService_Create_Rejects_ExecPlugin(t *testing.T) {
	svc, kcID, td := newSvc(t, []byte(execYAML), nil)
	defer td()

	_, err := svc.Create(context.Background(), 0, "", "", cluster.CreateRequest{
		Name: "x", KubeconfigID: kcID, Context: "ctx",
	})
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, "k8s.kubeconfig.exec_forbidden", be.MessageKey)
}

func TestService_Create_NameConflict(t *testing.T) {
	svc, kcID, td := newSvc(t, []byte(goodYAML), nil)
	defer td()
	ctx := context.Background()

	_, err := svc.Create(ctx, 0, "", "", cluster.CreateRequest{
		Name: "dup", KubeconfigID: kcID, Context: "ctx",
	})
	require.NoError(t, err)
	_, err = svc.Create(ctx, 0, "", "", cluster.CreateRequest{
		Name: "dup", KubeconfigID: kcID, Context: "ctx",
	})
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, be.Code)
	require.Equal(t, "k8s.cluster.name_taken", be.MessageKey)
}

func TestService_Update_PartialAndTags(t *testing.T) {
	svc, kcID, td := newSvc(t, []byte(goodYAML), nil)
	defer td()
	ctx := context.Background()

	det, err := svc.Create(ctx, 0, "", "", cluster.CreateRequest{
		Name: "u1", KubeconfigID: kcID, Context: "ctx", Tags: []string{"alpha"},
	})
	require.NoError(t, err)

	desc := "the prod cluster"
	newTags := []string{"prod", "us-east"}
	out, err := svc.Update(ctx, 0, "", "", det.ID, cluster.UpdateRequest{
		Description: &desc,
		Tags:        &newTags,
	})
	require.NoError(t, err)
	require.Equal(t, "the prod cluster", out.Description)
	require.Equal(t, []string{"prod", "us-east"}, out.Tags)
}

func TestService_Ping_OK(t *testing.T) {
	prober := &fakeProber{p: &fakeVersionProbe{v: &version.Info{GitVersion: "v1.30.5"}}}
	svc, kcID, td := newSvc(t, []byte(goodYAML), prober)
	defer td()
	ctx := context.Background()

	det, err := svc.Create(ctx, 0, "", "", cluster.CreateRequest{
		Name: "p", KubeconfigID: kcID, Context: "ctx",
	})
	require.NoError(t, err)
	pr, err := svc.Ping(ctx, 0, "", "", det.ID)
	require.NoError(t, err)
	require.True(t, pr.OK)
	require.Equal(t, "v1.30.5", pr.ServerVersion)

	// health row should be persisted.
	got, err := svc.Get(ctx, det.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastHealthOK)
	require.True(t, *got.LastHealthOK)
}

func TestService_Ping_DiscoveryErrUpdatesHealth(t *testing.T) {
	prober := &fakeProber{p: &fakeVersionProbe{err: errors.New("dial tcp: refused")}}
	svc, kcID, td := newSvc(t, []byte(goodYAML), prober)
	defer td()
	ctx := context.Background()

	det, _ := svc.Create(ctx, 0, "", "", cluster.CreateRequest{
		Name: "down", KubeconfigID: kcID, Context: "ctx",
	})
	pr, err := svc.Ping(ctx, 0, "", "", det.ID)
	require.NoError(t, err)
	require.False(t, pr.OK)
	require.Contains(t, pr.Message, "refused")

	got, err := svc.Get(ctx, det.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastHealthOK)
	require.False(t, *got.LastHealthOK)
	require.Contains(t, got.LastHealthMsg, "refused")
}

func TestService_Ping_NoProber(t *testing.T) {
	svc, kcID, td := newSvc(t, []byte(goodYAML), nil)
	defer td()
	ctx := context.Background()
	det, _ := svc.Create(ctx, 0, "", "", cluster.CreateRequest{
		Name: "np", KubeconfigID: kcID, Context: "ctx",
	})
	pr, err := svc.Ping(ctx, 0, "", "", det.ID)
	require.NoError(t, err)
	require.False(t, pr.OK)
	require.Equal(t, "prober not configured", pr.Message)
}

// fakeAppsCounter satisfies cluster.AppsApplicationCounter for the P3
// delete-refused test. n is the count returned for any cluster id.
type fakeAppsCounter struct {
	n   int
	err error
}

func (f *fakeAppsCounter) CountByClusterID(_ context.Context, _ uint64) (int, error) {
	return f.n, f.err
}

func TestService_Delete_RefusedWhenAppsReference(t *testing.T) {
	svc, kcID, td := newSvc(t, []byte(goodYAML), nil)
	defer td()
	ctx := context.Background()
	det, err := svc.Create(ctx, 0, "", "", cluster.CreateRequest{
		Name: "in-use", KubeconfigID: kcID, Context: "ctx",
	})
	require.NoError(t, err)

	svc.SetAppsCounter(&fakeAppsCounter{n: 3})
	err = svc.Delete(ctx, 0, "", "", det.ID)
	require.Error(t, err)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeAppsApplicationInUse, be.Code)
	require.Equal(t, "k8s.cluster.in_use_by_apps", be.MessageKey)

	// Re-checking with n=0 should allow the delete.
	svc.SetAppsCounter(&fakeAppsCounter{n: 0})
	require.NoError(t, svc.Delete(ctx, 0, "", "", det.ID))
}

func TestService_Delete(t *testing.T) {
	svc, kcID, td := newSvc(t, []byte(goodYAML), nil)
	defer td()
	ctx := context.Background()
	det, err := svc.Create(ctx, 0, "", "", cluster.CreateRequest{
		Name: "d", KubeconfigID: kcID, Context: "ctx",
	})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, 0, "", "", det.ID))
	_, err = svc.Get(ctx, det.ID)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestService_List_FiltersAndPaginates(t *testing.T) {
	// Build a kubeconfig with 3 distinct contexts so the (kubeconfig_id,
	// context) partial unique index doesn't reject the 3 creates.
	multiCtxYAML := `apiVersion: v1
kind: Config
clusters:
- {name: c, cluster: {server: https://x:6443, insecure-skip-tls-verify: true}}
users:
- {name: u, user: {token: t}}
contexts:
- {name: alpha-ctx, context: {cluster: c, user: u}}
- {name: beta-ctx,  context: {cluster: c, user: u}}
- {name: gamma-ctx, context: {cluster: c, user: u}}
current-context: alpha-ctx
`
	svc, kcID, td := newSvc(t, []byte(multiCtxYAML), nil)
	defer td()
	ctx := context.Background()
	for _, n := range []string{"alpha", "beta", "gamma"} {
		_, err := svc.Create(ctx, 0, "", "", cluster.CreateRequest{
			Name: n, KubeconfigID: kcID, Context: n + "-ctx", Tags: []string{n + "-tag"},
		})
		require.NoError(t, err)
	}
	res, err := svc.List(ctx, cluster.ListQuery{Page: 1, PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, int64(3), res.Total)
	require.Len(t, res.Items, 2)

	res, err = svc.List(ctx, cluster.ListQuery{Tag: "beta-tag"})
	require.NoError(t, err)
	require.Equal(t, int64(1), res.Total)
	require.Equal(t, "beta", res.Items[0].Name)
	// Listing should join kubeconfig_name through the preload.
	require.Equal(t, "kc", res.Items[0].KubeconfigName)
}
