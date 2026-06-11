package helmclient

import (
	"testing"

	"k8s.io/client-go/rest"
)

// TestRESTClientGetter_Conformance asserts the four methods of the
// genericclioptions.RESTClientGetter contract that helm SDK requires return
// non-nil values for a minimal *rest.Config. We don't hit the network — the
// discovery client is constructed lazily.
func TestRESTClientGetter_Conformance(t *testing.T) {
	cfg := &rest.Config{Host: "https://127.0.0.1:6443"}
	g := newRESTClientGetter(cfg, "default")

	t.Run("ToRESTConfig returns the same config", func(t *testing.T) {
		got, err := g.ToRESTConfig()
		if err != nil {
			t.Fatalf("ToRESTConfig: %v", err)
		}
		if got == nil {
			t.Fatal("ToRESTConfig returned nil")
		}
		if got.Host != "https://127.0.0.1:6443" {
			t.Errorf("Host=%q, want https://127.0.0.1:6443", got.Host)
		}
	})

	t.Run("ToDiscoveryClient builds a cached client", func(t *testing.T) {
		disc, err := g.ToDiscoveryClient()
		if err != nil {
			t.Fatalf("ToDiscoveryClient: %v", err)
		}
		if disc == nil {
			t.Fatal("ToDiscoveryClient returned nil")
		}
	})

	t.Run("ToRESTMapper wraps the discovery client", func(t *testing.T) {
		mapper, err := g.ToRESTMapper()
		if err != nil {
			t.Fatalf("ToRESTMapper: %v", err)
		}
		if mapper == nil {
			t.Fatal("ToRESTMapper returned nil")
		}
	})

	t.Run("ToRawKubeConfigLoader honours the namespace", func(t *testing.T) {
		loader := g.ToRawKubeConfigLoader()
		if loader == nil {
			t.Fatal("ToRawKubeConfigLoader returned nil")
		}
		ns, _, err := loader.Namespace()
		if err != nil {
			t.Fatalf("loader.Namespace: %v", err)
		}
		if ns != "default" {
			t.Errorf("namespace=%q, want default", ns)
		}
	})
}

// TestRESTClientGetter_NamespaceOverride changes the namespace and verifies
// the override propagates through ToRawKubeConfigLoader.
func TestRESTClientGetter_NamespaceOverride(t *testing.T) {
	cfg := &rest.Config{Host: "https://127.0.0.1:6443"}
	g := newRESTClientGetter(cfg, "kube-system")
	loader := g.ToRawKubeConfigLoader()
	ns, _, err := loader.Namespace()
	if err != nil {
		t.Fatalf("loader.Namespace: %v", err)
	}
	if ns != "kube-system" {
		t.Errorf("namespace=%q, want kube-system", ns)
	}
}

// TestDebugLog_Smoke pokes the debug log bridge — we only assert it does not
// panic and accepts the helm SDK's format string + variadic args shape.
func TestDebugLog_Smoke(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("debugLog panicked: %v", r)
		}
	}()
	debugLog("hello %s %d", "world", 42)
}

// TestBuildRESTConfig_BadYAML asserts clientcmd.Load reports an error for
// non-YAML input, exercising the parse-failure path inside buildRESTConfig.
func TestBuildRESTConfig_BadYAML(t *testing.T) {
	_, err := buildRESTConfig([]byte("not: a: valid: kubeconfig: ::"), "ignored")
	if err == nil {
		t.Fatal("expected error from buildRESTConfig with bad YAML, got nil")
	}
}
