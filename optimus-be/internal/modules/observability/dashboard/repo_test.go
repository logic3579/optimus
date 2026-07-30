//go:build dbtest

package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/tests/dbtest"
)

type rejectingAudit struct{}

func (rejectingAudit) Record(context.Context, audit.Event) error {
	return errors.New("audit unavailable")
}

func testSave(ds uint64, name string) SaveRequest {
	return SaveRequest{Name: name, Description: "desc", RefreshIntervalS: 30, TimeRange: "1h", Panels: []PanelInput{{DatasourceID: ds, Title: "CPU", PanelType: "time_series", PromQL: `sum(rate(secret_tenant_metric[5m]))`, Unit: "cores", SortOrder: 0, Width: 12}}}
}

func TestAggregateCRUDRollbackAuditAndDatasourceState(t *testing.T) {
	gdb, done := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(done)
	user := dbtest.SeedUser(t, gdb, "dashboard-user")
	ds := &models.ObservabilityDatasource{Name: "prom", BaseURL: "https://prom.example", AuthType: "none"}
	require.NoError(t, gdb.Create(ds).Error)
	svc := NewService(NewRepo(gdb), audit.NewRecorder(gdb))
	created, err := svc.Create(t.Context(), user.ID, "127.0.0.1", "test", testSave(ds.ID, "Ops"))
	require.NoError(t, err)
	require.Len(t, created.Panels, 1)
	items, err := svc.List(t.Context(), ListQuery{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, items.Total)
	updatedReq := testSave(ds.ID, "Ops updated")
	updatedReq.Panels = append(updatedReq.Panels, PanelInput{DatasourceID: ds.ID, Title: "Memory", PanelType: "stat", PromQL: "memory_secret", Unit: "bytes", SortOrder: 1, Width: 6})
	updated, err := svc.Update(t.Context(), user.ID, "", "", created.ID, updatedReq)
	require.NoError(t, err)
	require.Len(t, updated.Panels, 2)
	require.Equal(t, "Ops updated", updated.Name)
	var logs []models.AuditLog
	require.NoError(t, gdb.Where("target_type=?", "observability_dashboard").Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	for _, log := range logs {
		require.NotContains(t, string(log.Payload), "secret")
		var p map[string]any
		require.NoError(t, json.Unmarshal(log.Payload, &p))
		require.Contains(t, p, "panel_count")
		require.Contains(t, p, "panel_fingerprints")
	}
	bad := updatedReq
	bad.Panels[1].SortOrder = 0
	_, err = svc.Update(t.Context(), user.ID, "", "", created.ID, bad)
	require.Error(t, err)
	still, err := svc.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.Len(t, still.Panels, 2)
	require.NoError(t, gdb.Delete(ds).Error)
	_, err = svc.Update(t.Context(), user.ID, "", "", created.ID, updatedReq)
	require.Error(t, err)
	require.NoError(t, gdb.Unscoped().Model(ds).Update("deleted_at", nil).Error)
	require.NoError(t, svc.Delete(t.Context(), user.ID, "", "", created.ID))
	var panelCount int64
	require.NoError(t, gdb.Model(&models.ObservabilityPanel{}).Where("dashboard_id=?", created.ID).Count(&panelCount).Error)
	require.Zero(t, panelCount)
	_, err = svc.Get(t.Context(), created.ID)
	require.Error(t, err)
}

func TestDatasourceParentLockBlocksConcurrentSoftDelete(t *testing.T) {
	gdb, done := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(done)
	ds := &models.ObservabilityDatasource{Name: "locked-prom", BaseURL: "https://prom.example", AuthType: "none"}
	require.NoError(t, gdb.Create(ds).Error)
	r := NewRepo(gdb)
	tx := gdb.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { tx.Rollback() })
	require.NoError(t, r.LockActiveDatasourcesTx(t.Context(), tx, []uint64{ds.ID}))
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	err := gdb.WithContext(ctx).Delete(ds).Error
	require.Error(t, err)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	require.True(t, strings.Contains(err.Error(), "context deadline exceeded"))
}

func TestAggregateRollsBackWhenTransactionalAuditFails(t *testing.T) {
	gdb, done := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(done)
	ds := &models.ObservabilityDatasource{Name: "rollback-prom", BaseURL: "https://prom.example", AuthType: "none"}
	require.NoError(t, gdb.Create(ds).Error)
	svc := &Service{repo: NewRepo(gdb), auditTx: func(*gorm.DB) auditWriter { return rejectingAudit{} }}
	_, err := svc.Create(t.Context(), 0, "", "", testSave(ds.ID, "must-rollback"))
	require.ErrorContains(t, err, "audit unavailable")
	var dashboards, panels int64
	require.NoError(t, gdb.Model(&models.ObservabilityDashboard{}).Where("name=?", "must-rollback").Count(&dashboards).Error)
	require.NoError(t, gdb.Model(&models.ObservabilityPanel{}).Count(&panels).Error)
	require.Zero(t, dashboards)
	require.Zero(t, panels)
}
