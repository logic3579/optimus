//go:build dbtest

package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps/release"
	"optimus-be/tests/dbtest"
)

func TestAppsReleaseOperationConcurrency(t *testing.T) {
	_, database := setupServer(t)
	cluster := dbtest.SeedCluster(t, database, "release-operation-race")
	chartRepo := &models.AppsChartRepo{
		Name: "release-operation-race-repo",
		Type: "http",
		URL:  "https://charts.example.test",
	}
	require.NoError(t, database.Create(chartRepo).Error)
	application := &models.AppsApplication{
		Name:        "release-operation-race-app",
		ClusterID:   cluster.ID,
		Namespace:   "default",
		ReleaseName: "release-operation-race",
		ChartRepoID: chartRepo.ID,
		ChartName:   "example",
	}
	require.NoError(t, database.Create(application).Error)

	first := release.NewCoordinator(database.Session(&gorm.Session{NewDB: true}))
	second := release.NewCoordinator(database.Session(&gorm.Session{NewDB: true}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type outcome struct {
		result release.AcquireResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var workers sync.WaitGroup
	for _, attempt := range []struct {
		coordinator *release.Coordinator
		operationID string
		owner       string
	}{
		{coordinator: first, operationID: "release-operation-race-1", owner: "worker-1"},
		{coordinator: second, operationID: "release-operation-race-2", owner: "worker-2"},
	} {
		attempt := attempt
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, err := attempt.coordinator.Acquire(
				ctx,
				application.ID,
				attempt.operationID,
				"upgrade",
				attempt.owner,
				time.Minute,
			)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(outcomes)

	acquired := 0
	busy := 0
	for got := range outcomes {
		if got.err == nil {
			require.True(t, got.result.Acquired)
			acquired++
			continue
		}
		business, ok := apperr.AsBiz(got.err)
		require.True(t, ok)
		require.Equal(t, apperr.CodeDeliveryOperationBusy, business.Code)
		busy++
	}
	require.Equal(t, 1, acquired)
	require.Equal(t, 1, busy)

	var active int64
	require.NoError(t, database.Model(&models.AppsReleaseOperation{}).
		Where("application_id = ? AND state IN ?", application.ID, []models.AppsReleaseOperationState{
			models.AppsReleaseOperationActive,
			models.AppsReleaseOperationReconciling,
		}).
		Count(&active).Error)
	require.EqualValues(t, 1, active)
}

func TestAppsReleaseOperationReconciliationLeaseTakeover(t *testing.T) {
	_, database := setupServer(t)
	cluster := dbtest.SeedCluster(t, database, "release-operation-reconciliation")
	chartRepo := &models.AppsChartRepo{
		Name: "release-operation-reconciliation-repo",
		Type: "http",
		URL:  "https://charts.example.test",
	}
	require.NoError(t, database.Create(chartRepo).Error)
	application := &models.AppsApplication{
		Name:        "release-operation-reconciliation-app",
		ClusterID:   cluster.ID,
		Namespace:   "default",
		ReleaseName: "release-operation-reconciliation",
		ChartRepoID: chartRepo.ID,
		ChartName:   "example",
	}
	require.NoError(t, database.Create(application).Error)

	expired := time.Now().Add(-time.Minute)
	owner := "expired-mutation-owner"
	operation := &models.AppsReleaseOperation{
		ApplicationID:  application.ID,
		OperationID:    "release-operation-reconciliation-1",
		Kind:           "upgrade",
		State:          models.AppsReleaseOperationActive,
		LeaseOwner:     &owner,
		LeaseExpiresAt: &expired,
	}
	require.NoError(t, database.Create(operation).Error)
	coordinator := release.NewCoordinator(database)

	first, err := coordinator.Acquire(
		context.Background(), application.ID, "next-operation-1", "upgrade", "reconciler-1", time.Minute,
	)
	require.NoError(t, err)
	require.False(t, first.Acquired)
	require.True(t, first.NeedsReconciliation)
	require.Equal(t, "reconciler-1", *first.Operation.LeaseOwner)
	require.NotNil(t, first.Operation.LeaseExpiresAt)

	require.NoError(t, database.Model(&models.AppsReleaseOperation{}).
		Where("id = ?", operation.ID).
		Update("lease_expires_at", expired).Error)
	second, err := coordinator.Acquire(
		context.Background(), application.ID, "next-operation-2", "upgrade", "reconciler-2", time.Minute,
	)
	require.NoError(t, err)
	require.False(t, second.Acquired)
	require.True(t, second.NeedsReconciliation)
	require.Equal(t, "reconciler-2", *second.Operation.LeaseOwner)
	require.NotNil(t, second.Operation.LeaseExpiresAt)
}
