//go:build dbtest

package integration_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/tests/dbtest"
)

func TestP5SchemaConstraints(t *testing.T) {
	_, db := setupServer(t)

	user := dbtest.SeedUser(t, db, "p5-schema")
	credential := dbtest.SeedHTTPCredential(t, db, user.ID, "prom-basic", "basic")
	cluster := dbtest.SeedCluster(t, db, "p5-cluster")
	datasource := dbtest.SeedObservabilityDatasource(t, db, credential.ID, cluster.ID)
	dashboard := dbtest.SeedObservabilityDashboard(t, db, user.ID)
	dbtest.SeedObservabilityPanel(t, db, dashboard.ID, datasource.ID, "time_series", 12)

	require.Error(t, db.Exec(`INSERT INTO credentials_http_credentials (name, auth_type, username, secret_ciphertext) VALUES (?, 'basic', 'reader', ?)`, credential.Name, []byte("encrypted")).Error, "duplicate active HTTP credential name")
	require.Error(t, db.Exec(`INSERT INTO credentials_http_credentials (name, auth_type, secret_ciphertext) VALUES ('bad-basic', 'basic', ?)`, []byte("encrypted")).Error, "basic credential without username")
	require.Error(t, db.Exec(`INSERT INTO credentials_http_credentials (name, auth_type, username, secret_ciphertext) VALUES ('bad-bearer', 'bearer', 'reader', ?)`, []byte("encrypted")).Error, "bearer credential with username")
	require.Error(t, db.Exec(`INSERT INTO credentials_http_credentials (name, auth_type, secret_ciphertext) VALUES ('bad-auth', 'none', ?)`, []byte("encrypted")).Error, "invalid HTTP credential auth type")
	require.Error(t, db.Exec(`INSERT INTO observability_datasources (name, base_url, auth_type, http_credential_id) VALUES (?, 'https://other.example.test', 'basic', ?)`, datasource.Name, credential.ID).Error, "duplicate active datasource name")
	require.Error(t, db.Exec(`INSERT INTO observability_dashboards (name) VALUES (?)`, dashboard.Name).Error, "duplicate active dashboard name")
	require.NoError(t, db.Exec(`UPDATE observability_dashboards SET deleted_at = NOW() WHERE id = ?`, dashboard.ID).Error)
	require.NoError(t, db.Exec(`INSERT INTO observability_dashboards (name) VALUES (?)`, dashboard.Name).Error, "soft-deleted dashboard name is reusable")
	require.Error(t, db.Exec(`INSERT INTO observability_datasources (name, base_url, auth_type, http_credential_id) VALUES ('bad-none', 'https://prometheus.example.test', 'none', ?)`, credential.ID).Error, "none auth with credential")
	require.Error(t, db.Exec(`INSERT INTO observability_datasources (name, base_url, auth_type) VALUES ('bad-basic', 'https://prometheus.example.test', 'basic')`).Error, "basic auth without credential")
	require.Error(t, db.Exec(`INSERT INTO observability_datasources (name, base_url, auth_type) VALUES ('bad-auth', 'https://prometheus.example.test', 'oauth2')`).Error, "invalid datasource auth type")
	require.Error(t, db.Exec(`INSERT INTO observability_datasources (name, base_url, auth_type, cluster_id) VALUES ('bad-cluster', 'https://prometheus.example.test', 'none', 9223372036854775807)`).Error, "unknown cluster")
	require.Error(t, db.Exec(`INSERT INTO observability_dashboards (name, refresh_interval_s) VALUES ('too-fast', 14)`).Error, "invalid dashboard refresh interval")
	require.Error(t, db.Exec(`INSERT INTO observability_dashboards (name, refresh_interval_s) VALUES ('too-slow', 3601)`).Error, "dashboard refresh interval above upper bound")
	require.Error(t, db.Exec(`INSERT INTO observability_dashboards (name, time_range) VALUES ('bad-range', '30d')`).Error, "invalid dashboard time range")
	require.Error(t, db.Exec(`INSERT INTO observability_panels (dashboard_id, datasource_id, title, panel_type, promql, sort_order, width) VALUES (?, ?, 'bad-type', 'gauge', 'up', 1, 12)`, dashboard.ID, datasource.ID).Error, "invalid panel type")
	require.Error(t, db.Exec(`INSERT INTO observability_panels (dashboard_id, datasource_id, title, panel_type, promql, sort_order, width) VALUES (?, ?, 'bad-width', 'stat', 'up', 2, 5)`, dashboard.ID, datasource.ID).Error, "invalid panel width")
	require.Error(t, db.Exec(`INSERT INTO observability_panels (dashboard_id, datasource_id, title, panel_type, promql, sort_order, width) VALUES (?, ?, 'blank-query', 'stat', '  ', 3, 12)`, dashboard.ID, datasource.ID).Error, "blank panel query")
	require.Error(t, db.Exec(`INSERT INTO observability_panels (dashboard_id, datasource_id, title, panel_type, promql, sort_order, width) VALUES (?, ?, 'long-query', 'stat', ?, 6, 12)`, dashboard.ID, datasource.ID, strings.Repeat("x", 8193)).Error, "panel query above upper bound")
	require.Error(t, db.Exec(`INSERT INTO observability_panels (dashboard_id, datasource_id, title, panel_type, promql, sort_order, width) VALUES (?, ?, 'duplicate-order', 'stat', 'up', 0, 12)`, dashboard.ID, datasource.ID).Error, "duplicate panel sort order")
	require.Error(t, db.Exec(`INSERT INTO observability_panels (dashboard_id, datasource_id, title, panel_type, promql, sort_order, width) VALUES (9223372036854775807, ?, 'bad-dashboard', 'stat', 'up', 4, 12)`, datasource.ID).Error, "unknown dashboard")
	require.Error(t, db.Exec(`INSERT INTO observability_panels (dashboard_id, datasource_id, title, panel_type, promql, sort_order, width) VALUES (?, 9223372036854775807, 'bad-datasource', 'stat', 'up', 5, 12)`, dashboard.ID).Error, "unknown datasource")

	require.Error(t, db.Exec(`DELETE FROM credentials_http_credentials WHERE id = ?`, credential.ID).Error, "referenced HTTP credential hard delete")
	require.Error(t, db.Exec(`DELETE FROM clusters WHERE id = ?`, cluster.ID).Error, "referenced cluster hard delete")
	require.Error(t, db.Exec(`DELETE FROM observability_datasources WHERE id = ?`, datasource.ID).Error, "referenced datasource hard delete")

	var cascadeDashboardID uint64
	require.NoError(t, db.Raw(`INSERT INTO observability_dashboards (name, created_by_user_id) VALUES ('cascade-overview', ?) RETURNING id`, user.ID).Scan(&cascadeDashboardID).Error)
	dbtest.SeedObservabilityPanel(t, db, cascadeDashboardID, datasource.ID, "table", 6)
	require.NoError(t, db.Exec(`DELETE FROM observability_dashboards WHERE id = ?`, cascadeDashboardID).Error)
	var panelCount int64
	require.NoError(t, db.Table("observability_panels").Where("dashboard_id = ?", cascadeDashboardID).Count(&panelCount).Error)
	require.Zero(t, panelCount, "dashboard hard delete cascades to panels")
}
