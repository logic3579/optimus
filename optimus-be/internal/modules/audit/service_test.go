//go:build dbtest

package audit_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/pagination"
	"optimus-be/internal/modules/audit"
)

func TestService_List_FiltersByAction(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer td()
	rec := audit.NewRecorder(gdb)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		require.NoError(t, rec.Record(ctx, audit.Event{Action: "user.create"}))
	}
	require.NoError(t, rec.Record(ctx, audit.Event{Action: "user.delete"}))

	svc := audit.NewService(audit.NewRepo(gdb))
	page, err := svc.List(ctx, audit.ListQuery{Action: "user.create"}, pagination.Params{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.EqualValues(t, 3, page.Total)
	for _, it := range page.Items {
		require.Equal(t, "user.create", it.Action)
	}
}

func TestService_List_FiltersByTimeRange(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer td()
	rec := audit.NewRecorder(gdb)
	ctx := context.Background()
	require.NoError(t, rec.Record(ctx, audit.Event{Action: "x"}))

	future := time.Now().Add(24 * time.Hour)
	svc := audit.NewService(audit.NewRepo(gdb))
	page, err := svc.List(ctx, audit.ListQuery{Start: &future}, pagination.Params{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.EqualValues(t, 0, page.Total)
}
