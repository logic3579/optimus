package helmclient

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// restClientGetter is the minimal genericclioptions.RESTClientGetter that
// helm SDK requires. It owns a fresh *rest.Config and a namespace; nothing
// is cached across calls (deliberately — helm action.Configuration is also
// per-request).
type restClientGetter struct {
	cfg       *rest.Config
	namespace string
}

func newRESTClientGetter(cfg *rest.Config, namespace string) genericclioptions.RESTClientGetter {
	return &restClientGetter{cfg: cfg, namespace: namespace}
}

func (g *restClientGetter) ToRESTConfig() (*rest.Config, error) {
	return g.cfg, nil
}

func (g *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	disc, err := discovery.NewDiscoveryClientForConfig(g.cfg)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(disc), nil
}

func (g *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	d, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(d)
	return restmapper.NewShortcutExpander(mapper, d, nil), nil
}

func (g *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return clientcmd.NewDefaultClientConfig(*clientcmdapi.NewConfig(), &clientcmd.ConfigOverrides{
		Context: clientcmdapi.Context{Namespace: g.namespace},
	})
}
