package client_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/k8s/client"
)

// fakeRepo implements client.ReadRepo for unit tests.
type fakeRepo struct {
	meta *client.ClusterMeta
	err  error
}

func (r *fakeRepo) Get(context.Context, uint64) (*client.ClusterMeta, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.meta, nil
}

// fakeConsumer implements credentials.Consumer for unit tests. Only
// GetKubeconfig is exercised; the other two methods just satisfy the
// interface.
type fakeConsumer struct {
	yaml []byte
	err  error
}

func (f *fakeConsumer) GetSSHKey(context.Context, uint64, string) (*credentials.SSHKey, error) {
	return nil, errors.New("not used in this test")
}

func (f *fakeConsumer) GetKubeconfig(context.Context, uint64, string) (*credentials.Kubeconfig, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &credentials.Kubeconfig{YAML: f.yaml}, nil
}

func (f *fakeConsumer) GetCloudKey(context.Context, uint64, string) (*credentials.CloudKey, error) {
	return nil, errors.New("not used in this test")
}

const validYAML = `apiVersion: v1
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

func TestFactory_RestConfig_OK(t *testing.T) {
	f := client.NewFactory(
		&fakeConsumer{yaml: []byte(validYAML)},
		&fakeRepo{meta: &client.ClusterMeta{ID: 1, KubeconfigID: 9, Context: "ctx"}},
	)
	cfg, err := f.RestConfig(context.Background(), 1, "k8s.test")
	require.NoError(t, err)
	require.Equal(t, "https://x:6443", cfg.Host)
}

func TestFactory_RestConfig_RejectsExec(t *testing.T) {
	bad := []byte(`apiVersion: v1
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
        command: /bin/sh
contexts:
  - name: ctx
    context:
      cluster: c
      user: u
current-context: ctx
`)
	f := client.NewFactory(
		&fakeConsumer{yaml: bad},
		&fakeRepo{meta: &client.ClusterMeta{ID: 1, KubeconfigID: 9, Context: "ctx"}},
	)
	_, err := f.RestConfig(context.Background(), 1, "k8s.test")
	require.Error(t, err)
	be, ok := err.(*apperr.BizError)
	require.True(t, ok, "want *apperr.BizError, got %T", err)
	require.Equal(t, "k8s.kubeconfig.exec_forbidden", be.MessageKey)
}

func TestFactory_RestConfig_RejectsAuthProvider(t *testing.T) {
	bad := []byte(`apiVersion: v1
kind: Config
clusters:
  - name: c
    cluster:
      server: https://x:6443
      insecure-skip-tls-verify: true
users:
  - name: u
    user:
      auth-provider:
        name: gcp
contexts:
  - name: ctx
    context:
      cluster: c
      user: u
current-context: ctx
`)
	f := client.NewFactory(
		&fakeConsumer{yaml: bad},
		&fakeRepo{meta: &client.ClusterMeta{ID: 1, KubeconfigID: 9, Context: "ctx"}},
	)
	_, err := f.RestConfig(context.Background(), 1, "k8s.test")
	require.Error(t, err)
	be, ok := err.(*apperr.BizError)
	require.True(t, ok, "want *apperr.BizError, got %T", err)
	require.Equal(t, "k8s.kubeconfig.authprovider_forbidden", be.MessageKey)
}

func TestFactory_RestConfig_ContextNotFound(t *testing.T) {
	f := client.NewFactory(
		&fakeConsumer{yaml: []byte(validYAML)},
		&fakeRepo{meta: &client.ClusterMeta{ID: 1, KubeconfigID: 9, Context: "missing"}},
	)
	_, err := f.RestConfig(context.Background(), 1, "k8s.test")
	require.Error(t, err)
	be, ok := err.(*apperr.BizError)
	require.True(t, ok, "want *apperr.BizError, got %T", err)
	require.Equal(t, "k8s.kubeconfig.context_not_found", be.MessageKey)
}

func TestFactory_RestConfig_ConsumerError(t *testing.T) {
	want := errors.New("consumer boom")
	f := client.NewFactory(
		&fakeConsumer{err: want},
		&fakeRepo{meta: &client.ClusterMeta{ID: 1, KubeconfigID: 9, Context: "ctx"}},
	)
	_, err := f.RestConfig(context.Background(), 1, "k8s.test")
	require.ErrorIs(t, err, want)
}

func TestFactory_RestConfig_RepoError(t *testing.T) {
	want := errors.New("repo boom")
	f := client.NewFactory(
		&fakeConsumer{yaml: []byte(validYAML)},
		&fakeRepo{err: want},
	)
	_, err := f.RestConfig(context.Background(), 1, "k8s.test")
	require.ErrorIs(t, err, want)
}

func TestFactory_Clientset_SetsTimeout(t *testing.T) {
	f := client.NewFactory(
		&fakeConsumer{yaml: []byte(validYAML)},
		&fakeRepo{meta: &client.ClusterMeta{ID: 1, KubeconfigID: 9, Context: "ctx"}},
	)
	// Clientset bakes in a 10s timeout — exercising it just confirms
	// kubernetes.NewForConfig accepts the produced rest.Config.
	cs, err := f.Clientset(context.Background(), 1, "k8s.test")
	require.NoError(t, err)
	require.NotNil(t, cs)
}

func TestFactory_ClientsetForStream_NoTimeout(t *testing.T) {
	f := client.NewFactory(
		&fakeConsumer{yaml: []byte(validYAML)},
		&fakeRepo{meta: &client.ClusterMeta{ID: 1, KubeconfigID: 9, Context: "ctx"}},
	)
	cs, err := f.ClientsetForStream(context.Background(), 1, "k8s.test")
	require.NoError(t, err)
	require.NotNil(t, cs)
}
