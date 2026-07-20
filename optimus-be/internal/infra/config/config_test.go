package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/config"
)

func TestLoad_DefaultsFromYAML(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, 8080, cfg.Server.Port)
	require.Equal(t, 15*time.Second, cfg.Server.ReadTimeout)
	require.Equal(t, "info", cfg.Log.Level)
	require.Equal(t, []string{"zh-CN", "en-US"}, cfg.I18n.Supported)
	require.Equal(t, "*/15 * * * *", cfg.Assets.SyncCron)
	require.Equal(t, 30*time.Second, cfg.Assets.SyncStartupDelay)
	require.Equal(t, 90, cfg.Assets.SyncRunRetentionDays)
	require.Equal(t, 30*time.Second, cfg.Assets.AWSRequestTimeout)
}

func TestLoad_AssetsDefaultsWhenBlockIsOmitted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  port: 8080\n"), 0o600))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, "*/15 * * * *", cfg.Assets.SyncCron)
	require.Equal(t, 30*time.Second, cfg.Assets.SyncStartupDelay)
	require.Equal(t, 90, cfg.Assets.SyncRunRetentionDays)
	require.Equal(t, 30*time.Second, cfg.Assets.AWSRequestTimeout)
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("OPTIMUS_SERVER_PORT", "9090")
	t.Setenv("OPTIMUS_JWT_SECRET", "x_very_long_jwt_secret_for_testing_only_32+")
	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, 9090, cfg.Server.Port)
	require.Equal(t, "x_very_long_jwt_secret_for_testing_only_32+", cfg.JWT.Secret)
}

func TestLoad_AssetsEnvOverride(t *testing.T) {
	t.Setenv("OPTIMUS_ASSETS_SYNC_CRON", "0 * * * *")
	t.Setenv("OPTIMUS_ASSETS_SYNC_STARTUP_DELAY", "5s")
	t.Setenv("OPTIMUS_ASSETS_SYNC_RUN_RETENTION_DAYS", "30")
	t.Setenv("OPTIMUS_ASSETS_AWS_REQUEST_TIMEOUT", "45s")
	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, "0 * * * *", cfg.Assets.SyncCron)
	require.Equal(t, 5*time.Second, cfg.Assets.SyncStartupDelay)
	require.Equal(t, 30, cfg.Assets.SyncRunRetentionDays)
	require.Equal(t, 45*time.Second, cfg.Assets.AWSRequestTimeout)
}

func TestLoad_RejectsShortJWTSecretWhenProvided(t *testing.T) {
	t.Setenv("OPTIMUS_JWT_SECRET", "tooshort")
	_, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.Error(t, err)
}

func TestValidate_RequiresJWTSecretWhenStrict(t *testing.T) {
	cfg := &config.Config{}
	require.Error(t, cfg.ValidateStrict())
}

func TestValidateStrict_RejectsInvalidAssetsConfig(t *testing.T) {
	valid := config.Config{
		Database: config.DatabaseConfig{DSN: "postgres://test"},
		JWT:      config.JWTConfig{Secret: "12345678901234567890123456789012"},
		Assets: config.AssetsConfig{
			SyncCron:             "*/15 * * * *",
			SyncRunRetentionDays: 90,
			AWSRequestTimeout:    30 * time.Second,
		},
	}
	tests := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{"empty cron", func(c *config.Config) { c.Assets.SyncCron = "" }},
		{"invalid cron", func(c *config.Config) { c.Assets.SyncCron = "not-a-cron" }},
		{"negative delay", func(c *config.Config) { c.Assets.SyncStartupDelay = -time.Second }},
		{"zero retention", func(c *config.Config) { c.Assets.SyncRunRetentionDays = 0 }},
		{"zero request timeout", func(c *config.Config) { c.Assets.AWSRequestTimeout = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := valid
			test.mutate(&cfg)
			require.Error(t, cfg.ValidateStrict())
		})
	}
	require.NoError(t, valid.ValidateStrict())
}

func TestValidateForMigrate_RequiresDSN(t *testing.T) {
	cfg := &config.Config{}
	require.Error(t, cfg.ValidateForMigrate())
}

func TestValidateForMigrate_AcceptsDSNOnly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Database.DSN = "host=localhost port=5432 user=u password=p dbname=d sslmode=disable"
	// Deliberately no JWT secret — migrations don't need it.
	require.NoError(t, cfg.ValidateForMigrate())
}

func TestLoad_VaultDefaultsEmpty(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, "", cfg.Vault.MasterKey)
	require.Equal(t, "", cfg.Vault.MasterKeyFile)
}

func TestLoad_VaultMasterKeyFromEnv(t *testing.T) {
	t.Setenv("OPTIMUS_VAULT_MASTER_KEY", "from-env")
	t.Setenv("OPTIMUS_VAULT_MASTER_KEY_FILE", "/etc/optimus/vault.key")
	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, "from-env", cfg.Vault.MasterKey)
	require.Equal(t, "/etc/optimus/vault.key", cfg.Vault.MasterKeyFile)
}
