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
