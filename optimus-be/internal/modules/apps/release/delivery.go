package release

import (
	"context"
	"errors"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	apprepo "optimus-be/internal/modules/apps/repo"
)

const (
	deliveryUpgradeKind            = "upgrade"
	deliveryUpgradeSucceededStatus = "deployed"
)

// VerifiedChartLoader is the Task 6 artifact-verification boundary required
// by delivery. UpgradeForDelivery fails closed when the ordinary P3 loader
// does not also implement this API.
type VerifiedChartLoader interface {
	LoadVerifiedChart(context.Context, apprepo.Artifact) (*chart.Chart, error)
}

// DeliveryOperationCoordinator is the Task 7 coordination boundary consumed
// by the constrained delivery upgrade.
type DeliveryOperationCoordinator interface {
	Acquire(context.Context, uint64, string, string, string, time.Duration) (AcquireResult, error)
	Complete(context.Context, string, string, SafeOperationResult) error
}

type deliveryRuntime struct {
	coordinator DeliveryOperationCoordinator
	owner       string
	lease       time.Duration
}

// SetDeliveryCoordinator configures delivery execution for this Service
// instance. Invalid or nil configuration disables delivery fail-closed while
// leaving the existing direct P3 lifecycle paths unchanged.
func (s *Service) SetDeliveryCoordinator(coordinator DeliveryOperationCoordinator, owner string, lease time.Duration) {
	if coordinator == nil || !validBounded(owner, 128) || lease <= 0 {
		s.delivery = nil
		return
	}
	s.delivery = &deliveryRuntime{coordinator: coordinator, owner: owner, lease: lease}
}

// UpgradeForDelivery upgrades one already-installed release to an immutable,
// verified chart artifact. It reuses the release's current Helm values and
// never accepts or returns values, notes, manifests, or raw executor errors.
func (s *Service) UpgradeForDelivery(ctx context.Context, req DeliveryUpgradeRequest) (*DeliveryUpgradeResult, error) {
	runtime := s.delivery
	verifiedLoader, verified := s.loader.(VerifiedChartLoader)
	if runtime == nil || !verified || !validDeliveryUpgradeRequest(req) {
		return nil, executionUnavailableError()
	}

	acquired, err := runtime.coordinator.Acquire(
		ctx, req.ApplicationID, req.OperationID, deliveryUpgradeKind, runtime.owner, runtime.lease,
	)
	if err != nil {
		return nil, err
	}
	if acquired.NeedsReconciliation {
		return nil, reconciliationRequiredError()
	}
	if acquired.Replayed && !acquired.Acquired {
		return replayDeliveryUpgrade(acquired.Operation)
	}
	if !acquired.Acquired {
		return nil, operationBusyError()
	}

	app, err := s.appsGet(ctx, req.ApplicationID)
	if err != nil {
		return nil, s.completeDefiniteDeliveryFailure(
			ctx, runtime, req.OperationID, deliveryApplicationUnavailableError(err),
		)
	}
	if app.ChartRepoID != req.RepoID || app.ChartName != req.ChartName {
		return nil, s.completeDefiniteDeliveryFailure(
			ctx, runtime, req.OperationID, deliveryChartIdentityError(),
		)
	}

	cfg, err := s.factory.NewForCluster(ctx, app.ClusterID, app.Namespace, req.Purpose)
	if err != nil {
		return nil, s.completeDefiniteDeliveryFailure(
			ctx, runtime, req.OperationID, deliveryExecutionUnavailableError(err),
		)
	}
	observed, err := action.NewStatus(cfg).Run(app.ReleaseName)
	if err != nil || observed == nil || observed.Info == nil || string(observed.Info.Status) == "uninstalled" {
		return nil, s.completeDefiniteDeliveryFailure(
			ctx, runtime, req.OperationID, deliveryApplicationUnavailableError(err),
		)
	}

	artifact := apprepo.Artifact{
		RepoID: req.RepoID, ChartName: req.ChartName, Version: req.ChartVersion, Digest: req.Digest,
	}
	ch, err := verifiedLoader.LoadVerifiedChart(ctx, artifact)
	if err != nil {
		if errors.Is(err, apprepo.ErrArtifactDigestMismatch) {
			return nil, s.completeDefiniteDeliveryFailure(
				ctx, runtime, req.OperationID, deliveryArtifactDriftError(err),
			)
		}
		return nil, s.completeDefiniteDeliveryFailure(
			ctx, runtime, req.OperationID, deliveryExecutionUnavailableError(err),
		)
	}
	if ch == nil || ch.Metadata == nil || ch.Metadata.Name != req.ChartName || ch.Metadata.Version != req.ChartVersion {
		return nil, s.completeDefiniteDeliveryFailure(
			ctx, runtime, req.OperationID, deliveryArtifactDriftError(nil),
		)
	}

	deliveryCtx := withDeliveryUpgrade(ctx, req.ApplicationID, req.OperationID)
	if err := s.authorizeMutation(deliveryCtx, req.ApplicationID, MutationActionUpgrade); err != nil {
		return nil, s.completeDefiniteDeliveryFailure(ctx, runtime, req.OperationID, err)
	}

	upgrade := action.NewUpgrade(cfg)
	upgrade.Namespace = app.Namespace
	upgrade.ReuseValues = true
	upgrade.Wait = false
	upgrade.Atomic = false
	if _, err := upgrade.RunWithContext(deliveryCtx, app.ReleaseName, ch, map[string]any{}); err != nil {
		return nil, deliveryReconciliationError(err)
	}

	observed, err = action.NewStatus(cfg).Run(app.ReleaseName)
	if err != nil || observed == nil || observed.Info == nil {
		return nil, deliveryReconciliationError(err)
	}
	result := &DeliveryUpgradeResult{
		Revision: observed.Version,
		Status:   string(observed.Info.Status),
		Digest:   req.Digest,
	}
	if err := runtime.coordinator.Complete(ctx, req.OperationID, runtime.owner, SafeOperationResult{
		Succeeded: true,
		Revision:  int64(result.Revision),
		Digest:    result.Digest,
	}); err != nil {
		return nil, err
	}

	s.writeAudit(ctx, req.InitiatorID, "", "", "apps.release.delivery_upgrade", app.ID, map[string]any{
		"operation_id":  req.OperationID,
		"repo_id":       req.RepoID,
		"chart_name":    req.ChartName,
		"chart_version": req.ChartVersion,
		"digest":        req.Digest,
		"revision":      result.Revision,
		"status":        result.Status,
	})
	return result, nil
}

func (s *Service) completeDefiniteDeliveryFailure(
	ctx context.Context,
	runtime *deliveryRuntime,
	operationID string,
	cause error,
) error {
	if err := runtime.coordinator.Complete(ctx, operationID, runtime.owner, SafeOperationResult{}); err != nil {
		return err
	}
	return cause
}

func replayDeliveryUpgrade(operation *Operation) (*DeliveryUpgradeResult, error) {
	if operation == nil {
		return nil, executionUnavailableError()
	}
	if operation.State != models.AppsReleaseOperationSucceeded || operation.ResultRevision == nil || operation.ResultDigest == nil {
		return nil, executionUnavailableError()
	}
	return &DeliveryUpgradeResult{
		Revision: int(*operation.ResultRevision),
		Status:   deliveryUpgradeSucceededStatus,
		Digest:   *operation.ResultDigest,
	}, nil
}

func validDeliveryUpgradeRequest(req DeliveryUpgradeRequest) bool {
	return req.ApplicationID != 0 && req.RepoID != 0 && req.InitiatorID != 0 &&
		validDeliveryOperationID(req.OperationID) &&
		validDeliveryText(req.ChartName, 255) && validDeliveryText(req.ChartVersion, 64) &&
		operationDigestRE.MatchString(req.Digest) && validDeliveryText(req.Purpose, 128)
}

func validDeliveryText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value
}

func deliveryApplicationUnavailableError(cause error) error {
	if cause == nil {
		return apperr.New(
			apperr.CodeDeliveryApplicationUnavailable,
			"delivery.application.unavailable",
			"delivery application is unavailable",
		)
	}
	return apperr.Wrap(
		cause,
		apperr.CodeDeliveryApplicationUnavailable,
		"delivery.application.unavailable",
		"delivery application is unavailable",
	)
}

func deliveryChartIdentityError() error {
	return apperr.New(
		apperr.CodeDeliveryChartIdentityMismatch,
		"delivery.chart.identity_mismatch",
		"delivery chart identity does not match",
	)
}

func deliveryArtifactDriftError(cause error) error {
	if cause == nil {
		return apperr.New(
			apperr.CodeDeliveryArtifactDrift,
			"delivery.execution.artifact_drift",
			"delivery artifact does not match",
		)
	}
	return apperr.Wrap(
		cause,
		apperr.CodeDeliveryArtifactDrift,
		"delivery.execution.artifact_drift",
		"delivery artifact does not match",
	)
}

func deliveryExecutionUnavailableError(cause error) error {
	if cause == nil {
		return executionUnavailableError()
	}
	return apperr.Wrap(
		cause,
		apperr.CodeDeliveryExecutionUnavailable,
		"delivery.execution.unavailable",
		"delivery execution is unavailable",
	)
}

func deliveryReconciliationError(cause error) error {
	if cause == nil {
		return reconciliationRequiredError()
	}
	return apperr.Wrap(
		cause,
		apperr.CodeDeliveryReconciliationRequired,
		"delivery.execution.reconciliation_required",
		"delivery execution requires reconciliation",
	)
}
