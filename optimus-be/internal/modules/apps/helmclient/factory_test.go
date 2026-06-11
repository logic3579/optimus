package helmclient_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/apps/helmclient"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/k8s/cluster"
)

// --- fakes -----------------------------------------------------------------

// fakeConsumer implements credentials.Consumer. SSHKey and CloudKey panic
// because the helmclient code path must never reach them — only GetKubeconfig
// is in scope. If a future caller starts asking for SSH / cloud through the
// helm factory, the panic flags the regression loudly.
type fakeConsumer struct {
	kc  *credentials.Kubeconfig
	err error
}

func (f *fakeConsumer) GetSSHKey(context.Context, uint64, string) (*credentials.SSHKey, error) {
	panic("helmclient: fakeConsumer.GetSSHKey called — not expected")
}

func (f *fakeConsumer) GetKubeconfig(_ context.Context, _ uint64, _ string) (*credentials.Kubeconfig, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.kc, nil
}

func (f *fakeConsumer) GetCloudKey(context.Context, uint64, string) (*credentials.CloudKey, error) {
	panic("helmclient: fakeConsumer.GetCloudKey called — not expected")
}

// fakeClusters implements helmclient.ClusterLookup.
type fakeClusters struct {
	c   *cluster.Detail
	err error
}

func (f *fakeClusters) Get(_ context.Context, _ uint64) (*cluster.Detail, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.c, nil
}

// --- helpers ---------------------------------------------------------------

const validKubeconfig = `apiVersion: v1
kind: Config
current-context: test-ctx
clusters:
- name: test-cluster
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: test-ctx
  context:
    cluster: test-cluster
    user: test-user
    namespace: default
users:
- name: test-user
  user:
    token: fake-token
`

// kubeconfigWithExec triggers the P1 exec-plugin rejection rule.
const kubeconfigWithExec = `apiVersion: v1
kind: Config
current-context: test-ctx
clusters:
- name: test-cluster
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: test-ctx
  context:
    cluster: test-cluster
    user: test-user
    namespace: default
users:
- name: test-user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: /bin/echo
      args: ["hi"]
`

// kubeconfigWithAuthProvider triggers the P1 auth-provider-plugin rejection rule.
const kubeconfigWithAuthProvider = `apiVersion: v1
kind: Config
current-context: test-ctx
clusters:
- name: test-cluster
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: test-ctx
  context:
    cluster: test-cluster
    user: test-user
    namespace: default
users:
- name: test-user
  user:
    auth-provider:
      name: gcp
`

func newOKFactory() *helmclient.Factory {
	return helmclient.NewFactory(
		&fakeConsumer{kc: &credentials.Kubeconfig{
			Name: "kc1", DefaultNamespace: "default", YAML: []byte(validKubeconfig),
		}},
		&fakeClusters{c: &cluster.Detail{
			ID: 1, Name: "c1", KubeconfigID: 7, Context: "test-ctx",
		}},
	)
}

// --- tests -----------------------------------------------------------------

func TestNewFactory_PanicsOnNilSeams(t *testing.T) {
	cases := []struct {
		name  string
		setup func()
	}{
		{"nil consumer", func() { _ = helmclient.NewFactory(nil, &fakeClusters{}) }},
		{"nil clusters", func() {
			_ = helmclient.NewFactory(&fakeConsumer{}, nil)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic, got none")
				}
			}()
			c.setup()
		})
	}
}

func TestFactory_NewForCluster_Success(t *testing.T) {
	f := newOKFactory()
	cfg, err := f.NewForCluster(context.Background(), 1, "default", "test:smoke")
	if err != nil {
		t.Fatalf("NewForCluster: %v", err)
	}
	if cfg == nil {
		t.Fatal("nil action.Configuration")
	}
	if cfg.KubeClient == nil {
		t.Error("KubeClient unset")
	}
	if cfg.Releases == nil {
		t.Error("Releases storage unset")
	}
}

func TestFactory_NewForCluster_ClusterLookupErr(t *testing.T) {
	want := errors.New("cluster gone")
	f := helmclient.NewFactory(
		&fakeConsumer{},
		&fakeClusters{err: want},
	)
	_, err := f.NewForCluster(context.Background(), 99, "default", "test:smoke")
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want errors.Is %v", err, want)
	}
}

func TestFactory_NewForCluster_ConsumerErr(t *testing.T) {
	want := errors.New("vault offline")
	f := helmclient.NewFactory(
		&fakeConsumer{err: want},
		&fakeClusters{c: &cluster.Detail{ID: 1, KubeconfigID: 7, Context: "test-ctx"}},
	)
	_, err := f.NewForCluster(context.Background(), 1, "default", "test:smoke")
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want errors.Is %v", err, want)
	}
}

func TestFactory_NewForCluster_RejectsExecPlugin(t *testing.T) {
	f := helmclient.NewFactory(
		&fakeConsumer{kc: &credentials.Kubeconfig{
			Name: "kc1", YAML: []byte(kubeconfigWithExec),
		}},
		&fakeClusters{c: &cluster.Detail{ID: 1, KubeconfigID: 7, Context: "test-ctx"}},
	)
	_, err := f.NewForCluster(context.Background(), 1, "default", "test:smoke")
	if err == nil {
		t.Fatal("expected error for exec plugin, got nil")
	}
	be, ok := apperr.AsBiz(err)
	if !ok {
		t.Fatalf("not BizError: %v", err)
	}
	if be.Code != apperr.CodeValidation {
		t.Errorf("got code %d, want %d (CodeValidation)", be.Code, apperr.CodeValidation)
	}
	if !strings.Contains(strings.ToLower(be.Error()), "exec") {
		t.Errorf("error text does not mention exec: %q", be.Error())
	}
}

func TestFactory_NewForCluster_RejectsAuthProviderPlugin(t *testing.T) {
	f := helmclient.NewFactory(
		&fakeConsumer{kc: &credentials.Kubeconfig{
			Name: "kc1", YAML: []byte(kubeconfigWithAuthProvider),
		}},
		&fakeClusters{c: &cluster.Detail{ID: 1, KubeconfigID: 7, Context: "test-ctx"}},
	)
	_, err := f.NewForCluster(context.Background(), 1, "default", "test:smoke")
	if err == nil {
		t.Fatal("expected error for auth-provider plugin, got nil")
	}
	be, ok := apperr.AsBiz(err)
	if !ok {
		t.Fatalf("not BizError: %v", err)
	}
	if be.Code != apperr.CodeValidation {
		t.Errorf("got code %d, want %d (CodeValidation)", be.Code, apperr.CodeValidation)
	}
}

func TestFactory_NewForCluster_RejectsBadYAML(t *testing.T) {
	f := helmclient.NewFactory(
		&fakeConsumer{kc: &credentials.Kubeconfig{
			Name: "kc1", YAML: []byte("not a kubeconfig: ::: invalid"),
		}},
		&fakeClusters{c: &cluster.Detail{ID: 1, KubeconfigID: 7, Context: "test-ctx"}},
	)
	_, err := f.NewForCluster(context.Background(), 1, "default", "test:smoke")
	if err == nil {
		t.Fatal("expected error for bad YAML, got nil")
	}
}

func TestFactory_NewForCluster_MissingContext(t *testing.T) {
	f := helmclient.NewFactory(
		&fakeConsumer{kc: &credentials.Kubeconfig{
			Name: "kc1", YAML: []byte(validKubeconfig),
		}},
		&fakeClusters{c: &cluster.Detail{ID: 1, KubeconfigID: 7, Context: "does-not-exist"}},
	)
	_, err := f.NewForCluster(context.Background(), 1, "default", "test:smoke")
	if err == nil {
		t.Fatal("expected error for missing context, got nil")
	}
	be, ok := apperr.AsBiz(err)
	if !ok {
		t.Fatalf("not BizError: %v", err)
	}
	if be.Code != apperr.CodeValidation {
		t.Errorf("got code %d, want %d (CodeValidation)", be.Code, apperr.CodeValidation)
	}
}
