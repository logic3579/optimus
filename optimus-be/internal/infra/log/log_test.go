package log_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/log"
)

func TestNew_JSONFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(log.Options{Level: "info", Format: "json", Writer: buf})
	logger.Info("hello", "k", "v")
	require.Contains(t, buf.String(), `"msg":"hello"`)
	require.Contains(t, buf.String(), `"k":"v"`)
}

func TestNew_RespectsLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(log.Options{Level: "warn", Format: "json", Writer: buf})
	logger.Debug("debug-msg")
	logger.Info("info-msg")
	logger.Warn("warn-msg")
	out := buf.String()
	require.False(t, strings.Contains(out, "debug-msg"))
	require.False(t, strings.Contains(out, "info-msg"))
	require.True(t, strings.Contains(out, "warn-msg"))
}

func TestNew_DefaultsToInfo(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(log.Options{Format: "text", Writer: buf})
	logger.Info("v")
	require.Contains(t, buf.String(), "v")
}
