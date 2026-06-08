//go:build dbtest

package audit_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
)

func TestRecorder_Record_InsertsRow(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()

	rec := audit.NewRecorder(gdb)
	uid := uint64(42)
	require.NoError(t, gdb.Create(&models.User{ID: 42, Username: "u42", Email: "u42@x", PasswordHash: "x"}).Error)

	err := rec.Record(context.Background(), audit.Event{
		UserID:     &uid,
		Action:     "user.create",
		TargetType: "user",
		TargetID:   "99",
		Payload:    map[string]any{"after": map[string]any{"username": "alice"}},
		IP:         "127.0.0.1",
		UserAgent:  "go-test",
	})
	require.NoError(t, err)

	var rows []models.AuditLog
	require.NoError(t, gdb.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "user.create", rows[0].Action)
	require.Equal(t, "user", rows[0].TargetType)
	require.Equal(t, "99", rows[0].TargetID)
	require.Equal(t, "127.0.0.1", rows[0].IP)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(rows[0].Payload, &payload))
	require.Contains(t, payload, "after")
}

func TestRecorder_NilPayload_StoresEmptyObject(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()

	rec := audit.NewRecorder(gdb)
	require.NoError(t, rec.Record(context.Background(), audit.Event{Action: "x.y"}))

	var row models.AuditLog
	require.NoError(t, gdb.First(&row).Error)
	require.JSONEq(t, `{}`, string(row.Payload))
}
