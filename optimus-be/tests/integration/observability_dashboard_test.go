//go:build dbtest

package integration_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/observability/dashboard"
	"optimus-be/internal/modules/observability/datasource"
)

func TestObservabilityDashboardAggregateAndDatasourceReferences(t *testing.T) {
	_, db := setupServer(t)
	ctx := t.Context()
	recorder := audit.NewRecorder(db)
	dashboardRepo := dashboard.NewRepo(db)
	dashboardService := dashboard.NewService(dashboardRepo, recorder)
	datasourceService := datasource.NewService(
		datasource.NewRepo(db),
		nil,
		nil,
		dashboardRepo,
		nil,
		nil,
		recorder,
	)

	primaryDatasource, err := datasourceService.Create(ctx, 0, "", "", datasource.CreateRequest{
		Name:     "prometheus-primary",
		BaseURL:  "https://prometheus-primary.example.test",
		AuthType: "none",
	})
	require.NoError(t, err)
	secondaryDatasource, err := datasourceService.Create(ctx, 0, "", "", datasource.CreateRequest{
		Name:     "prometheus-secondary",
		BaseURL:  "https://prometheus-secondary.example.test",
		AuthType: "none",
	})
	require.NoError(t, err)

	created, err := dashboardService.Create(ctx, 0, "", "", dashboard.SaveRequest{
		Name:             "Platform Overview",
		Description:      "initial dashboard",
		RefreshIntervalS: 30,
		TimeRange:        "1h",
		Panels: []dashboard.PanelInput{
			{
				DatasourceID: primaryDatasource.ID,
				Title:        "CPU",
				PanelType:    "time_series",
				PromQL:       "rate(process_cpu_seconds_total[5m])",
				Unit:         "cores",
				SortOrder:    0,
				Width:        12,
			},
			{
				DatasourceID: secondaryDatasource.ID,
				Title:        "Memory",
				PanelType:    "stat",
				PromQL:       "process_resident_memory_bytes",
				Unit:         "bytes",
				SortOrder:    1,
				Width:        6,
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, created.Panels, 2)

	_, err = dashboardService.Create(ctx, 0, "", "", dashboard.SaveRequest{
		Name:             created.Name,
		RefreshIntervalS: 30,
		TimeRange:        "1h",
		Panels:           []dashboard.PanelInput{},
	})
	requireDashboardBizCode(t, err, apperr.CodeObservabilityDashboardNameTaken)

	matching, err := dashboardService.List(ctx, dashboard.ListQuery{Page: 1, PageSize: 20, Q: "platform"})
	require.NoError(t, err)
	require.EqualValues(t, 1, matching.Total)
	require.Len(t, matching.Items, 1)
	require.Equal(t, created.ID, matching.Items[0].ID)

	notMatching, err := dashboardService.List(ctx, dashboard.ListQuery{Page: 1, PageSize: 20, Q: "missing"})
	require.NoError(t, err)
	require.Zero(t, notMatching.Total)
	require.Empty(t, notMatching.Items)

	originalPanelIDs := []uint64{created.Panels[0].ID, created.Panels[1].ID}
	replacement := dashboard.SaveRequest{
		Name:             "Platform Overview",
		Description:      "replacement dashboard",
		RefreshIntervalS: 60,
		TimeRange:        "6h",
		Panels: []dashboard.PanelInput{{
			DatasourceID: primaryDatasource.ID,
			Title:        "Requests",
			PanelType:    "table",
			PromQL:       "sum(rate(http_requests_total[5m])) by (status)",
			Unit:         "requests_per_second",
			Legend:       "{{status}}",
			SortOrder:    0,
			Width:        6,
		}},
	}
	updated, err := dashboardService.Update(ctx, 0, "", "", created.ID, replacement)
	require.NoError(t, err)
	require.Equal(t, replacement.Description, updated.Description)
	require.Equal(t, replacement.RefreshIntervalS, updated.RefreshIntervalS)
	require.Equal(t, replacement.TimeRange, updated.TimeRange)
	require.Len(t, updated.Panels, 1)
	require.Equal(t, "Requests", updated.Panels[0].Title)
	require.NotContains(t, originalPanelIDs, updated.Panels[0].ID)

	var removedPanels int64
	require.NoError(t, db.Model(&models.ObservabilityPanel{}).Where("id IN ?", originalPanelIDs).Count(&removedPanels).Error)
	require.Zero(t, removedPanels, "aggregate replacement removes every old panel")

	badReplacement := replacement
	badReplacement.Description = "must roll back"
	badReplacement.Panels = []dashboard.PanelInput{{
		DatasourceID: ^uint64(0) >> 1,
		Title:        "Broken",
		PanelType:    "stat",
		PromQL:       "up",
		Unit:         "none",
		SortOrder:    0,
		Width:        12,
	}}
	_, err = dashboardService.Update(ctx, 0, "", "", created.ID, badReplacement)
	requireDashboardBizCode(t, err, apperr.CodeObservabilityDashboardInvalidPanel)

	afterRollback, err := dashboardService.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, replacement.Description, afterRollback.Description)
	require.Len(t, afterRollback.Panels, 1)
	require.Equal(t, updated.Panels[0].ID, afterRollback.Panels[0].ID)
	require.Equal(t, "Requests", afterRollback.Panels[0].Title)

	err = datasourceService.Delete(ctx, 0, "", "", primaryDatasource.ID)
	requireDashboardBizCode(t, err, apperr.CodeObservabilityDatasourceInUse)

	require.NoError(t, dashboardService.Delete(ctx, 0, "", "", created.ID))
	require.NoError(t, datasourceService.Delete(ctx, 0, "", "", primaryDatasource.ID))

	reused, err := dashboardService.Create(ctx, 0, "", "", dashboard.SaveRequest{
		Name:             created.Name,
		RefreshIntervalS: 30,
		TimeRange:        "15m",
		Panels:           []dashboard.PanelInput{},
	})
	require.NoError(t, err, "a soft-deleted dashboard name is reusable")
	require.NotEqual(t, created.ID, reused.ID)
}

func requireDashboardBizCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	require.Error(t, err)
	bizErr, ok := apperr.AsBiz(err)
	require.True(t, ok, "expected business error, got %T: %v", err, err)
	require.Equal(t, code, bizErr.Code)
}
