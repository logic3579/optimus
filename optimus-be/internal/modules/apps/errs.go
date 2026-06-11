// Package apps provides MapError, the central error-mapping function used by
// the helmclient/, repo/, application/, and release/ sub-packages to normalise
// helm SDK / OCI registry / chart repository / network errors into BizError
// values in the 42xxx range. See P3 spec §5.
package apps

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"helm.sh/helm/v3/pkg/storage/driver"

	apperr "optimus-be/internal/infra/errors"
)

// MapError normalises an upstream error from helm SDK / OCI registry / chart
// repository transport / network into an apperr.BizError in the 42xxx range.
// nil pass-through. Callers MUST funnel every upstream error through this
// function before returning to the handler layer — handlers never see raw
// helm/registry/HTTP text.
//
// Classification order (first match wins):
//  1. helm storage driver sentinel errors (release not found / exists / no deployed).
//  2. Network errors (net.Error) and URL parse errors (*url.Error) → repo unreachable.
//  3. String-matched registry / repo categories (auth, chart-not-found, bad index, OCI).
//  4. Fallthrough → CodeAppsReleaseOther.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	// 1) helm storage driver sentinels.
	switch {
	case errors.Is(err, driver.ErrReleaseNotFound):
		return apperr.Wrap(err, apperr.CodeAppsReleaseNotFound, "apps.release.not_found", err.Error())
	case errors.Is(err, driver.ErrReleaseExists):
		return apperr.Wrap(err, apperr.CodeAppsReleaseAlreadyExists, "apps.release.already_exists", err.Error())
	case errors.Is(err, driver.ErrNoDeployedReleases):
		return apperr.Wrap(err, apperr.CodeAppsReleaseNotFound, "apps.release.no_deployed", err.Error())
	}

	// 2) network / URL errors → repo unreachable.
	var ne net.Error
	if errors.As(err, &ne) {
		return apperr.Wrap(err, apperr.CodeAppsRepoUnreachable, "apps.repo.unreachable", err.Error())
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return apperr.Wrap(err, apperr.CodeAppsRepoUnreachable, "apps.repo.unreachable", err.Error())
	}

	// 3) registry / repo string matching. Lower-cased matching against the
	// upstream error text. Order matters: more specific categories first.
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unauthorized"),
		strings.Contains(msg, "denied"),
		strings.Contains(msg, "authentication required"):
		return apperr.Wrap(err, apperr.CodeAppsRepoUnauthorized, "apps.repo.unauthorized", err.Error())
	case strings.Contains(msg, "not found") &&
		(strings.Contains(msg, "chart") ||
			strings.Contains(msg, "manifest") ||
			strings.Contains(msg, "tag")):
		return apperr.Wrap(err, apperr.CodeAppsRepoChartNotFound, "apps.repo.chart_not_found", err.Error())
	case strings.Contains(msg, "yaml") && strings.Contains(msg, "index"):
		return apperr.Wrap(err, apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_index", err.Error())
	case strings.Contains(msg, "manifest"), strings.Contains(msg, "blob"):
		return apperr.Wrap(err, apperr.CodeAppsRepoOCIError, "apps.repo.oci_error", err.Error())
	}

	// 4) fallthrough.
	return apperr.Wrap(err, apperr.CodeAppsReleaseOther, "apps.release.other", err.Error())
}
