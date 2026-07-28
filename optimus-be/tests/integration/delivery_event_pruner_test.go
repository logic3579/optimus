//go:build dbtest

package integration_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"optimus-be/internal/models"
	deliveryevent "optimus-be/internal/modules/delivery/event"
)

func TestDeliveryEventPrunerBatchesAndRetainsSummaries(t *testing.T) {
	_, db := setupServer(t)
	run, first, next := seedRetryOrigin(t, db, "pruner")
	now := time.Now().UTC()
	approval := &models.DeliveryApproval{RunID: run.ID, RunStageID: next.ID, RequestedAt: now, Decision: models.DeliveryApprovalPending, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, db.Create(approval).Error)

	old := make([]models.DeliveryRunEvent, 501)
	for i := range old {
		old[i] = models.DeliveryRunEvent{RunID: run.ID, RunStageID: &first.ID, EventType: "test.old", ActorType: models.DeliveryEventActorSystem, OccurredAt: now.AddDate(0, 0, -181), Metadata: datatypes.JSON(`{}`)}
	}
	require.NoError(t, db.CreateInBatches(&old, 100).Error)
	boundary := models.DeliveryRunEvent{RunID: run.ID, EventType: "test.boundary", ActorType: models.DeliveryEventActorSystem, OccurredAt: now.AddDate(0, 0, -180).Add(time.Minute), Metadata: datatypes.JSON(`{}`)}
	after := models.DeliveryRunEvent{RunID: run.ID, EventType: "test.after", ActorType: models.DeliveryEventActorSystem, OccurredAt: boundary.OccurredAt.Add(time.Minute), Metadata: datatypes.JSON(`{}`)}
	require.NoError(t, db.Create(&boundary).Error)
	require.NoError(t, db.Create(&after).Error)

	deleted, err := deliveryevent.NewPruner(db, slog.New(slog.NewTextHandler(io.Discard, nil)), 180, 0).Prune(context.Background())
	require.NoError(t, err)
	require.Equal(t, 501, deleted)
	for _, id := range []uint64{boundary.ID, after.ID} {
		var count int64
		require.NoError(t, db.Model(&models.DeliveryRunEvent{}).Where("id = ?", id).Count(&count).Error)
		require.Equal(t, int64(1), count)
	}
	assertSummaryExists(t, db, &models.DeliveryRun{}, run.ID)
	assertSummaryExists(t, db, &models.DeliveryRunStage{}, first.ID)
	assertSummaryExists(t, db, &models.DeliveryRunStage{}, next.ID)
	assertSummaryExists(t, db, &models.DeliveryApproval{}, approval.ID)

	deleted, err = deliveryevent.NewPruner(db, nil, 180, 0).Prune(context.Background())
	require.NoError(t, err)
	require.Zero(t, deleted)
}

func TestDeliveryEventPrunerReportsZeroWhenDeleteTransactionRollsBack(t *testing.T) {
	_, db := setupServer(t)
	run, first, _ := seedRetryOrigin(t, db, "pruner-rollback")
	event := &models.DeliveryRunEvent{RunID: run.ID, RunStageID: &first.ID, EventType: "test.old", ActorType: models.DeliveryEventActorSystem, OccurredAt: time.Now().UTC().AddDate(0, 0, -181), Metadata: datatypes.JSON(`{}`)}
	require.NoError(t, db.Create(event).Error)
	require.NoError(t, db.Exec(`CREATE FUNCTION delivery_pruner_test_delay() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.2); RETURN OLD; END $$`).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER delivery_pruner_test_delay BEFORE DELETE ON delivery_run_events FOR EACH ROW EXECUTE FUNCTION delivery_pruner_test_delay()`).Error)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	deleted, err := deliveryevent.NewPruner(db, nil, 180, 0).Prune(ctx)
	require.Error(t, err)
	require.Zero(t, deleted)
	var count int64
	require.NoError(t, db.Model(&models.DeliveryRunEvent{}).Where("id = ?", event.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func assertSummaryExists(t *testing.T, db *gorm.DB, model any, id uint64) {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(model).Where("id = ?", id).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
