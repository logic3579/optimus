//go:build dbtest

package dbtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/models"
)

func SeedCloudKey(t *testing.T, db *gorm.DB, name string) *models.CredentialCloudKey {
	t.Helper()
	row := &models.CredentialCloudKey{
		Name:               name,
		Provider:           "aws",
		AccessKeyIDEnc:     []byte("access-key"),
		SecretAccessKeyEnc: []byte("secret-key"),
	}
	require.NoError(t, db.Create(row).Error)
	return row
}

func SeedCloudAccount(t *testing.T, db *gorm.DB, cloudKeyID uint64, name string, regions ...string) *models.CloudAccount {
	t.Helper()
	row := &models.CloudAccount{
		Name:           name,
		Provider:       "aws",
		CloudKeyID:     cloudKeyID,
		EnabledRegions: models.StringArray(regions),
		Enabled:        true,
	}
	require.NoError(t, db.Create(row).Error)
	return row
}

func SeedAWSInstance(t *testing.T, db *gorm.DB, accountID uint64, region, instanceID string) *models.AWSInstance {
	t.Helper()
	row := &models.AWSInstance{
		CloudAccountID: accountID,
		Region:         region,
		InstanceID:     instanceID,
		LastSeenAt:     time.Now(),
	}
	require.NoError(t, db.Create(row).Error)
	return row
}

func SeedUser(t *testing.T, db *gorm.DB, name string) *models.User {
	t.Helper()
	row := &models.User{Username: name, Email: name + "@example.test", PasswordHash: "hash"}
	require.NoError(t, db.Create(row).Error)
	return row
}

func SeedHTTPCredential(t *testing.T, db *gorm.DB, userID uint64, name, authType string) *models.HTTPCredential {
	t.Helper()
	username := "reader"
	row := &models.HTTPCredential{Name: name, AuthType: authType, SecretCiphertext: []byte("encrypted"), CreatedByUserID: &userID}
	if authType == "basic" {
		row.Username = &username
	}
	require.NoError(t, db.Create(row).Error)
	return row
}

func SeedCluster(t *testing.T, db *gorm.DB, name string) *models.Cluster {
	t.Helper()
	kubeconfig := &models.CredentialKubeconfig{Name: name + "-kubeconfig", KubeconfigEnc: []byte("encrypted")}
	require.NoError(t, db.Create(kubeconfig).Error)
	row := &models.Cluster{Name: name, KubeconfigID: kubeconfig.ID, Context: "default"}
	require.NoError(t, db.Create(row).Error)
	return row
}

func SeedObservabilityDatasource(t *testing.T, db *gorm.DB, credentialID, clusterID uint64) *models.ObservabilityDatasource {
	t.Helper()
	row := &models.ObservabilityDatasource{Name: "prometheus", BaseURL: "https://prometheus.example.test", AuthType: "basic", HTTPCredentialID: &credentialID, ClusterID: &clusterID}
	require.NoError(t, db.Create(row).Error)
	return row
}

func SeedObservabilityDashboard(t *testing.T, db *gorm.DB, userID uint64) *models.ObservabilityDashboard {
	t.Helper()
	row := &models.ObservabilityDashboard{Name: "overview", RefreshIntervalS: 30, TimeRange: "1h", CreatedByUserID: &userID}
	require.NoError(t, db.Create(row).Error)
	return row
}

func SeedObservabilityPanel(t *testing.T, db *gorm.DB, dashboardID, datasourceID uint64, panelType string, width int) *models.ObservabilityPanel {
	t.Helper()
	row := &models.ObservabilityPanel{DashboardID: dashboardID, DatasourceID: datasourceID, Title: "CPU", PanelType: panelType, PromQL: "up", SortOrder: 0, Width: width}
	require.NoError(t, db.Create(row).Error)
	return row
}
