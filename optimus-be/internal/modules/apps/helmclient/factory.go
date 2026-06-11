// Package helmclient builds per-request *action.Configuration objects used by
// the apps/release sub-package. helm action.Configuration's internal
// KubeClient holds REST handles and is NOT safe to share across goroutines,
// so the rule is: build per request, discard before the handler returns.
// See P3 spec §7.1.
package helmclient

import (
	"context"
	"fmt"
	"log/slog"

	"helm.sh/helm/v3/pkg/action"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"optimus-be/internal/modules/apps"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/k8s/cluster"
)

// ClusterLookup is the narrow seam Factory needs to resolve a cluster ID into
// its kubeconfig reference and context. Satisfied by *cluster.Service. Defined
// here so tests can inject a tiny fake without spinning up the full cluster
// service + repo + DB.
type ClusterLookup interface {
	Get(ctx context.Context, id uint64) (*cluster.Detail, error)
}

// Factory holds the cross-request seams. Safe to share across handlers — it
// owns no mutable state itself; everything per-request comes off of the
// credentials Consumer and the cluster lookup.
type Factory struct {
	consumer credentials.Consumer
	clusters ClusterLookup
}

// NewFactory wires a Factory. Both seams are required; pass a credentials
// Consumer (typically from credentials.NewConsumer at module wiring time) and
// the cluster service (or any ClusterLookup-compatible value).
func NewFactory(consumer credentials.Consumer, clusters ClusterLookup) *Factory {
	if consumer == nil {
		panic("helmclient: NewFactory: consumer is nil")
	}
	if clusters == nil {
		panic("helmclient: NewFactory: clusters is nil")
	}
	return &Factory{consumer: consumer, clusters: clusters}
}

// NewForCluster returns a fresh *action.Configuration pointed at the given
// cluster with the helm release namespace set to `namespace` and the helm
// storage driver set to "secrets" (helm 3 default). purpose threads through
// to credentials.Consumer for audit attribution (e.g., "apps.release.install").
//
// Errors from upstream (credentials fetch, kubeconfig parse, helm Init) are
// normalised through apps.MapError before being returned, so handlers never
// see raw helm or registry text. cluster.ValidateContextAndAuth errors and
// credentials.Consumer errors are already BizError values and surface as-is.
func (f *Factory) NewForCluster(
	ctx context.Context, clusterID uint64, namespace, purpose string,
) (*action.Configuration, error) {
	// 1. Resolve the cluster row → kubeconfig ID + context name.
	c, err := f.clusters.Get(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	// 2. Decrypt the kubeconfig through the Consumer seam (audit trail).
	kc, err := f.consumer.GetKubeconfig(ctx, c.KubeconfigID, purpose)
	if err != nil {
		return nil, err
	}
	// 3. Defense-in-depth: rerun the P1 kubeconfig validation on every build.
	//    Catches a kubeconfig that was somehow mutated post-create to add an
	//    exec / auth-provider plugin.
	if err := cluster.ValidateContextAndAuth(kc.YAML, c.Context); err != nil {
		return nil, err
	}
	// 4. Parse kubeconfig and bind to the named context.
	restCfg, err := buildRESTConfig(kc.YAML, c.Context)
	if err != nil {
		return nil, apps.MapError(err)
	}
	// 5. Construct the per-request RESTClientGetter and helm action.Configuration.
	rcg := newRESTClientGetter(restCfg, namespace)
	actionCfg := new(action.Configuration)
	if err := actionCfg.Init(rcg, namespace, "secrets", debugLog); err != nil {
		return nil, apps.MapError(err)
	}
	return actionCfg, nil
}

// buildRESTConfig parses the kubeconfig YAML and produces a *rest.Config bound
// to the named context. clientcmd.NewDefaultClientConfig is used (not
// BuildConfigFromKubeconfigGetter) so we never go through file-system loaders.
func buildRESTConfig(y []byte, contextName string) (*rest.Config, error) {
	apiCfg, err := clientcmd.Load(y)
	if err != nil {
		return nil, err
	}
	apiCfg.CurrentContext = contextName
	return clientcmd.NewDefaultClientConfig(*apiCfg, &clientcmd.ConfigOverrides{}).ClientConfig()
}

// debugLog bridges helm SDK verbose output into slog at DEBUG level. It MUST
// never propagate back to the HTTP client — leaking helm internals would
// violate the "no raw error text to clients" rule.
func debugLog(format string, args ...interface{}) {
	slog.Debug("helm", slog.String("msg", fmt.Sprintf(format, args...)))
}
