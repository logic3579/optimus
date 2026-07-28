package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Composition belongs in main; this guard prevents delivery from silently
// reaching private P1/P3 repositories or losing its shared lifecycle context.
func TestDeliveryCompositionContract(t *testing.T) {
	source, err := os.ReadFile("main.go")
	require.NoError(t, err)
	text := string(source)
	for _, required := range []string{
		"appsCoordinator := release.NewCoordinator(gdb)",
		"appsModule.DeliveryAdapters(appsCoordinator)",
		"appsModule.SetDeliveryGovernance(deliveryModule.Governance)",
		"SetDeliveryApplicationCounter(deliveryModule.ApplicationCounter)",
		"deliveryModule.MountRoutes(protected, permCache)",
		"deliveryModule.Start(deliveryCtx, cfg.Delivery.ReconcileInterval, logger)",
		"deliveryModule.Shutdown(deliveryShutdownCtx)",
	} {
		require.Contains(t, text, required)
	}
	require.Equal(t, 1, strings.Count(text, "release.NewCoordinator(gdb)"))
}

func TestDatabaseClosesOnlyAfterAllBackgroundModulesDrain(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		deliveryErr, assetsErr error
		wantClose              bool
	}{
		{"both drained", nil, nil, true},
		{"delivery timed out", context.DeadlineExceeded, nil, false},
		{"assets timed out", nil, context.DeadlineExceeded, false},
		{"both failed", errors.New("delivery"), errors.New("assets"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			closed := false
			got := closeDBIfDrained(tc.deliveryErr, tc.assetsErr, func() error { closed = true; return nil })
			require.Equal(t, tc.wantClose, got)
			require.Equal(t, tc.wantClose, closed)
		})
	}
}

func TestDeliveryWorkerOwnerIsBoundedAndNonEmpty(t *testing.T) {
	owner := deliveryWorkerOwner()
	require.NotEmpty(t, owner)
	require.LessOrEqual(t, len(owner), 128)
}
