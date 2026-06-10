// Package k8s hosts the shared error mapper used by every read endpoint in
// the k8s sub-packages (clusterscoped, workload, network, config, secret,
// yaml, log). Keeping it at this level — rather than inside `client` —
// avoids forcing every leaf package to import the heavy client-go transitive
// chain just to map an error.
package k8s

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	apperr "optimus-be/internal/infra/errors"
)

// MapAPIError converts apiserver / network errors into Optimus BizErrors.
// Returns nil for nil input.
//
// The 41xxx ladder (CodeClusterUnreachable / Forbidden / Unauthorized /
// Other) is reserved for runtime apiserver failures; the existing 40401
// CodeNotFound is reused for resource-missing because clients treat it like
// any other not-found and surface a generic empty-state.
//
// CodeClusterUnreachable subsumes both apiserver-side timeouts
// (apierrors.IsTimeout / IsServerTimeout) and client-side networking errors
// (dial refused, DNS lookup failure, TLS handshake) — from the user's PoV
// they all mean "the cluster isn't reachable."
func MapAPIError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case apierrors.IsNotFound(err):
		return apperr.New(apperr.CodeNotFound, "k8s.apiserver.not_found", err.Error())
	case apierrors.IsForbidden(err):
		return apperr.New(apperr.CodeAPIServerForbidden, "k8s.apiserver.forbidden", err.Error())
	case apierrors.IsUnauthorized(err):
		return apperr.New(apperr.CodeAPIServerUnauthorized, "k8s.apiserver.unauthorized", err.Error())
	case apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return apperr.New(apperr.CodeClusterUnreachable, "k8s.cluster.unreachable", err.Error())
	case isNetworkErr(err):
		return apperr.New(apperr.CodeClusterUnreachable, "k8s.cluster.unreachable", err.Error())
	default:
		return apperr.New(apperr.CodeAPIServerOther, "k8s.apiserver.other", err.Error())
	}
}

// isNetworkErr recognises the four standard-library transport error types
// and three substring fallbacks that show up in client-go wrapped errors
// (which often hide the typed root cause behind a fmt.Errorf chain).
func isNetworkErr(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return true
	}
	var unkAuth x509.UnknownAuthorityError
	if errors.As(err, &unkAuth) {
		return true
	}
	var rec tls.RecordHeaderError
	if errors.As(err, &rec) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "connection refused") ||
		strings.Contains(s, "no such host") ||
		strings.Contains(s, "i/o timeout")
}
