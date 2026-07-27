package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/config"
)

var deliveryEnvKeys = []string{
	"OPTIMUS_DELIVERY_WORKER_CONCURRENCY",
	"OPTIMUS_DELIVERY_LEASE_DURATION",
	"OPTIMUS_DELIVERY_LEASE_RENEW_INTERVAL",
	"OPTIMUS_DELIVERY_DEFAULT_STAGE_TIMEOUT",
	"OPTIMUS_DELIVERY_MAX_STAGE_TIMEOUT",
	"OPTIMUS_DELIVERY_RECONCILE_INTERVAL",
	"OPTIMUS_DELIVERY_EVENT_RETENTION_DAYS",
	"OPTIMUS_DELIVERY_SSE_HEARTBEAT",
	"OPTIMUS_DELIVERY_SSE_MAX_CONNECTIONS",
}

func clearDeliveryEnv(t *testing.T) {
	t.Helper()
	for _, key := range deliveryEnvKeys {
		t.Setenv(key, "")
	}
}

func deliveryDefaults() config.DeliveryConfig {
	return config.DeliveryConfig{
		WorkerConcurrency: 4, LeaseDuration: 30 * time.Second, LeaseRenewInterval: 10 * time.Second,
		DefaultStageTimeout: 10 * time.Minute, MaxStageTimeout: 30 * time.Minute,
		ReconcileInterval: 15 * time.Second, EventRetentionDays: 180, SSEHeartbeat: 20 * time.Second,
		SSEMaxConnections: 100,
	}
}

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
	require.Empty(t, cfg.Observability.AllowedPrivateCIDRs)
	require.Equal(t, 15*time.Second, cfg.Observability.QueryTimeout)
	require.Equal(t, 12, cfg.Observability.MaxBatchQueries)
	require.Equal(t, 4, cfg.Observability.MaxConcurrent)
	require.Equal(t, 168*time.Hour, cfg.Observability.MaxRange)
	require.Equal(t, 15*time.Second, cfg.Observability.MinStep)
	require.Equal(t, 11000, cfg.Observability.MaxPointsPerSeries)
	require.Equal(t, 1000, cfg.Observability.MaxSeries)
	require.Equal(t, int64(16777216), cfg.Observability.MaxResponseBytes)
	require.Equal(t, 100, cfg.Observability.MaxEnrichmentIPs)
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

func TestLoadDeliveryDefaults(t *testing.T) {
	clearDeliveryEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  port: 8080\n"), 0o600))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, deliveryDefaults(), cfg.Delivery)
}

func TestLoadDeliveryDefaultsFromYAML(t *testing.T) {
	clearDeliveryEnv(t)

	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, deliveryDefaults(), cfg.Delivery)
}

func TestLoadDeliveryEnvOverride(t *testing.T) {
	values := map[string]string{
		"OPTIMUS_DELIVERY_WORKER_CONCURRENCY":    "7",
		"OPTIMUS_DELIVERY_LEASE_DURATION":        "45s",
		"OPTIMUS_DELIVERY_LEASE_RENEW_INTERVAL":  "12s",
		"OPTIMUS_DELIVERY_DEFAULT_STAGE_TIMEOUT": "12m",
		"OPTIMUS_DELIVERY_MAX_STAGE_TIMEOUT":     "45m",
		"OPTIMUS_DELIVERY_RECONCILE_INTERVAL":    "25s",
		"OPTIMUS_DELIVERY_EVENT_RETENTION_DAYS":  "365",
		"OPTIMUS_DELIVERY_SSE_HEARTBEAT":         "30s",
		"OPTIMUS_DELIVERY_SSE_MAX_CONNECTIONS":   "250",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}

	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, config.DeliveryConfig{
		WorkerConcurrency: 7, LeaseDuration: 45 * time.Second, LeaseRenewInterval: 12 * time.Second,
		DefaultStageTimeout: 12 * time.Minute, MaxStageTimeout: 45 * time.Minute,
		ReconcileInterval: 25 * time.Second, EventRetentionDays: 365, SSEHeartbeat: 30 * time.Second,
		SSEMaxConnections: 250,
	}, cfg.Delivery)
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
	valid := validStrictConfig()
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

func TestLoad_ObservabilityDefaultsWhenBlockIsOmitted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  port: 8080\n"), 0o600))
	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, 15*time.Second, cfg.Observability.QueryTimeout)
	require.Equal(t, 12, cfg.Observability.MaxBatchQueries)
	require.Empty(t, cfg.Observability.AllowedPrivateCIDRs)
}

func TestLoad_ObservabilityEnvOverride(t *testing.T) {
	t.Setenv("OPTIMUS_OBSERVABILITY_QUERY_TIMEOUT", "9s")
	t.Setenv("OPTIMUS_OBSERVABILITY_MAX_BATCH_QUERIES", "6")
	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, 9*time.Second, cfg.Observability.QueryTimeout)
	require.Equal(t, 6, cfg.Observability.MaxBatchQueries)
}

func TestValidateStrict_RejectsInvalidObservabilityConfig(t *testing.T) {
	valid := validStrictConfig()
	tests := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{"invalid CIDR", func(c *config.Config) { c.Observability.AllowedPrivateCIDRs = []string{"bad"} }},
		{"CIDR containing IPv4 metadata", func(c *config.Config) { c.Observability.AllowedPrivateCIDRs = []string{"169.254.0.0/16"} }},
		{"CIDR containing IPv6 metadata", func(c *config.Config) { c.Observability.AllowedPrivateCIDRs = []string{"fc00::/7"} }},
		{"zero query timeout", func(c *config.Config) { c.Observability.QueryTimeout = 0 }},
		{"zero max batch", func(c *config.Config) { c.Observability.MaxBatchQueries = 0 }},
		{"concurrency over batch", func(c *config.Config) { c.Observability.MaxConcurrent = c.Observability.MaxBatchQueries + 1 }},
		{"zero max range", func(c *config.Config) { c.Observability.MaxRange = 0 }},
		{"zero min step", func(c *config.Config) { c.Observability.MinStep = 0 }},
		{"zero points", func(c *config.Config) { c.Observability.MaxPointsPerSeries = 0 }},
		{"zero series", func(c *config.Config) { c.Observability.MaxSeries = 0 }},
		{"zero response bytes", func(c *config.Config) { c.Observability.MaxResponseBytes = 0 }},
		{"zero enrichment IPs", func(c *config.Config) { c.Observability.MaxEnrichmentIPs = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			require.Error(t, cfg.ValidateStrict())
		})
	}
	require.NoError(t, valid.ValidateStrict())
}

func TestValidateDelivery(t *testing.T) {
	valid := validStrictConfig()
	tests := []struct {
		name     string
		mutate   func(*config.Config)
		expected string
	}{
		{"worker concurrency below minimum", func(c *config.Config) { c.Delivery.WorkerConcurrency = 0 }, "delivery.worker_concurrency must be within allowed range [1, 32]"},
		{"worker concurrency above maximum", func(c *config.Config) { c.Delivery.WorkerConcurrency = 33 }, "delivery.worker_concurrency must be within allowed range [1, 32]"},
		{"lease duration below minimum", func(c *config.Config) { c.Delivery.LeaseDuration = 9 * time.Second }, "delivery.lease_duration must be within allowed range [10s, 5m0s]"},
		{"lease duration above maximum", func(c *config.Config) { c.Delivery.LeaseDuration = 5*time.Minute + time.Second }, "delivery.lease_duration must be within allowed range [10s, 5m0s]"},
		{"lease renew interval below minimum", func(c *config.Config) { c.Delivery.LeaseRenewInterval = 0 }, "delivery.lease_renew_interval must be within allowed range [1s, 1m0s]"},
		{"lease renew interval above maximum", func(c *config.Config) { c.Delivery.LeaseRenewInterval = time.Minute + time.Second }, "delivery.lease_renew_interval must be within allowed range [1s, 1m0s]"},
		{"default stage timeout below minimum", func(c *config.Config) { c.Delivery.DefaultStageTimeout = time.Minute - time.Second }, "delivery.default_stage_timeout must be within allowed range [1m0s, 24h0m0s]"},
		{"default stage timeout above maximum", func(c *config.Config) { c.Delivery.DefaultStageTimeout = 24*time.Hour + time.Second }, "delivery.default_stage_timeout must be within allowed range [1m0s, 24h0m0s]"},
		{"max stage timeout below minimum", func(c *config.Config) { c.Delivery.MaxStageTimeout = time.Minute - time.Second }, "delivery.max_stage_timeout must be within allowed range [1m0s, 24h0m0s]"},
		{"max stage timeout above maximum", func(c *config.Config) { c.Delivery.MaxStageTimeout = 24*time.Hour + time.Second }, "delivery.max_stage_timeout must be within allowed range [1m0s, 24h0m0s]"},
		{"reconcile interval below minimum", func(c *config.Config) { c.Delivery.ReconcileInterval = 0 }, "delivery.reconcile_interval must be within allowed range [1s, 5m0s]"},
		{"reconcile interval above maximum", func(c *config.Config) { c.Delivery.ReconcileInterval = 5*time.Minute + time.Second }, "delivery.reconcile_interval must be within allowed range [1s, 5m0s]"},
		{"event retention days below minimum", func(c *config.Config) { c.Delivery.EventRetentionDays = 0 }, "delivery.event_retention_days must be within allowed range [1, 3650]"},
		{"event retention days above maximum", func(c *config.Config) { c.Delivery.EventRetentionDays = 3651 }, "delivery.event_retention_days must be within allowed range [1, 3650]"},
		{"SSE heartbeat below minimum", func(c *config.Config) { c.Delivery.SSEHeartbeat = 4 * time.Second }, "delivery.sse_heartbeat must be within allowed range [5s, 5m0s]"},
		{"SSE heartbeat above maximum", func(c *config.Config) { c.Delivery.SSEHeartbeat = 5*time.Minute + time.Second }, "delivery.sse_heartbeat must be within allowed range [5s, 5m0s]"},
		{"SSE max connections below minimum", func(c *config.Config) { c.Delivery.SSEMaxConnections = 0 }, "delivery.sse_max_connections must be within allowed range [1, 1000]"},
		{"SSE max connections above maximum", func(c *config.Config) { c.Delivery.SSEMaxConnections = 1001 }, "delivery.sse_max_connections must be within allowed range [1, 1000]"},
		{"negative worker concurrency", func(c *config.Config) { c.Delivery.WorkerConcurrency = -1 }, "delivery.worker_concurrency must be within allowed range [1, 32]"},
		{"negative lease duration", func(c *config.Config) { c.Delivery.LeaseDuration = -time.Second }, "delivery.lease_duration must be within allowed range [10s, 5m0s]"},
		{"negative lease renew interval", func(c *config.Config) { c.Delivery.LeaseRenewInterval = -time.Second }, "delivery.lease_renew_interval must be within allowed range [1s, 1m0s]"},
		{"negative default stage timeout", func(c *config.Config) { c.Delivery.DefaultStageTimeout = -time.Second }, "delivery.default_stage_timeout must be within allowed range [1m0s, 24h0m0s]"},
		{"negative max stage timeout", func(c *config.Config) { c.Delivery.MaxStageTimeout = -time.Second }, "delivery.max_stage_timeout must be within allowed range [1m0s, 24h0m0s]"},
		{"negative reconcile interval", func(c *config.Config) { c.Delivery.ReconcileInterval = -time.Second }, "delivery.reconcile_interval must be within allowed range [1s, 5m0s]"},
		{"negative event retention days", func(c *config.Config) { c.Delivery.EventRetentionDays = -1 }, "delivery.event_retention_days must be within allowed range [1, 3650]"},
		{"negative SSE heartbeat", func(c *config.Config) { c.Delivery.SSEHeartbeat = -time.Second }, "delivery.sse_heartbeat must be within allowed range [5s, 5m0s]"},
		{"negative SSE max connections", func(c *config.Config) { c.Delivery.SSEMaxConnections = -1 }, "delivery.sse_max_connections must be within allowed range [1, 1000]"},
		{"renew interval equals lease duration", func(c *config.Config) { c.Delivery.LeaseRenewInterval = c.Delivery.LeaseDuration }, "delivery.lease_renew_interval must be < delivery.lease_duration"},
		{"default timeout exceeds maximum", func(c *config.Config) { c.Delivery.DefaultStageTimeout = c.Delivery.MaxStageTimeout + time.Second }, "delivery.default_stage_timeout must be <= delivery.max_stage_timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			require.EqualError(t, cfg.ValidateStrict(), tt.expected)
		})
	}
	require.NoError(t, valid.ValidateStrict())
}

func TestValidateDeliveryBoundariesAccepted(t *testing.T) {
	valid := validStrictConfig()
	tests := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{"worker concurrency minimum", func(c *config.Config) { c.Delivery.WorkerConcurrency = 1 }},
		{"worker concurrency maximum", func(c *config.Config) { c.Delivery.WorkerConcurrency = 32 }},
		{"lease duration minimum", func(c *config.Config) {
			c.Delivery.LeaseDuration, c.Delivery.LeaseRenewInterval = 10*time.Second, time.Second
		}},
		{"lease duration maximum", func(c *config.Config) {
			c.Delivery.LeaseDuration, c.Delivery.LeaseRenewInterval = 5*time.Minute, time.Second
		}},
		{"lease renew interval minimum", func(c *config.Config) { c.Delivery.LeaseRenewInterval = time.Second }},
		{"lease renew interval maximum", func(c *config.Config) {
			c.Delivery.LeaseDuration, c.Delivery.LeaseRenewInterval = 5*time.Minute, time.Minute
		}},
		{"default stage timeout minimum", func(c *config.Config) { c.Delivery.DefaultStageTimeout = time.Minute }},
		{"default stage timeout maximum", func(c *config.Config) {
			c.Delivery.DefaultStageTimeout, c.Delivery.MaxStageTimeout = 24*time.Hour, 24*time.Hour
		}},
		{"max stage timeout minimum", func(c *config.Config) {
			c.Delivery.DefaultStageTimeout, c.Delivery.MaxStageTimeout = time.Minute, time.Minute
		}},
		{"max stage timeout maximum", func(c *config.Config) { c.Delivery.MaxStageTimeout = 24 * time.Hour }},
		{"default timeout equals maximum", func(c *config.Config) {
			c.Delivery.DefaultStageTimeout, c.Delivery.MaxStageTimeout = 12*time.Minute, 12*time.Minute
		}},
		{"reconcile interval minimum", func(c *config.Config) { c.Delivery.ReconcileInterval = time.Second }},
		{"reconcile interval maximum", func(c *config.Config) { c.Delivery.ReconcileInterval = 5 * time.Minute }},
		{"event retention days minimum", func(c *config.Config) { c.Delivery.EventRetentionDays = 1 }},
		{"event retention days maximum", func(c *config.Config) { c.Delivery.EventRetentionDays = 3650 }},
		{"SSE heartbeat minimum", func(c *config.Config) { c.Delivery.SSEHeartbeat = 5 * time.Second }},
		{"SSE heartbeat maximum", func(c *config.Config) { c.Delivery.SSEHeartbeat = 5 * time.Minute }},
		{"SSE max connections minimum", func(c *config.Config) { c.Delivery.SSEMaxConnections = 1 }},
		{"SSE max connections maximum", func(c *config.Config) { c.Delivery.SSEMaxConnections = 1000 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			require.NoError(t, cfg.ValidateStrict())
		})
	}
}

func validStrictConfig() config.Config {
	return config.Config{
		Database: config.DatabaseConfig{DSN: "postgres://test"},
		JWT:      config.JWTConfig{Secret: "12345678901234567890123456789012"},
		Assets:   config.AssetsConfig{SyncCron: "*/15 * * * *", SyncRunRetentionDays: 90, AWSRequestTimeout: 30 * time.Second},
		Observability: config.ObservabilityConfig{
			QueryTimeout: 15 * time.Second, MaxBatchQueries: 12, MaxConcurrent: 4,
			MaxRange: 168 * time.Hour, MinStep: 15 * time.Second, MaxPointsPerSeries: 11000,
			MaxSeries: 1000, MaxResponseBytes: 16777216, MaxEnrichmentIPs: 100,
		},
		Delivery: config.DeliveryConfig{
			WorkerConcurrency: 4, LeaseDuration: 30 * time.Second, LeaseRenewInterval: 10 * time.Second,
			DefaultStageTimeout: 10 * time.Minute, MaxStageTimeout: 30 * time.Minute,
			ReconcileInterval: 15 * time.Second, EventRetentionDays: 180, SSEHeartbeat: 20 * time.Second,
			SSEMaxConnections: 100,
		},
	}
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
