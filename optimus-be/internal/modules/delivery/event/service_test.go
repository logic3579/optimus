package event

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"optimus-be/internal/models"
)

func TestServiceReadsOrderedBoundedPagesAfterCursor(t *testing.T) {
	repo := &fakeRepository{exists: true, rows: []models.DeliveryRunEvent{{ID: 3, RunID: 7}, {ID: 4, RunID: 7}}}
	svc := NewService(repo)
	rows, err := svc.ReadAfter(context.Background(), 7, 2)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, uint64(2), repo.cursor)
	require.Equal(t, pageLimit, repo.limit)
}

func TestServiceRejectsUnknownSensitiveMetadataKeysAndOversizedEvents(t *testing.T) {
	for _, key := range []string{"values", "manifest", "notes", "credential", "credentials", "raw_error", "kubeconfig", "authorization", "registry_token", "secret"} {
		t.Run(key, func(t *testing.T) {
			metadata, err := json.Marshal(map[string]any{"nested": map[string]string{key: "secret"}})
			require.NoError(t, err)
			svc := NewService(&fakeRepository{exists: true, rows: []models.DeliveryRunEvent{{ID: 1, RunID: 7, EventType: "run.failed", Metadata: metadata}}})
			_, err = svc.ReadAfter(context.Background(), 7, 0)
			require.Error(t, err)
		})
	}

	metadata := datatypes.JSON([]byte(`{"reason":"` + strings.Repeat("x", maxEventBytes) + `"}`))
	svc := NewService(&fakeRepository{exists: true, rows: []models.DeliveryRunEvent{{ID: 1, RunID: 7, EventType: "run.failed", Metadata: metadata}}})
	_, err := svc.ReadAfter(context.Background(), 7, 0)
	require.Error(t, err)
}

func TestServiceRequiresMetadataObjectAndAllowsKnownNestedKeys(t *testing.T) {
	for _, raw := range []string{`null`, `true`, `"text"`, `12`, `[]`} {
		svc := NewService(&fakeRepository{rows: []models.DeliveryRunEvent{{ID: 1, RunID: 7, Metadata: datatypes.JSON(raw)}}})
		_, err := svc.ReadAfter(context.Background(), 7, 0)
		require.Error(t, err)
	}
	svc := NewService(&fakeRepository{rows: []models.DeliveryRunEvent{{ID: 1, RunID: 7, Metadata: datatypes.JSON(`{"reason":[{"operation_id":"op"}]}`)}}})
	_, err := svc.ReadAfter(context.Background(), 7, 0)
	require.NoError(t, err)
}

func TestServiceRejectsUnorderedOrWrongRunRows(t *testing.T) {
	for _, rows := range [][]models.DeliveryRunEvent{
		{{ID: 2, RunID: 7}, {ID: 1, RunID: 7}},
		{{ID: 2, RunID: 8}},
	} {
		svc := NewService(&fakeRepository{exists: true, rows: rows})
		_, err := svc.ReadAfter(context.Background(), 7, 0)
		require.Error(t, err)
	}
}

type fakeRepository struct {
	exists bool
	rows   []models.DeliveryRunEvent
	err    error
	cursor uint64
	limit  int
}

func (r *fakeRepository) RunExists(context.Context, uint64) (bool, error) { return r.exists, nil }
func (r *fakeRepository) ListAfter(_ context.Context, _ uint64, cursor uint64, limit int) ([]models.DeliveryRunEvent, error) {
	r.cursor, r.limit = cursor, limit
	return r.rows, r.err
}

func eventRow(id uint64) models.DeliveryRunEvent {
	return models.DeliveryRunEvent{ID: id, RunID: 7, EventType: "run.running", ActorType: models.DeliveryEventActorSystem, OccurredAt: time.Unix(1, 0).UTC(), Metadata: datatypes.JSON(`{}`)}
}
