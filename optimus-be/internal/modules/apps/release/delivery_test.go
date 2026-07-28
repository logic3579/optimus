package release

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/kube"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	helmrelease "helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	apprepo "optimus-be/internal/modules/apps/repo"
)

const deliveryDigest = "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

type deliveryFactory struct {
	storage        *storage.Storage
	statusDriver   *deliveryStatusDriver
	purposes       []string
	factoryErr     error
	upgradeErr     error
	upgradeStarted chan struct{}
	upgradeGate    <-chan struct{}
	upgradeCalls   *atomic.Int32
}

func newDeliveryFactory() *deliveryFactory {
	statusDriver := &deliveryStatusDriver{Driver: driver.NewMemory()}
	return &deliveryFactory{
		storage:      storage.Init(statusDriver),
		statusDriver: statusDriver,
	}
}

func (f *deliveryFactory) NewForCluster(_ context.Context, _ uint64, _, purpose string) (*action.Configuration, error) {
	f.purposes = append(f.purposes, purpose)
	if f.factoryErr != nil {
		return nil, f.factoryErr
	}
	kubeClient := &kubefake.FailingKubeClient{
		PrintingKubeClient: kubefake.PrintingKubeClient{Out: io.Discard},
		CreateError:        f.upgradeErr,
		UpdateError:        f.upgradeErr,
	}
	var client kube.Interface = kubeClient
	if f.upgradeCalls != nil {
		client = &blockingDeliveryKubeClient{
			FailingKubeClient: *kubeClient,
			started:           f.upgradeStarted,
			gate:              f.upgradeGate,
			calls:             f.upgradeCalls,
		}
	}
	return &action.Configuration{
		Releases:     f.storage,
		KubeClient:   client,
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(_ string, _ ...interface{}) {},
	}, nil
}

type blockingDeliveryKubeClient struct {
	kubefake.FailingKubeClient
	started chan struct{}
	gate    <-chan struct{}
	calls   *atomic.Int32
}

func (c *blockingDeliveryKubeClient) Update(
	original, target kube.ResourceList,
	force bool,
) (*kube.Result, error) {
	call := c.calls.Add(1)
	if call == 1 {
		close(c.started)
		<-c.gate
	} else {
		return nil, errors.New("duplicate Helm upgrade invocation")
	}
	return c.FailingKubeClient.Update(original, target, force)
}

type deliveryStatusDriver struct {
	driver.Driver
	mu         sync.Mutex
	nextStatus helmrelease.Status
	armed      bool
}

func (d *deliveryStatusDriver) SetPostUpgradeStatus(status helmrelease.Status) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nextStatus = status
}

func (d *deliveryStatusDriver) Update(key string, release *helmrelease.Release) error {
	if err := d.Driver.Update(key, release); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.nextStatus != "" && release.Version > 1 && release.Info != nil && release.Info.Status == helmrelease.StatusDeployed {
		d.armed = true
	}
	return nil
}

func (d *deliveryStatusDriver) Query(labels map[string]string) ([]*helmrelease.Release, error) {
	releases, err := d.Driver.Query(labels)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.armed {
		return releases, nil
	}
	for _, release := range releases {
		if release.Version > 1 && release.Info != nil {
			release.Info.Status = d.nextStatus
		}
	}
	d.armed = false
	return releases, nil
}

type verifiedDeliveryLoader struct {
	artifacts []apprepo.Artifact
	err       error
}

func (l *verifiedDeliveryLoader) LoadChart(_ context.Context, _ uint64, name, version string) (*chart.Chart, error) {
	return loader.LoadArchive(buildMinimalChartTgz(name, version))
}

func (l *verifiedDeliveryLoader) LoadVerifiedChart(_ context.Context, artifact apprepo.Artifact) (*chart.Chart, error) {
	l.artifacts = append(l.artifacts, artifact)
	if l.err != nil {
		return nil, l.err
	}
	return loader.LoadArchive(buildMinimalChartTgz(artifact.ChartName, artifact.Version))
}

func newDeliveryService(t *testing.T) (*Service, *stubAppService, *inMemoryRecorder, *deliveryFactory, *verifiedDeliveryLoader, *Coordinator) {
	t.Helper()
	factory := newDeliveryFactory()
	apps := newStubAppService()
	recorder := &inMemoryRecorder{}
	chartLoader := &verifiedDeliveryLoader{}
	service := NewService(factory, apps, chartLoader, recorder)
	coordinator, _, _ := newCoordinatorHarness()
	service.SetDeliveryCoordinator(coordinator, "worker-1", time.Minute)
	return service, apps, recorder, factory, chartLoader, coordinator
}

func deliveryRequest(applicationID uint64) DeliveryUpgradeRequest {
	return DeliveryUpgradeRequest{
		ApplicationID: applicationID,
		OperationID:   "delivery-operation-1",
		RepoID:        7,
		ChartName:     "mychart",
		ChartVersion:  "2.0.0",
		Digest:        deliveryDigest,
		InitiatorID:   91,
		Purpose:       "delivery.run.11.stage.12",
	}
}

func TestDeliveryUpgrade(t *testing.T) {
	t.Run("exact request and result shapes", func(t *testing.T) {
		require.Equal(t, []string{
			"ApplicationID", "OperationID", "RepoID", "ChartName", "ChartVersion", "Digest", "InitiatorID", "Purpose",
		}, exportedFieldNames(reflect.TypeOf(DeliveryUpgradeRequest{})))
		require.Equal(t, []string{"Revision", "Status", "Digest"}, exportedFieldNames(reflect.TypeOf(DeliveryUpgradeResult{})))
	})

	t.Run("upgrades an installed verified artifact with reused values and a safe result", func(t *testing.T) {
		service, apps, recorder, factory, chartLoader, coordinator := newDeliveryService(t)
		ctx := context.Background()
		_, err := service.Install(ctx, 1, "", "", apps.app.ID, InstallRequest{
			ChartVersion: "1.0.0",
			ValuesYAML:   "preserved: environment-secret-reference\n",
		})
		require.NoError(t, err)
		policy := &managedGovernance{operationID: "delivery-operation-1"}
		service.SetGovernance(policy)

		result, err := service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
		require.NoError(t, err)
		require.Equal(t, &DeliveryUpgradeResult{Revision: 2, Status: "deployed", Digest: deliveryDigest}, result)
		require.Equal(t, []apprepo.Artifact{{
			RepoID: 7, ChartName: "mychart", Version: "2.0.0", Digest: deliveryDigest,
		}}, chartLoader.artifacts)
		require.Equal(t, "delivery.run.11.stage.12", factory.purposes[len(factory.purposes)-1])
		require.Equal(t, []MutationAction{MutationActionUpgrade}, policy.calls)

		deployed, err := factory.storage.Last(apps.app.ReleaseName)
		require.NoError(t, err)
		require.Equal(t, "environment-secret-reference", deployed.Config["preserved"], "ReuseValues must retain existing values")
		require.Empty(t, resultDigestForbiddenStrings(result))

		operation, err := coordinator.Inspect(ctx, "delivery-operation-1")
		require.NoError(t, err)
		require.Equal(t, models.AppsReleaseOperationSucceeded, operation.State)
		require.EqualValues(t, 2, derefInt64(operation.ResultRevision))
		require.Equal(t, deliveryDigest, derefString(operation.ResultDigest))

		events := recorder.snapshot()
		require.Len(t, events, 2)
		auditEvent := events[1]
		require.Equal(t, "apps.release.delivery_upgrade", auditEvent.Action)
		require.NotNil(t, auditEvent.UserID)
		require.Equal(t, uint64(91), *auditEvent.UserID)
		payload, ok := auditEvent.Payload.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "delivery-operation-1", payload["operation_id"])
		require.Equal(t, deliveryDigest, payload["digest"])
		for key, value := range payload {
			text := strings.ToLower(key + " " + stringify(value))
			require.NotContains(t, text, "values")
			require.NotContains(t, text, "notes")
			require.NotContains(t, text, "environment-secret-reference")
			require.NotContains(t, text, "raw_error")
		}
	})

	t.Run("terminal success replays persisted result without external calls", func(t *testing.T) {
		service, apps, _, factory, chartLoader, _ := newDeliveryService(t)
		ctx := context.Background()
		_, err := service.Install(ctx, 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
		require.NoError(t, err)
		service.SetGovernance(&managedGovernance{operationID: "delivery-operation-1"})
		first, err := service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
		require.NoError(t, err)
		factoryCalls := len(factory.purposes)
		artifactCalls := len(chartLoader.artifacts)
		factory.factoryErr = errors.New("cluster must not be contacted during replay")
		chartLoader.err = errors.New("repository must not be contacted during replay")
		mismatchRequest := deliveryRequest(apps.app.ID)
		mismatchRequest.Digest = validOperationDigest
		_, err = service.UpgradeForDelivery(ctx, mismatchRequest)
		requireBizError(t, err, apperr.CodeDeliveryArtifactDrift, "delivery.execution.artifact_drift")
		require.Len(t, factory.purposes, factoryCalls)
		require.Len(t, chartLoader.artifacts, artifactCalls)

		replayRequest := deliveryRequest(apps.app.ID)
		replayRequest.RepoID = 99
		replayRequest.ChartName = "changed-chart"
		replayRequest.ChartVersion = "9.9.9"
		replay, err := service.UpgradeForDelivery(ctx, replayRequest)
		require.NoError(t, err)
		require.Equal(t, first, replay)
		require.Len(t, factory.purposes, factoryCalls)
		require.Len(t, chartLoader.artifacts, artifactCalls)
		deployed, err := factory.storage.Last(apps.app.ReleaseName)
		require.NoError(t, err)
		require.Equal(t, 2, deployed.Version)
	})

	t.Run("concurrent same operation permits one Helm invocation", func(t *testing.T) {
		service, apps, _, factory, _, _ := newDeliveryService(t)
		ctx := context.Background()
		_, err := service.Install(ctx, 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
		require.NoError(t, err)
		service.SetGovernance(&managedGovernance{operationID: "delivery-operation-1"})
		started := make(chan struct{})
		gate := make(chan struct{})
		calls := &atomic.Int32{}
		factory.upgradeStarted = started
		factory.upgradeGate = gate
		factory.upgradeCalls = calls

		firstResult := make(chan error, 1)
		go func() {
			_, upgradeErr := service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
			firstResult <- upgradeErr
		}()
		<-started
		_, secondErr := service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
		close(gate)
		firstErr := <-firstResult

		require.NoError(t, firstErr)
		requireBizError(t, secondErr, apperr.CodeDeliveryOperationBusy, "delivery.execution.operation_busy")
		require.EqualValues(t, 1, calls.Load())
	})

	t.Run("terminal failed replay returns safe failure without external calls", func(t *testing.T) {
		service, apps, _, factory, chartLoader, coordinator := newDeliveryService(t)
		ctx := context.Background()
		_, err := coordinator.Acquire(ctx, apps.app.ID, "delivery-operation-1", "upgrade", "worker-1", time.Minute)
		require.NoError(t, err)
		require.NoError(t, coordinator.Complete(ctx, "delivery-operation-1", "worker-1", SafeOperationResult{}))
		factory.factoryErr = errors.New("cluster must not be contacted during failed replay")
		chartLoader.err = errors.New("repository must not be contacted during failed replay")

		_, err = service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
		requireBizError(t, err, apperr.CodeDeliveryExecutionUnavailable, "delivery.execution.unavailable")
		require.Empty(t, factory.purposes)
		require.Empty(t, chartLoader.artifacts)
	})

	t.Run("busy and reconciliation acquisitions never mutate", func(t *testing.T) {
		service, apps, _, factory, _, coordinator := newDeliveryService(t)
		ctx := context.Background()
		_, err := service.Install(ctx, 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
		require.NoError(t, err)
		service.SetGovernance(&managedGovernance{operationID: "delivery-operation-2"})
		_, err = coordinator.Acquire(ctx, apps.app.ID, "blocking-operation", "upgrade", "other-worker", time.Minute)
		require.NoError(t, err)

		request := deliveryRequest(apps.app.ID)
		request.OperationID = "delivery-operation-2"
		_, err = service.UpgradeForDelivery(ctx, request)
		requireBizError(t, err, apperr.CodeDeliveryOperationBusy, "delivery.execution.operation_busy")
		deployed, loadErr := factory.storage.Last(apps.app.ReleaseName)
		require.NoError(t, loadErr)
		require.Equal(t, 1, deployed.Version)

		coordinator2, clock, _ := newCoordinatorHarness()
		service.SetDeliveryCoordinator(coordinator2, "worker-1", time.Minute)
		_, err = coordinator2.Acquire(ctx, apps.app.ID, "expired-operation", "upgrade", "old-worker", time.Minute)
		require.NoError(t, err)
		clock.Advance(2 * time.Minute)
		_, err = service.UpgradeForDelivery(ctx, request)
		requireBizError(t, err, apperr.CodeDeliveryReconciliationRequired, "delivery.execution.reconciliation_required")
		deployed, loadErr = factory.storage.Last(apps.app.ReleaseName)
		require.NoError(t, loadErr)
		require.Equal(t, 1, deployed.Version)
	})

	t.Run("missing release is a definite failed operation", func(t *testing.T) {
		service, apps, _, _, _, coordinator := newDeliveryService(t)
		_, err := service.UpgradeForDelivery(context.Background(), deliveryRequest(apps.app.ID))
		requireBizError(t, err, apperr.CodeDeliveryApplicationUnavailable, "delivery.application.unavailable")
		operation, inspectErr := coordinator.Inspect(context.Background(), "delivery-operation-1")
		require.NoError(t, inspectErr)
		require.Equal(t, models.AppsReleaseOperationFailed, operation.State)
		require.NotNil(t, operation.FinishedAt)
	})

	t.Run("verified artifact mismatch is a definite failed operation", func(t *testing.T) {
		service, apps, _, _, chartLoader, coordinator := newDeliveryService(t)
		ctx := context.Background()
		_, err := service.Install(ctx, 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
		require.NoError(t, err)
		chartLoader.err = apprepo.ErrArtifactDigestMismatch
		_, err = service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
		requireBizError(t, err, apperr.CodeDeliveryArtifactDrift, "delivery.execution.artifact_drift")
		operation, inspectErr := coordinator.Inspect(ctx, "delivery-operation-1")
		require.NoError(t, inspectErr)
		require.Equal(t, models.AppsReleaseOperationFailed, operation.State)
		require.NotNil(t, operation.FinishedAt)
	})

	t.Run("uninstalled release completes failed without artifact or upgrade", func(t *testing.T) {
		service, apps, _, factory, chartLoader, coordinator := newDeliveryService(t)
		ctx := context.Background()
		_, err := service.Install(ctx, 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
		require.NoError(t, err)
		require.NoError(t, service.Uninstall(ctx, 1, "", "", apps.app.ID, UninstallRequest{KeepHistory: true}))
		factoryCalls := len(factory.purposes)

		_, err = service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
		requireBizError(t, err, apperr.CodeDeliveryApplicationUnavailable, "delivery.application.unavailable")
		require.Len(t, factory.purposes, factoryCalls+1, "only installed-status inspection may build Helm config")
		require.Empty(t, chartLoader.artifacts)
		operation, inspectErr := coordinator.Inspect(ctx, "delivery-operation-1")
		require.NoError(t, inspectErr)
		require.Equal(t, models.AppsReleaseOperationFailed, operation.State)
		require.NotNil(t, operation.FinishedAt)
		uninstalled, statusErr := factory.storage.Last(apps.app.ReleaseName)
		require.NoError(t, statusErr)
		require.Equal(t, "uninstalled", string(uninstalled.Info.Status))
	})

	t.Run("ambiguous Helm failure stays active for reconciliation", func(t *testing.T) {
		service, apps, _, factory, _, coordinator := newDeliveryService(t)
		ctx := context.Background()
		_, err := service.Install(ctx, 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
		require.NoError(t, err)
		service.SetGovernance(&managedGovernance{operationID: "delivery-operation-1"})
		factory.upgradeErr = errors.New("raw Kubernetes Secret data must not be persisted")
		_, err = service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
		requireBizError(t, err, apperr.CodeDeliveryReconciliationRequired, "delivery.execution.reconciliation_required")
		operation, inspectErr := coordinator.Inspect(ctx, "delivery-operation-1")
		require.NoError(t, inspectErr)
		require.Equal(t, models.AppsReleaseOperationActive, operation.State)
		require.Nil(t, operation.FinishedAt)
		deployed, loadErr := factory.storage.Last(apps.app.ReleaseName)
		require.NoError(t, loadErr)
		require.LessOrEqual(t, deployed.Version, 2)
	})

	for _, status := range []helmrelease.Status{
		helmrelease.StatusFailed,
		helmrelease.StatusPendingInstall,
		helmrelease.StatusPendingUpgrade,
		helmrelease.StatusPendingRollback,
		helmrelease.StatusUninstalling,
		helmrelease.StatusUninstalled,
		helmrelease.StatusUnknown,
	} {
		status := status
		t.Run("post-upgrade "+string(status)+" remains reconcilable", func(t *testing.T) {
			service, apps, _, factory, _, coordinator := newDeliveryService(t)
			ctx := context.Background()
			_, err := service.Install(ctx, 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
			require.NoError(t, err)
			service.SetGovernance(&managedGovernance{operationID: "delivery-operation-1"})
			factory.statusDriver.SetPostUpgradeStatus(status)

			_, err = service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
			requireBizError(t, err, apperr.CodeDeliveryReconciliationRequired, "delivery.execution.reconciliation_required")
			operation, inspectErr := coordinator.Inspect(ctx, "delivery-operation-1")
			require.NoError(t, inspectErr)
			require.Equal(t, models.AppsReleaseOperationActive, operation.State)
			require.Nil(t, operation.FinishedAt)
		})
	}

	t.Run("fails closed without verified loader or coordinator", func(t *testing.T) {
		factory := newDeliveryFactory()
		apps := newStubAppService()
		service := NewService(factory, apps, fakeChartLoader{}, &inMemoryRecorder{})
		_, err := service.UpgradeForDelivery(context.Background(), deliveryRequest(apps.app.ID))
		requireBizError(t, err, apperr.CodeDeliveryExecutionUnavailable, "delivery.execution.unavailable")
		service.SetDeliveryCoordinator(nil, "worker-1", time.Minute)
	})

	t.Run("delivery configuration publishes safely during reads", func(t *testing.T) {
		service, _, _, _, _, coordinator := newDeliveryService(t)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				service.SetDeliveryCoordinator(coordinator, "worker", time.Minute)
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				_, _ = service.UpgradeForDelivery(context.Background(), DeliveryUpgradeRequest{})
			}
		}()
		wg.Wait()
	})
}

func exportedFieldNames(typ reflect.Type) []string {
	names := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).IsExported() {
			names = append(names, typ.Field(i).Name)
		}
	}
	return names
}

func stringify(value any) string {
	if value == nil {
		return ""
	}
	return reflect.ValueOf(value).String()
}

func resultDigestForbiddenStrings(result *DeliveryUpgradeResult) []string {
	if result == nil {
		return []string{"nil"}
	}
	var found []string
	for _, value := range []string{result.Status, result.Digest} {
		lower := strings.ToLower(value)
		for _, forbidden := range []string{"values", "notes", "secret", "error"} {
			if strings.Contains(lower, forbidden) {
				found = append(found, forbidden)
			}
		}
	}
	return found
}
