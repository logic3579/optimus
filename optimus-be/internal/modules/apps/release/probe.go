package release

import (
	"context"
	"log/slog"

	"helm.sh/helm/v3/pkg/action"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps"
)

// StatusForApplication satisfies application.HelmStatusProbe. Wired by
// main.go via application.Service.SetHelmStatusProbe(releaseSvc).
//
// Returns ("unknown", nil, "", "", "", nil) on any underlying error: the
// application detail page should always render even when the cluster is
// unreachable. The error is swallowed (logged at DEBUG) here because the
// caller in application.Service.Get treats non-nil errors as "leave the
// fields blank" — better to surface an explicit "unknown" status instead.
func (s *Service) StatusForApplication(ctx context.Context, app *models.AppsApplication) (string, *int, string, string, string, error) {
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.status")
	if err != nil {
		slog.Debug("release.StatusForApplication: factory failed",
			slog.Uint64("app_id", app.ID), slog.String("err", err.Error()))
		return "unknown", nil, "", "", "", nil
	}
	st := action.NewStatus(cfg)
	rel, err := st.Run(app.ReleaseName)
	if err != nil {
		// Translate so we log a structured biz code rather than the raw helm
		// text. The error is NOT propagated to the caller — application.Get
		// treats a non-nil err as "skip decoration" which would hide the row.
		mapped := apps.MapError(err)
		if be, ok := apperr.AsBiz(mapped); ok {
			slog.Debug("release.StatusForApplication: helm status failed",
				slog.Uint64("app_id", app.ID), slog.Int("code", int(be.Code)),
				slog.String("key", be.MessageKey))
		}
		return "unknown", nil, "", "", "", nil
	}
	rev := rel.Version
	return string(rel.Info.Status), &rev,
		chartVersion(rel),
		chartAppVersion(rel),
		rel.Info.LastDeployed.UTC().Format("2006-01-02T15:04:05Z"),
		nil
}

// IsReleaseInstalled satisfies application.HelmInstalledChecker. true means
// "delete must be refused"; false means "uninstalled (or never installed)".
//
// Trade-off: a network error during the check shouldn't block the delete
// forever. We treat unexpected helm errors as "not installed" (return false,
// nil) so delete can proceed — the worst case is a stale helm secret on the
// cluster, which the operator can clean up via `helm uninstall` directly.
// CodeAppsReleaseNotFound is the cleanly-expected "not installed" signal.
func (s *Service) IsReleaseInstalled(ctx context.Context, app *models.AppsApplication) (bool, error) {
	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, "apps.release.status")
	if err != nil {
		slog.Debug("release.IsReleaseInstalled: factory failed; allowing delete",
			slog.Uint64("app_id", app.ID), slog.String("err", err.Error()))
		return false, nil
	}
	st := action.NewStatus(cfg)
	rel, err := st.Run(app.ReleaseName)
	if err != nil {
		mapped := apps.MapError(err)
		if be, ok := apperr.AsBiz(mapped); ok && be.Code == apperr.CodeAppsReleaseNotFound {
			return false, nil
		}
		// Unexpected error — log but allow delete (see trade-off comment).
		slog.Debug("release.IsReleaseInstalled: helm status failed; allowing delete",
			slog.Uint64("app_id", app.ID), slog.String("err", err.Error()))
		return false, nil
	}
	// "uninstalled" status (history retained via keep_history=true) counts as
	// "not installed" — the helm secret exists but the workload is gone, so
	// the Optimus row can be safely removed.
	return string(rel.Info.Status) != "uninstalled", nil
}
