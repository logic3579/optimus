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
