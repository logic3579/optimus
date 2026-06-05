package config_test

import (
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
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("OPTIMUS_SERVER_PORT", "9090")
	t.Setenv("OPTIMUS_JWT_SECRET", "x_very_long_jwt_secret_for_testing_only_32+")
	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, 9090, cfg.Server.Port)
	require.Equal(t, "x_very_long_jwt_secret_for_testing_only_32+", cfg.JWT.Secret)
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
