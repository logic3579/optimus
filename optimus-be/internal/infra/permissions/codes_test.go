package permissions

import (
	"strings"
	"testing"
)

func TestAll_UniqueCodes(t *testing.T) {
	seen := make(map[string]bool, len(All))
	for _, p := range All {
		if seen[p.Code] {
			t.Errorf("duplicate permission code: %s", p.Code)
		}
		seen[p.Code] = true
	}
}

func TestAll_RequiredFieldsPopulated(t *testing.T) {
	for _, p := range All {
		if p.Code == "" {
			t.Errorf("permission missing Code: %+v", p)
		}
		if p.Name == "" {
			t.Errorf("%s: Name (i18n key) empty", p.Code)
		}
		if p.Category == "" {
			t.Errorf("%s: Category empty", p.Code)
		}
		// Code must be category:resource:action.
		if parts := strings.Split(p.Code, ":"); len(parts) != 3 || parts[0] != p.Category {
			t.Errorf("%s: code first segment must match Category %q", p.Code, p.Category)
		}
	}
}

func TestAll_IncludesCredentialPermissions(t *testing.T) {
	want := []string{
		"credentials:ssh_key:read", "credentials:ssh_key:write", "credentials:ssh_key:delete", "credentials:ssh_key:use",
		"credentials:kubeconfig:read", "credentials:kubeconfig:write", "credentials:kubeconfig:delete", "credentials:kubeconfig:use",
		"credentials:cloud_key:read", "credentials:cloud_key:write", "credentials:cloud_key:delete", "credentials:cloud_key:use",
	}
	got := map[string]Permission{}
	for _, p := range All {
		got[p.Code] = p
	}
	for _, code := range want {
		p, ok := got[code]
		if !ok {
			t.Errorf("missing permission code: %s", code)
			continue
		}
		if p.Category != "credentials" {
			t.Errorf("%s: Category=%q, want credentials", code, p.Category)
		}
		if p.Name == "" {
			t.Errorf("%s: Name (i18n key) empty", code)
		}
	}
}

func TestRegistry_AppsCodesPresent(t *testing.T) {
	want := []string{
		"apps:application:read", "apps:application:write", "apps:application:delete",
		"apps:release:install", "apps:release:upgrade",
		"apps:release:rollback", "apps:release:uninstall",
		"apps:repo:read", "apps:repo:write", "apps:repo:delete",
	}
	byCode := map[string]Permission{}
	for _, p := range All {
		byCode[p.Code] = p
	}
	for _, w := range want {
		got, ok := byCode[w]
		if !ok {
			t.Errorf("permission %q not registered", w)
			continue
		}
		if got.Category != "apps" {
			t.Errorf("permission %q has category %q, want %q", w, got.Category, "apps")
		}
		if got.Name == "" {
			t.Errorf("permission %q has empty Name (i18n key)", w)
		}
	}
}

func TestRegistry_K8sCodes(t *testing.T) {
	want := []string{
		"k8s:cluster:read",
		"k8s:cluster:write",
		"k8s:workload:read",
		"k8s:network:read",
		"k8s:config:read",
		"k8s:secret:read",
		"k8s:secret:reveal",
		"k8s:cluster_resource:read",
		"k8s:log:read",
	}
	got := map[string]bool{}
	for _, p := range All {
		if p.Category == "k8s" {
			got[p.Code] = true
		}
	}
	for _, c := range want {
		if !got[c] {
			t.Errorf("missing k8s code %s", c)
		}
	}
	if len(want) != len(got) {
		t.Errorf("unexpected extra k8s codes: want %d, got %d", len(want), len(got))
	}
}
