package config

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
)

type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	JWT           JWTConfig           `mapstructure:"jwt"`
	Auth          AuthConfig          `mapstructure:"auth"`
	Log           LogConfig           `mapstructure:"log"`
	CORS          CORSConfig          `mapstructure:"cors"`
	I18n          I18nConfig          `mapstructure:"i18n"`
	Boot          BootstrapConfig     `mapstructure:"bootstrap"`
	Vault         VaultConfig         `mapstructure:"vault"`
	Assets        AssetsConfig        `mapstructure:"assets"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	Delivery      DeliveryConfig      `mapstructure:"delivery"`
}

type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type DatabaseConfig struct {
	Driver          string        `mapstructure:"driver"`
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

type AuthConfig struct {
	BcryptCost     int                  `mapstructure:"bcrypt_cost"`
	LoginRateLimit LoginRateLimitConfig `mapstructure:"login_rate_limit"`
}

type LoginRateLimitConfig struct {
	PerIP       int           `mapstructure:"per_ip"`
	PerUsername int           `mapstructure:"per_username"`
	Window      time.Duration `mapstructure:"window"`
	Block       time.Duration `mapstructure:"block"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowedMethods   []string `mapstructure:"allowed_methods"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
}

type I18nConfig struct {
	DefaultLang string   `mapstructure:"default_lang"`
	Supported   []string `mapstructure:"supported"`
}

type BootstrapConfig struct {
	AdminUsername string `mapstructure:"admin_username"`
	AdminEmail    string `mapstructure:"admin_email"`
}

type VaultConfig struct {
	MasterKey     string `mapstructure:"master_key"`
	MasterKeyFile string `mapstructure:"master_key_file"`
}

type AssetsConfig struct {
	SyncCron             string        `mapstructure:"sync_cron"`
	SyncStartupDelay     time.Duration `mapstructure:"sync_startup_delay"`
	SyncRunRetentionDays int           `mapstructure:"sync_run_retention_days"`
	AWSRequestTimeout    time.Duration `mapstructure:"aws_request_timeout"`
}

type ObservabilityConfig struct {
	AllowedPrivateCIDRs []string      `mapstructure:"allowed_private_cidrs"`
	QueryTimeout        time.Duration `mapstructure:"query_timeout"`
	MaxBatchQueries     int           `mapstructure:"max_batch_queries"`
	MaxConcurrent       int           `mapstructure:"max_concurrent"`
	MaxRange            time.Duration `mapstructure:"max_range"`
	MinStep             time.Duration `mapstructure:"min_step"`
	MaxPointsPerSeries  int           `mapstructure:"max_points_per_series"`
	MaxSeries           int           `mapstructure:"max_series"`
	MaxResponseBytes    int64         `mapstructure:"max_response_bytes"`
	MaxEnrichmentIPs    int           `mapstructure:"max_enrichment_ips"`
}

type DeliveryConfig struct {
	WorkerConcurrency   int           `mapstructure:"worker_concurrency"`
	LeaseDuration       time.Duration `mapstructure:"lease_duration"`
	LeaseRenewInterval  time.Duration `mapstructure:"lease_renew_interval"`
	DefaultStageTimeout time.Duration `mapstructure:"default_stage_timeout"`
	MaxStageTimeout     time.Duration `mapstructure:"max_stage_timeout"`
	ReconcileInterval   time.Duration `mapstructure:"reconcile_interval"`
	EventRetentionDays  int           `mapstructure:"event_retention_days"`
	SSEHeartbeat        time.Duration `mapstructure:"sse_heartbeat"`
	SSEMaxConnections   int           `mapstructure:"sse_max_connections"`
}

const (
	DeliveryMinWorkerConcurrency = 1
	DeliveryMaxWorkerConcurrency = 32

	DeliveryMinSSEMaxConnections = 1
	DeliveryMaxSSEMaxConnections = 1000

	DeliveryMinEventRetentionDays = 1
	DeliveryMaxEventRetentionDays = 3650

	DeliveryMinLeaseDuration = 10 * time.Second
	DeliveryMaxLeaseDuration = 5 * time.Minute

	DeliveryMinLeaseRenewInterval = time.Second
	DeliveryMaxLeaseRenewInterval = time.Minute

	DeliveryMinStageTimeout = time.Minute
	DeliveryMaxStageTimeout = 24 * time.Hour

	DeliveryMinReconcileInterval = time.Second
	DeliveryMaxReconcileInterval = 5 * time.Minute

	DeliveryMinSSEHeartbeat = 5 * time.Second
	DeliveryMaxSSEHeartbeat = 5 * time.Minute
)

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("OPTIMUS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetDefault("assets.sync_cron", "*/15 * * * *")
	v.SetDefault("assets.sync_startup_delay", 30*time.Second)
	v.SetDefault("assets.sync_run_retention_days", 90)
	v.SetDefault("assets.aws_request_timeout", 30*time.Second)
	v.SetDefault("observability.allowed_private_cidrs", []string{})
	v.SetDefault("observability.query_timeout", 15*time.Second)
	v.SetDefault("observability.max_batch_queries", 12)
	v.SetDefault("observability.max_concurrent", 4)
	v.SetDefault("observability.max_range", 168*time.Hour)
	v.SetDefault("observability.min_step", 15*time.Second)
	v.SetDefault("observability.max_points_per_series", 11000)
	v.SetDefault("observability.max_series", 1000)
	v.SetDefault("observability.max_response_bytes", int64(16777216))
	v.SetDefault("observability.max_enrichment_ips", 100)
	v.SetDefault("delivery.worker_concurrency", 4)
	v.SetDefault("delivery.lease_duration", 30*time.Second)
	v.SetDefault("delivery.lease_renew_interval", 10*time.Second)
	v.SetDefault("delivery.default_stage_timeout", 10*time.Minute)
	v.SetDefault("delivery.max_stage_timeout", 30*time.Minute)
	v.SetDefault("delivery.reconcile_interval", 15*time.Second)
	v.SetDefault("delivery.event_retention_days", 180)
	v.SetDefault("delivery.sse_heartbeat", 20*time.Second)
	v.SetDefault("delivery.sse_max_connections", 100)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if cfg.JWT.Secret != "" && len(cfg.JWT.Secret) < 32 {
		return nil, fmt.Errorf("jwt.secret too short: must be >= 32 bytes, got %d", len(cfg.JWT.Secret))
	}
	return cfg, nil
}

// ValidateStrict enforces that all sensitive fields are populated.
// Called at server startup but skipped in tests.
func (c *Config) ValidateStrict() error {
	if c.JWT.Secret == "" {
		return errors.New("jwt.secret is required (set OPTIMUS_JWT_SECRET)")
	}
	if len(c.JWT.Secret) < 32 {
		return errors.New("jwt.secret must be >= 32 bytes")
	}
	if c.Database.DSN == "" {
		return errors.New("database.dsn is required")
	}
	if strings.TrimSpace(c.Assets.SyncCron) == "" {
		return errors.New("assets.sync_cron is required")
	}
	if _, err := cron.ParseStandard(c.Assets.SyncCron); err != nil {
		return fmt.Errorf("assets.sync_cron is invalid: %w", err)
	}
	if c.Assets.SyncStartupDelay < 0 {
		return errors.New("assets.sync_startup_delay must be >= 0")
	}
	if c.Assets.SyncRunRetentionDays <= 0 {
		return errors.New("assets.sync_run_retention_days must be > 0")
	}
	if c.Assets.AWSRequestTimeout <= 0 {
		return errors.New("assets.aws_request_timeout must be > 0")
	}
	if err := c.validateObservability(); err != nil {
		return err
	}
	if err := c.validateDelivery(); err != nil {
		return err
	}
	return nil
}

func (c *Config) validateObservability() error {
	metadata := []netip.Addr{netip.MustParseAddr("169.254.169.254"), netip.MustParseAddr("fd00:ec2::254")}
	for _, raw := range c.Observability.AllowedPrivateCIDRs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("observability.allowed_private_cidrs contains invalid CIDR: %w", err)
		}
		for _, addr := range metadata {
			if prefix.Contains(addr) {
				return errors.New("observability.allowed_private_cidrs must not include cloud metadata addresses")
			}
		}
	}
	o := c.Observability
	if o.QueryTimeout <= 0 || o.MaxBatchQueries <= 0 || o.MaxConcurrent <= 0 ||
		o.MaxRange <= 0 || o.MinStep <= 0 || o.MaxPointsPerSeries <= 0 || o.MaxSeries <= 0 ||
		o.MaxResponseBytes <= 0 || o.MaxEnrichmentIPs <= 0 {
		return errors.New("observability limits must be > 0")
	}
	if o.MaxConcurrent > o.MaxBatchQueries {
		return errors.New("observability.max_concurrent must not exceed max_batch_queries")
	}
	return nil
}

func (c *Config) validateDelivery() error {
	d := c.Delivery
	if err := validateDeliveryIntRange("delivery.worker_concurrency", d.WorkerConcurrency, DeliveryMinWorkerConcurrency, DeliveryMaxWorkerConcurrency); err != nil {
		return err
	}
	if err := validateDeliveryDurationRange("delivery.lease_duration", d.LeaseDuration, DeliveryMinLeaseDuration, DeliveryMaxLeaseDuration); err != nil {
		return err
	}
	if err := validateDeliveryDurationRange("delivery.lease_renew_interval", d.LeaseRenewInterval, DeliveryMinLeaseRenewInterval, DeliveryMaxLeaseRenewInterval); err != nil {
		return err
	}
	if err := validateDeliveryDurationRange("delivery.default_stage_timeout", d.DefaultStageTimeout, DeliveryMinStageTimeout, DeliveryMaxStageTimeout); err != nil {
		return err
	}
	if err := validateDeliveryDurationRange("delivery.max_stage_timeout", d.MaxStageTimeout, DeliveryMinStageTimeout, DeliveryMaxStageTimeout); err != nil {
		return err
	}
	if err := validateDeliveryDurationRange("delivery.reconcile_interval", d.ReconcileInterval, DeliveryMinReconcileInterval, DeliveryMaxReconcileInterval); err != nil {
		return err
	}
	if err := validateDeliveryIntRange("delivery.event_retention_days", d.EventRetentionDays, DeliveryMinEventRetentionDays, DeliveryMaxEventRetentionDays); err != nil {
		return err
	}
	if err := validateDeliveryDurationRange("delivery.sse_heartbeat", d.SSEHeartbeat, DeliveryMinSSEHeartbeat, DeliveryMaxSSEHeartbeat); err != nil {
		return err
	}
	if err := validateDeliveryIntRange("delivery.sse_max_connections", d.SSEMaxConnections, DeliveryMinSSEMaxConnections, DeliveryMaxSSEMaxConnections); err != nil {
		return err
	}
	if d.LeaseRenewInterval >= d.LeaseDuration {
		return errors.New("delivery.lease_renew_interval must be < delivery.lease_duration")
	}
	if d.DefaultStageTimeout > d.MaxStageTimeout {
		return errors.New("delivery.default_stage_timeout must be <= delivery.max_stage_timeout")
	}
	return nil
}

func validateDeliveryIntRange(field string, value, minimum, maximum int) error {
	if value < minimum || value > maximum {
		return fmt.Errorf("%s must be within allowed range [%d, %d]", field, minimum, maximum)
	}
	return nil
}

func validateDeliveryDurationRange(field string, value, minimum, maximum time.Duration) error {
	if value < minimum || value > maximum {
		return fmt.Errorf("%s must be within allowed range [%s, %s]", field, minimum, maximum)
	}
	return nil
}

// ValidateForMigrate enforces only DB connectivity. JWT secret is irrelevant
// to schema migrations and forcing it would require operators to set a dummy
// value in the migrate service env.
func (c *Config) ValidateForMigrate() error {
	if c.Database.DSN == "" {
		return errors.New("database.dsn is required")
	}
	return nil
}
