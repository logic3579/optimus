package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig    `mapstructure:"server"`
	Database DatabaseConfig  `mapstructure:"database"`
	JWT      JWTConfig       `mapstructure:"jwt"`
	Auth     AuthConfig      `mapstructure:"auth"`
	Log      LogConfig       `mapstructure:"log"`
	CORS     CORSConfig      `mapstructure:"cors"`
	I18n     I18nConfig      `mapstructure:"i18n"`
	Boot     BootstrapConfig `mapstructure:"bootstrap"`
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
	BcryptCost     int                 `mapstructure:"bcrypt_cost"`
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

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("OPTIMUS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
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
	return nil
}
