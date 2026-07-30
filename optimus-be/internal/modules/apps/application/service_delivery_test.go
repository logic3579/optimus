package application

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
)

type deletionRepositoryFake struct {
	events  []string
	row     *models.AppsApplication
	deleted bool
}

func (r *deletionRepositoryFake) transaction(_ context.Context, fn func(applicationDeletionRepository) error) error {
	r.events = append(r.events, "transaction.begin")
	err := fn(r)
	r.events = append(r.events, "transaction.end")
	return err
}

func (r *deletionRepositoryFake) lockApplication(context.Context, uint64) error {
	r.events = append(r.events, "application.lock")
	return nil
}

func (r *deletionRepositoryFake) Get(_ context.Context, id uint64) (*models.AppsApplication, error) {
	r.events = append(r.events, "application.get")
	if r.deleted || r.row == nil || r.row.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	copied := *r.row
	return &copied, nil
}

func (r *deletionRepositoryFake) Delete(_ context.Context, id uint64) error {
	r.events = append(r.events, "application.delete")
	if r.deleted || r.row == nil || r.row.ID != id {
		return gorm.ErrRecordNotFound
	}
	r.deleted = true
	return nil
}

type deliveryCounterFake struct {
	events *[]string
	count  int64
	err    error
}

type installedCheckerFake struct {
	events    *[]string
	installed bool
	err       error
}

func (c installedCheckerFake) IsReleaseInstalled(context.Context, *models.AppsApplication) (bool, error) {
	*c.events = append(*c.events, "helm.probe")
	return c.installed, c.err
}

func (c deliveryCounterFake) CountByApplicationID(context.Context, uint64) (int64, error) {
	*c.events = append(*c.events, "delivery.count")
	return c.count, c.err
}

func TestDeleteApplicationDeliveryUseCheckRunsUnderLifecycleLock(t *testing.T) {
	tests := []struct {
		name       string
		counter    func(*[]string) DeliveryApplicationCounter
		wantEvents []string
		wantCode   apperr.Code
		wantDelete bool
	}{
		{
			name: "nil counter preserves unbound behavior",
			counter: func(*[]string) DeliveryApplicationCounter {
				return nil
			},
			wantEvents: []string{"transaction.begin", "application.lock", "application.get", "application.delete", "transaction.end"},
			wantDelete: true,
		},
		{
			name: "zero bindings permits deletion",
			counter: func(events *[]string) DeliveryApplicationCounter {
				return deliveryCounterFake{events: events}
			},
			wantEvents: []string{"transaction.begin", "application.lock", "application.get", "delivery.count", "application.delete", "transaction.end"},
			wantDelete: true,
		},
		{
			name: "active binding blocks deletion",
			counter: func(events *[]string) DeliveryApplicationCounter {
				return deliveryCounterFake{events: events, count: 1}
			},
			wantEvents: []string{"transaction.begin", "application.lock", "application.get", "delivery.count", "transaction.end"},
			wantCode:   apperr.CodeDeliveryEnvironmentInUse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &deletionRepositoryFake{row: &models.AppsApplication{ID: 42, Name: "app"}}
			counter := tt.counter(&repo.events)
			_, err := deleteApplication(context.Background(), repo, counter, nil, 42)
			if tt.wantCode == 0 {
				require.NoError(t, err)
			} else {
				be, ok := apperr.AsBiz(err)
				require.True(t, ok)
				require.Equal(t, tt.wantCode, be.Code)
			}
			require.Equal(t, tt.wantEvents, repo.events)
			require.Equal(t, tt.wantDelete, repo.deleted)
		})
	}
}

func TestDeleteApplicationDeliveryUseLookupWrapsCauseSafely(t *testing.T) {
	repo := &deletionRepositoryFake{row: &models.AppsApplication{ID: 42, Name: "app"}}
	wantErr := errors.New("postgres exposed sensitive topology")
	counter := deliveryCounterFake{events: &repo.events, err: wantErr}

	_, err := deleteApplication(context.Background(), repo, counter, nil, 42)
	be, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeDeliveryApplicationUnavailable, be.Code)
	require.Equal(t, "delivery.application.unavailable", be.MessageKey)
	require.NotContains(t, be.Message, "sensitive topology")
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, []string{
		"transaction.begin", "application.lock", "application.get", "delivery.count", "transaction.end",
	}, repo.events)
	require.False(t, repo.deleted)
}

func TestDeleteApplicationHelmProbePrecedesLifecycleTransaction(t *testing.T) {
	t.Run("safe probe then short locked transaction", func(t *testing.T) {
		repo := &deletionRepositoryFake{row: &models.AppsApplication{ID: 42, Name: "app"}}
		checker := installedCheckerFake{events: &repo.events}
		counter := deliveryCounterFake{events: &repo.events}

		_, err := deleteApplication(context.Background(), repo, counter, checker, 42)
		require.NoError(t, err)
		require.Equal(t, []string{
			"application.get", "helm.probe",
			"transaction.begin", "application.lock", "application.get", "delivery.count", "application.delete", "transaction.end",
		}, repo.events)
		require.True(t, repo.deleted)
	})

	t.Run("installed release avoids transaction", func(t *testing.T) {
		repo := &deletionRepositoryFake{row: &models.AppsApplication{ID: 42, Name: "app"}}
		checker := installedCheckerFake{events: &repo.events, installed: true}

		_, err := deleteApplication(context.Background(), repo, nil, checker, 42)
		be, ok := apperr.AsBiz(err)
		require.True(t, ok)
		require.Equal(t, apperr.CodeAppsReleaseStillPresent, be.Code)
		require.Equal(t, []string{"application.get", "helm.probe"}, repo.events)
		require.False(t, repo.deleted)
	})

	t.Run("probe error avoids transaction", func(t *testing.T) {
		repo := &deletionRepositoryFake{row: &models.AppsApplication{ID: 42, Name: "app"}}
		wantErr := errors.New("helm unavailable")
		checker := installedCheckerFake{events: &repo.events, err: wantErr}

		_, err := deleteApplication(context.Background(), repo, nil, checker, 42)
		require.ErrorIs(t, err, wantErr)
		require.Equal(t, []string{"application.get", "helm.probe"}, repo.events)
		require.False(t, repo.deleted)
	})
}
