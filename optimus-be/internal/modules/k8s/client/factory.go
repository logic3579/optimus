// Package client builds per-request Kubernetes clientsets from credentials.
// Nothing is cached. Every call walks: cluster repo -> Consumer -> clientcmd
// -> kubernetes.NewForConfig. Discarding clientsets per-request avoids the
// connection-pool / credential-staleness foot-guns described in P2 spec §6.3
// and keeps the audit trail tight (every apiserver call traces back to one
// fresh GetKubeconfig consume).
package client

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/k8s/cluster"
)

const defaultRequestTimeout = 10 * time.Second

// ReadRepo is the subset of cluster.Repo the Factory depends on. Defined as a
// local interface so this package is free of the internal/models import — the
// repoAdapter below bridges to cluster.Repo's concrete *models.Cluster.
type ReadRepo interface {
	Get(ctx context.Context, id uint64) (*ClusterMeta, error)
}

// ClusterMeta is the minimum the Factory needs from a cluster row.
type ClusterMeta struct {
	ID           uint64
	KubeconfigID uint64
	Context      string
}

// repoAdapter bridges cluster.Repo (which returns *models.Cluster) to our
// local ClusterMeta type so this package can stay free of internal/models.
type repoAdapter struct{ inner *cluster.Repo }

func (a repoAdapter) Get(ctx context.Context, id uint64) (*ClusterMeta, error) {
	m, err := a.inner.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &ClusterMeta{ID: m.ID, KubeconfigID: m.KubeconfigID, Context: m.Context}, nil
}

// NewRepoAdapter wraps a cluster.Repo for use by Factory. Composition root
// (cmd/server/main.go) calls this to bridge the two seams.
func NewRepoAdapter(r *cluster.Repo) ReadRepo { return repoAdapter{inner: r} }

// Factory builds per-request rest.Config + kubernetes.Interface from a
// Consumer-fetched kubeconfig. Holds no per-cluster state; safe to share
// across all handlers.
type Factory struct {
	consumer credentials.Consumer
	repo     ReadRepo
}

// NewFactory constructs a Factory bound to the given Consumer (P1 seam) and
// cluster read repo.
func NewFactory(consumer credentials.Consumer, repo ReadRepo) *Factory {
	return &Factory{consumer: consumer, repo: repo}
}

// RestConfig builds a fresh *rest.Config for the cluster. Does NOT set a
// per-request Timeout — callers (Clientset / ClientsetForStream) wrap as
// appropriate. `purpose` is forwarded to Consumer.GetKubeconfig for audit.
func (f *Factory) RestConfig(ctx context.Context, clusterID uint64, purpose string) (*rest.Config, error) {
	m, err := f.repo.Get(ctx, clusterID)
	if err != nil {
		return nil, err // cluster.not_found mapped upstream by service layer
	}
	kc, err := f.consumer.GetKubeconfig(ctx, m.KubeconfigID, purpose)
	if err != nil {
		return nil, err
	}
	return buildRestConfig(kc.YAML, m.Context)
}

// Clientset returns a kubernetes.Interface with a 10s request timeout — for
// regular List / Get / etc. See spec §6.3.
func (f *Factory) Clientset(ctx context.Context, clusterID uint64, purpose string) (kubernetes.Interface, error) {
	cfg, err := f.RestConfig(ctx, clusterID, purpose)
	if err != nil {
		return nil, err
	}
	cfg.Timeout = defaultRequestTimeout
	return kubernetes.NewForConfig(cfg)
}

// ClientsetForStream returns a kubernetes.Interface WITHOUT a per-request
// timeout — for SSE log streams whose lifetime is bounded by client
// disconnect (the gin handler watches ctx.Done()).
func (f *Factory) ClientsetForStream(ctx context.Context, clusterID uint64, purpose string) (kubernetes.Interface, error) {
	cfg, err := f.RestConfig(ctx, clusterID, purpose)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

// Discover returns a cluster.VersionProbe for the cluster — used only by
// cluster.Service.Ping. Returns the narrow VersionProbe interface (just
// ServerVersion()) rather than the full discovery.DiscoveryInterface so this
// package keeps its surface tight; a real *discovery.DiscoveryClient
// satisfies VersionProbe structurally.
//
// Satisfies cluster.DiscoveryFunc via Go structural typing — the composition
// root passes cluster.DiscoveryFunc(factory.Discover) to NewService.
func (f *Factory) Discover(ctx context.Context, clusterID uint64, purpose string) (cluster.VersionProbe, error) {
	cs, err := f.Clientset(ctx, clusterID, purpose)
	if err != nil {
		return nil, err
	}
	return cs.Discovery(), nil
}

// buildRestConfig parses a kubeconfig YAML, runs the shared exec /
// auth-provider / context check, and returns a rest.Config bound to the
// requested context. The validation step is the same one Create/Update
// runs — repeated here as defense-in-depth so a maliciously crafted DB row
// (e.g., row tampered after upload) still fails before we issue a request.
func buildRestConfig(y []byte, contextName string) (*rest.Config, error) {
	if err := cluster.ValidateContextAndAuth(y, contextName); err != nil {
		return nil, err
	}
	apiCfg, err := clientcmd.Load(y)
	if err != nil {
		// ValidateContextAndAuth already covers Load's failure mode, but
		// guard here too so the call site stays self-contained.
		return nil, apperr.New(apperr.CodeValidation, "k8s.kubeconfig.invalid", err.Error())
	}
	apiCfg.CurrentContext = contextName
	return clientcmd.NewDefaultClientConfig(*apiCfg, &clientcmd.ConfigOverrides{}).ClientConfig()
}
