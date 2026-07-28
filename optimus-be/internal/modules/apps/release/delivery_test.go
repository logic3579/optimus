package release

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	apprepo "optimus-be/internal/modules/apps/repo"
)

const deliveryDigest = "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

type deliveryFactory struct {
	storage    *storage.Storage
	purposes   []string
	upgradeErr error
}

func newDeliveryFactory() *deliveryFactory {
	return &deliveryFactory{storage: storage.Init(driver.NewMemory())}
}

func (f *deliveryFactory) NewForCluster(_ context.Context, _ uint64, _, purpose string) (*action.Configuration, error) {
	f.purposes = append(f.purposes, purpose)
	kubeClient := &kubefake.FailingKubeClient{
		PrintingKubeClient: kubefake.PrintingKubeClient{Out: io.Discard},
		CreateError:        f.upgradeErr,
		UpdateError:        f.upgradeErr,
	}
	return &action.Configuration{
		Releases:     f.storage,
		KubeClient:   kubeClient,
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(_ string, _ ...interface{}) {},
	}, nil
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

	t.Run("terminal success replays without another mutation", func(t *testing.T) {
		service, apps, _, factory, _, _ := newDeliveryService(t)
		ctx := context.Background()
		_, err := service.Install(ctx, 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
		require.NoError(t, err)
		service.SetGovernance(&managedGovernance{operationID: "delivery-operation-1"})
		first, err := service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
		require.NoError(t, err)
		replay, err := service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
		require.NoError(t, err)
		require.Equal(t, first, replay)
		deployed, err := factory.storage.Last(apps.app.ReleaseName)
		require.NoError(t, err)
		require.Equal(t, 2, deployed.Version)
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

	t.Run("requires an installed release before acquiring", func(t *testing.T) {
		service, apps, _, _, _, coordinator := newDeliveryService(t)
		_, err := service.UpgradeForDelivery(context.Background(), deliveryRequest(apps.app.ID))
		requireBizError(t, err, apperr.CodeDeliveryApplicationUnavailable, "delivery.application.unavailable")
		_, inspectErr := coordinator.Inspect(context.Background(), "delivery-operation-1")
		require.Error(t, inspectErr)
	})

	t.Run("verified artifact mismatch fails before acquiring", func(t *testing.T) {
		service, apps, _, _, chartLoader, coordinator := newDeliveryService(t)
		ctx := context.Background()
		_, err := service.Install(ctx, 1, "", "", apps.app.ID, InstallRequest{ChartVersion: "1.0.0"})
		require.NoError(t, err)
		chartLoader.err = apprepo.ErrArtifactDigestMismatch
		_, err = service.UpgradeForDelivery(ctx, deliveryRequest(apps.app.ID))
		requireBizError(t, err, apperr.CodeDeliveryArtifactDrift, "delivery.execution.artifact_drift")
		_, inspectErr := coordinator.Inspect(ctx, "delivery-operation-1")
		require.Error(t, inspectErr)
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

	t.Run("fails closed without verified loader or coordinator", func(t *testing.T) {
		factory := newDeliveryFactory()
		apps := newStubAppService()
		service := NewService(factory, apps, fakeChartLoader{}, &inMemoryRecorder{})
		_, err := service.UpgradeForDelivery(context.Background(), deliveryRequest(apps.app.ID))
		requireBizError(t, err, apperr.CodeDeliveryExecutionUnavailable, "delivery.execution.unavailable")
		service.SetDeliveryCoordinator(nil, "worker-1", time.Minute)
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
