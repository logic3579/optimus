package apps_test

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	"helm.sh/helm/v3/pkg/storage/driver"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/apps"
)

// netErr is a synthetic net.Error used to exercise the network branch of
// MapError. The Temporary() method is deprecated in Go 1.18+ but is kept
// here for compatibility with older third-party error wrappers we might
// hit through transitive helm dependencies.
type netErr struct{ msg string }

func (e *netErr) Error() string   { return e.msg }
func (e *netErr) Timeout() bool   { return true }
func (e *netErr) Temporary() bool { return true }

func TestMapError_NilPassthrough(t *testing.T) {
	if got := apps.MapError(nil); got != nil {
		t.Fatalf("MapError(nil) = %v, want nil", got)
	}
}

func TestMapError_HelmSentinels(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want apperr.Code
	}{
		{"release not found", driver.ErrReleaseNotFound, apperr.CodeAppsReleaseNotFound},
		{"release exists", driver.ErrReleaseExists, apperr.CodeAppsReleaseAlreadyExists},
		{"no deployed releases", driver.ErrNoDeployedReleases, apperr.CodeAppsReleaseNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			be, ok := apperr.AsBiz(apps.MapError(c.in))
			if !ok {
				t.Fatalf("MapError(%v) did not produce BizError", c.in)
			}
			if be.Code != c.want {
				t.Errorf("got code %d, want %d", be.Code, c.want)
			}
			// Wrapped sentinel still surfaces via errors.Is.
			if !errors.Is(be, c.in) {
				t.Errorf("wrapped err loses cause; errors.Is fails")
			}
		})
	}
}

func TestMapError_HelmSentinelWrapped(t *testing.T) {
	// A driver sentinel wrapped one level deep must still classify correctly
	// via errors.Is — helm SDK wraps these in real call paths.
	wrapped := fmt.Errorf("install failed: %w", driver.ErrReleaseExists)
	be, ok := apperr.AsBiz(apps.MapError(wrapped))
	if !ok {
		t.Fatalf("not BizError")
	}
	if be.Code != apperr.CodeAppsReleaseAlreadyExists {
		t.Fatalf("got %d, want %d", be.Code, apperr.CodeAppsReleaseAlreadyExists)
	}
}

func TestMapError_NetworkErr(t *testing.T) {
	be, ok := apperr.AsBiz(apps.MapError(&netErr{msg: "i/o timeout"}))
	if !ok {
		t.Fatalf("not BizError")
	}
	if be.Code != apperr.CodeAppsRepoUnreachable {
		t.Errorf("got %d, want %d", be.Code, apperr.CodeAppsRepoUnreachable)
	}
}

func TestMapError_URLError(t *testing.T) {
	be, ok := apperr.AsBiz(apps.MapError(&url.Error{
		Op:  "Get",
		URL: "https://example.com/index.yaml",
		Err: errors.New("dial tcp: lookup example.com: no such host"),
	}))
	if !ok {
		t.Fatalf("not BizError")
	}
	if be.Code != apperr.CodeAppsRepoUnreachable {
		t.Errorf("got %d, want %d", be.Code, apperr.CodeAppsRepoUnreachable)
	}
}

func TestMapError_StringMatched(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want apperr.Code
	}{
		{"unauthorized", "registry returned 401 Unauthorized", apperr.CodeAppsRepoUnauthorized},
		{"denied", "access denied to repository", apperr.CodeAppsRepoUnauthorized},
		{"auth required", "authentication required for chart repo", apperr.CodeAppsRepoUnauthorized},
		{"chart not found", "chart not found: foo", apperr.CodeAppsRepoChartNotFound},
		{"manifest not found", "manifest not found: tag v1", apperr.CodeAppsRepoChartNotFound},
		{"tag not found", "tag not found in registry", apperr.CodeAppsRepoChartNotFound},
		{"bad index", "failed to parse yaml index file", apperr.CodeAppsRepoInvalidIndex},
		{"oci manifest error", "manifest digest mismatch", apperr.CodeAppsRepoOCIError},
		{"oci blob error", "blob upload incomplete", apperr.CodeAppsRepoOCIError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			be, ok := apperr.AsBiz(apps.MapError(errors.New(c.msg)))
			if !ok {
				t.Fatalf("not BizError")
			}
			if be.Code != c.want {
				t.Errorf("msg %q: got code %d, want %d", c.msg, be.Code, c.want)
			}
		})
	}
}

func TestMapError_Fallthrough(t *testing.T) {
	be, ok := apperr.AsBiz(apps.MapError(errors.New("some unrelated helm internal failure")))
	if !ok {
		t.Fatalf("not BizError")
	}
	if be.Code != apperr.CodeAppsReleaseOther {
		t.Errorf("got %d, want %d", be.Code, apperr.CodeAppsReleaseOther)
	}
}

func TestMapError_PreservesCause(t *testing.T) {
	// errors.Unwrap must yield the original error so callers can errors.Is
	// against driver sentinels even after MapError translates them.
	in := driver.ErrReleaseNotFound
	out := apps.MapError(in)
	if !errors.Is(out, in) {
		t.Fatalf("MapError lost cause chain")
	}
}
