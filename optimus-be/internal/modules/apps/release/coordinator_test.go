package release

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
)

const validOperationDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestCoordinatorAcquireFirstOperation(t *testing.T) {
	coordinator, _, now := newCoordinatorHarness()

	result, err := coordinator.Acquire(context.Background(), 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.False(t, result.Replayed)
	require.False(t, result.NeedsReconciliation)
	require.Equal(t, "operation-1", result.Operation.OperationID)
	require.Equal(t, models.AppsReleaseOperationActive, result.Operation.State)
	require.Equal(t, "worker-1", derefString(result.Operation.LeaseOwner))
	require.Equal(t, now.Add(time.Minute), derefTime(result.Operation.LeaseExpiresAt))
}

func TestCoordinatorAcquireSameOperationReplays(t *testing.T) {
	coordinator, _, _ := newCoordinatorHarness()
	ctx := context.Background()

	first, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	replay, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	require.True(t, replay.Acquired)
	require.True(t, replay.Replayed)
	require.Equal(t, first.Operation.ID, replay.Operation.ID)

	otherOwner, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-2", time.Minute)
	require.NoError(t, err)
	require.False(t, otherOwner.Acquired)
	require.True(t, otherOwner.Replayed)
	require.Equal(t, "worker-1", derefString(otherOwner.Operation.LeaseOwner))
}

func TestCoordinatorAcquireDifferentOperationReturnsBusy(t *testing.T) {
	coordinator, _, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	_, err = coordinator.Acquire(ctx, 41, "operation-2", "rollback", "worker-2", time.Minute)
	requireBizError(t, err, apperr.CodeDeliveryOperationBusy, "delivery.execution.operation_busy")
}

func TestCoordinatorRenewRequiresLeaseOwner(t *testing.T) {
	coordinator, _, now := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	requireBizError(t,
		coordinator.Renew(ctx, "operation-1", "worker-2", now.Add(2*time.Minute)),
		apperr.CodeDeliveryOperationBusy,
		"delivery.execution.operation_busy",
	)
	require.NoError(t, coordinator.Renew(ctx, "operation-1", "worker-1", now.Add(2*time.Minute)))

	operation, err := coordinator.Inspect(ctx, "operation-1")
	require.NoError(t, err)
	require.Equal(t, now.Add(2*time.Minute), derefTime(operation.LeaseExpiresAt))
}

func TestCoordinatorLostOwnerCannotComplete(t *testing.T) {
	coordinator, clock, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	clock.Advance(2 * time.Minute)

	err = coordinator.Complete(ctx, "operation-1", "worker-1", SafeOperationResult{
		Succeeded: true,
		Revision:  7,
		Digest:    validOperationDigest,
	})
	requireBizError(t, err, apperr.CodeDeliveryReconciliationRequired, "delivery.execution.reconciliation_required")
	operation, err := coordinator.Inspect(ctx, "operation-1")
	require.NoError(t, err)
	require.Equal(t, models.AppsReleaseOperationReconciling, operation.State)
	require.Nil(t, operation.ResultRevision)
	require.Nil(t, operation.ResultDigest)
}

func TestCoordinatorCompleteStoresOnlySafeResult(t *testing.T) {
	coordinator, _, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	require.NoError(t, coordinator.Complete(ctx, "operation-1", "worker-1", SafeOperationResult{
		Succeeded: true,
		Revision:  7,
		Digest:    validOperationDigest,
	}))

	operation, err := coordinator.Inspect(ctx, "operation-1")
	require.NoError(t, err)
	require.Equal(t, models.AppsReleaseOperationSucceeded, operation.State)
	require.EqualValues(t, 7, derefInt64(operation.ResultRevision))
	require.Equal(t, validOperationDigest, derefString(operation.ResultDigest))
	require.NotNil(t, operation.FinishedAt)

	replay, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-2", time.Minute)
	require.NoError(t, err)
	require.False(t, replay.Acquired)
	require.True(t, replay.Replayed)
	require.Equal(t, models.AppsReleaseOperationSucceeded, replay.Operation.State)
}

func TestCoordinatorExpiredLeaseAssignsReconciliationWithoutMutation(t *testing.T) {
	coordinator, clock, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	clock.Advance(2 * time.Minute)

	result, err := coordinator.Acquire(ctx, 41, "operation-2", "upgrade", "worker-2", time.Minute)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.False(t, result.Replayed)
	require.True(t, result.NeedsReconciliation)
	require.Equal(t, "operation-1", result.Operation.OperationID)
	require.Equal(t, models.AppsReleaseOperationReconciling, result.Operation.State)
	require.Equal(t, "worker-2", derefString(result.Operation.LeaseOwner))
	require.Equal(t, clock.Now().Add(time.Minute), derefTime(result.Operation.LeaseExpiresAt))

	clock.Advance(30 * time.Second)
	replay, err := coordinator.Acquire(ctx, 41, "operation-2", "upgrade", "worker-2", 2*time.Minute)
	require.NoError(t, err)
	require.False(t, replay.Acquired)
	require.True(t, replay.NeedsReconciliation)
	require.Equal(t, "operation-1", replay.Operation.OperationID)
	require.Equal(t, "worker-2", derefString(replay.Operation.LeaseOwner))
	require.Equal(t, clock.Now().Add(2*time.Minute), derefTime(replay.Operation.LeaseExpiresAt))
}

func TestCoordinatorRenewExtendsReconciliationLease(t *testing.T) {
	coordinator, clock, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	clock.Advance(2 * time.Minute)
	result, err := coordinator.Acquire(ctx, 41, "operation-2", "upgrade", "reconciler-1", time.Minute)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.True(t, result.NeedsReconciliation)

	until := clock.Now().Add(3 * time.Minute)
	require.NoError(t, coordinator.Renew(ctx, "operation-1", "reconciler-1", until))
	operation, err := coordinator.Inspect(ctx, "operation-1")
	require.NoError(t, err)
	require.Equal(t, models.AppsReleaseOperationReconciling, operation.State)
	require.Equal(t, until, derefTime(operation.LeaseExpiresAt))
	requireBizError(t,
		coordinator.Renew(ctx, "operation-1", "other-reconciler", until.Add(time.Minute)),
		apperr.CodeDeliveryOperationBusy,
		"delivery.execution.operation_busy",
	)
}

func TestCoordinatorRenewNonOwnerCannotClaimExpiredMutation(t *testing.T) {
	coordinator, clock, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	clock.Advance(2 * time.Minute)

	err = coordinator.Renew(ctx, "operation-1", "worker-2", clock.Now().Add(time.Minute))
	requireBizError(t, err, apperr.CodeDeliveryOperationBusy, "delivery.execution.operation_busy")
	operation, inspectErr := coordinator.Inspect(ctx, "operation-1")
	require.NoError(t, inspectErr)
	require.Equal(t, models.AppsReleaseOperationActive, operation.State)
	require.Equal(t, "worker-1", derefString(operation.LeaseOwner))
}

func TestCoordinatorExpiredReconciliationLeaseAllowsTakeover(t *testing.T) {
	coordinator, clock, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	clock.Advance(2 * time.Minute)
	firstClaim, err := coordinator.Acquire(ctx, 41, "operation-2", "upgrade", "reconciler-1", time.Minute)
	require.NoError(t, err)
	require.False(t, firstClaim.Acquired)
	require.True(t, firstClaim.NeedsReconciliation)

	clock.Advance(2 * time.Minute)
	takeover, err := coordinator.Acquire(ctx, 41, "operation-3", "upgrade", "reconciler-2", 2*time.Minute)
	require.NoError(t, err)
	require.False(t, takeover.Acquired, "reconciliation takeover must never authorize mutation")
	require.True(t, takeover.NeedsReconciliation)
	require.Equal(t, "operation-1", takeover.Operation.OperationID)
	require.Equal(t, "reconciler-2", derefString(takeover.Operation.LeaseOwner))
	require.Equal(t, clock.Now().Add(2*time.Minute), derefTime(takeover.Operation.LeaseExpiresAt))

	requireBizError(t,
		coordinator.Renew(ctx, "operation-1", "reconciler-1", clock.Now().Add(3*time.Minute)),
		apperr.CodeDeliveryOperationBusy,
		"delivery.execution.operation_busy",
	)
	requireBizError(t,
		coordinator.Complete(ctx, "operation-1", "reconciler-1", SafeOperationResult{Succeeded: false}),
		apperr.CodeDeliveryOperationBusy,
		"delivery.execution.operation_busy",
	)
	require.NoError(t, coordinator.Complete(ctx, "operation-1", "reconciler-2", SafeOperationResult{Succeeded: false}))
}

func TestCoordinatorNonExpiredMutationCannotBeTakenForReconciliation(t *testing.T) {
	coordinator, _, now := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", 5*time.Minute)
	require.NoError(t, err)
	_, err = coordinator.Acquire(ctx, 41, "operation-2", "upgrade", "worker-2", time.Minute)
	requireBizError(t, err, apperr.CodeDeliveryOperationBusy, "delivery.execution.operation_busy")

	operation, err := coordinator.Inspect(ctx, "operation-1")
	require.NoError(t, err)
	require.Equal(t, models.AppsReleaseOperationActive, operation.State)
	require.Equal(t, "worker-1", derefString(operation.LeaseOwner))
	require.Equal(t, now.Add(5*time.Minute), derefTime(operation.LeaseExpiresAt))
}

func TestCoordinatorReconciliationRejectsOtherOwner(t *testing.T) {
	coordinator, clock, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	clock.Advance(2 * time.Minute)
	result, err := coordinator.Acquire(ctx, 41, "operation-2", "upgrade", "worker-2", time.Minute)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.True(t, result.NeedsReconciliation)

	_, err = coordinator.Acquire(ctx, 41, "operation-3", "upgrade", "worker-3", time.Minute)
	requireBizError(t, err, apperr.CodeDeliveryOperationBusy, "delivery.execution.operation_busy")
	requireBizError(t,
		coordinator.Complete(ctx, "operation-1", "worker-3", SafeOperationResult{Succeeded: false}),
		apperr.CodeDeliveryOperationBusy,
		"delivery.execution.operation_busy",
	)
}

func TestCoordinatorReconciliationOwnerCanCompleteAndReleaseApplication(t *testing.T) {
	coordinator, clock, _ := newCoordinatorHarness()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	clock.Advance(2 * time.Minute)
	result, err := coordinator.Acquire(ctx, 41, "operation-2", "upgrade", "reconciler-1", time.Minute)
	require.NoError(t, err)
	require.False(t, result.Acquired, "reconciliation ownership must never authorize mutation")
	require.True(t, result.NeedsReconciliation)

	require.NoError(t, coordinator.Complete(ctx, "operation-1", "reconciler-1", SafeOperationResult{
		Succeeded: true,
		Revision:  9,
		Digest:    validOperationDigest,
	}))
	operation, err := coordinator.Inspect(ctx, "operation-1")
	require.NoError(t, err)
	require.Equal(t, models.AppsReleaseOperationSucceeded, operation.State)

	next, err := coordinator.Acquire(ctx, 41, "operation-2", "upgrade", "worker-2", time.Minute)
	require.NoError(t, err)
	require.True(t, next.Acquired)
	require.False(t, next.NeedsReconciliation)
}

func TestCoordinatorMapsUnexpectedStoreErrorsToSafeUnavailable(t *testing.T) {
	raw := errors.New("postgres leaked upstream-secret-value")
	coordinator := newCoordinator(&failingOperationStore{err: raw})
	ctx := context.Background()
	calls := map[string]func() error{
		"acquire": func() error {
			_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
			return err
		},
		"inspect": func() error {
			_, err := coordinator.Inspect(ctx, "operation-1")
			return err
		},
		"renew": func() error {
			return coordinator.Renew(ctx, "operation-1", "worker-1", time.Now().Add(time.Minute))
		},
		"complete": func() error {
			return coordinator.Complete(ctx, "operation-1", "worker-1", SafeOperationResult{Succeeded: false})
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			requireBizError(t, err, apperr.CodeDeliveryExecutionUnavailable, "delivery.execution.unavailable")
			business, ok := apperr.AsBiz(err)
			require.True(t, ok)
			require.Equal(t, "release operation is unavailable", business.Message)
			require.NotContains(t, business.Message, raw.Error())
			require.NotContains(t, business.Message, "upstream-secret-value")
			require.ErrorIs(t, err, raw)
		})
	}
}

func TestCoordinatorSamplesDatabaseTimeAfterLockBeforeDecision(t *testing.T) {
	tests := []struct {
		name string
		seed func(*Coordinator) error
		call func(*Coordinator, time.Time) error
		want []string
	}{
		{
			name: "acquire",
			seed: func(*Coordinator) error { return nil },
			call: func(coordinator *Coordinator, _ time.Time) error {
				_, err := coordinator.Acquire(context.Background(), 41, "operation-1", "upgrade", "worker-1", time.Minute)
				return err
			},
			want: []string{"lock", "db-now", "find"},
		},
		{
			name: "renew",
			seed: func(coordinator *Coordinator) error {
				_, err := coordinator.Acquire(context.Background(), 41, "operation-1", "upgrade", "worker-1", time.Minute)
				return err
			},
			call: func(coordinator *Coordinator, base time.Time) error {
				return coordinator.Renew(context.Background(), "operation-1", "worker-1", base.Add(2*time.Minute))
			},
			want: []string{"inspect", "lock", "db-now", "find"},
		},
		{
			name: "complete",
			seed: func(coordinator *Coordinator) error {
				_, err := coordinator.Acquire(context.Background(), 41, "operation-1", "upgrade", "worker-1", time.Minute)
				return err
			},
			call: func(coordinator *Coordinator, _ time.Time) error {
				return coordinator.Complete(context.Background(), "operation-1", "worker-1", SafeOperationResult{Succeeded: false})
			},
			want: []string{"inspect", "lock", "db-now", "find"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var events []string
			coordinator, clock, store, base := newCoordinatorHarnessWithStore()
			require.NoError(t, test.seed(coordinator))
			store.events = &events
			clock.onNow = func() { events = append(events, "db-now") }
			require.NoError(t, test.call(coordinator, base))
			require.Equal(t, test.want, events)
		})
	}
}

func TestCoordinatorUsesDatabaseTimeAfterLockWait(t *testing.T) {
	coordinator, clock, store, base := newCoordinatorHarnessWithStore()
	ctx := context.Background()

	_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
	require.NoError(t, err)
	store.onLocked = func() { clock.Advance(2 * time.Minute) }

	result, err := coordinator.Acquire(ctx, 41, "operation-2", "upgrade", "reconciler-1", time.Minute)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.True(t, result.NeedsReconciliation)
	require.Equal(t, models.AppsReleaseOperationReconciling, result.Operation.State)
	require.Equal(t, base.Add(3*time.Minute), derefTime(result.Operation.LeaseExpiresAt))
}

func TestCoordinatorValidatesSafeCompletionResult(t *testing.T) {
	tests := []struct {
		name   string
		result SafeOperationResult
	}{
		{name: "success requires revision", result: SafeOperationResult{Succeeded: true, Digest: validOperationDigest}},
		{name: "success requires digest", result: SafeOperationResult{Succeeded: true, Revision: 1}},
		{name: "success rejects uppercase digest", result: SafeOperationResult{Succeeded: true, Revision: 1, Digest: "sha256:ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef0123456789"}},
		{name: "success rejects short digest", result: SafeOperationResult{Succeeded: true, Revision: 1, Digest: "sha256:abcd"}},
		{name: "failure rejects revision", result: SafeOperationResult{Succeeded: false, Revision: 1}},
		{name: "failure rejects digest", result: SafeOperationResult{Succeeded: false, Digest: validOperationDigest}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinator, _, _ := newCoordinatorHarness()
			ctx := context.Background()
			_, err := coordinator.Acquire(ctx, 41, "operation-1", "upgrade", "worker-1", time.Minute)
			require.NoError(t, err)

			err = coordinator.Complete(ctx, "operation-1", "worker-1", test.result)
			requireBizError(t, err, apperr.CodeDeliveryExecutionUnavailable, "delivery.execution.unavailable")
			operation, inspectErr := coordinator.Inspect(ctx, "operation-1")
			require.NoError(t, inspectErr)
			require.Equal(t, models.AppsReleaseOperationActive, operation.State)
			require.Nil(t, operation.ResultRevision)
			require.Nil(t, operation.ResultDigest)
		})
	}
}

func newCoordinatorHarness() (*Coordinator, *fakeClock, time.Time) {
	coordinator, clock, _, now := newCoordinatorHarnessWithStore()
	return coordinator, clock, now
}

func newCoordinatorHarnessWithStore() (*Coordinator, *fakeClock, *memoryOperationStore, time.Time) {
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	store := newMemoryOperationStore()
	store.now = clock.Now
	return newCoordinator(store), clock, store, now
}

type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	onNow func()
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.onNow != nil {
		c.onNow()
	}
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type memoryOperationStore struct {
	mu            sync.Mutex
	nextID        uint64
	byOperation   map[string]*models.AppsReleaseOperation
	byApplication map[uint64][]*models.AppsReleaseOperation
	now           func() time.Time
	events        *[]string
	onLocked      func()
}

func newMemoryOperationStore() *memoryOperationStore {
	return &memoryOperationStore{
		nextID:        1,
		byOperation:   make(map[string]*models.AppsReleaseOperation),
		byApplication: make(map[uint64][]*models.AppsReleaseOperation),
		now:           time.Now,
	}
}

func (s *memoryOperationStore) WithApplicationLock(_ context.Context, _ uint64, fn func(operationTransaction) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.record("lock")
	if s.onLocked != nil {
		onLocked := s.onLocked
		s.onLocked = nil
		onLocked()
	}
	return fn((*memoryOperationTransaction)(s))
}

func (s *memoryOperationStore) Inspect(_ context.Context, operationID string) (*models.AppsReleaseOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.record("inspect")
	row := s.byOperation[operationID]
	if row == nil {
		return nil, errOperationNotFound
	}
	return cloneOperation(row), nil
}

type memoryOperationTransaction memoryOperationStore

func (s *memoryOperationStore) record(event string) {
	if s.events != nil {
		*s.events = append(*s.events, event)
	}
}

func (tx *memoryOperationTransaction) Now(context.Context) (time.Time, error) {
	return tx.now().UTC(), nil
}

type failingOperationStore struct{ err error }

func (s *failingOperationStore) WithApplicationLock(context.Context, uint64, func(operationTransaction) error) error {
	return s.err
}

func (s *failingOperationStore) Inspect(context.Context, string) (*models.AppsReleaseOperation, error) {
	return nil, s.err
}

func (tx *memoryOperationTransaction) FindByOperationID(_ context.Context, operationID string) (*models.AppsReleaseOperation, error) {
	(*memoryOperationStore)(tx).record("find")
	row := tx.byOperation[operationID]
	if row == nil {
		return nil, errOperationNotFound
	}
	return cloneOperation(row), nil
}

func (tx *memoryOperationTransaction) FindBlockingByApplication(_ context.Context, applicationID uint64) (*models.AppsReleaseOperation, error) {
	for _, row := range tx.byApplication[applicationID] {
		if row.State == models.AppsReleaseOperationActive || row.State == models.AppsReleaseOperationReconciling {
			return cloneOperation(row), nil
		}
	}
	return nil, errOperationNotFound
}

func (tx *memoryOperationTransaction) Create(_ context.Context, row *models.AppsReleaseOperation) error {
	if _, exists := tx.byOperation[row.OperationID]; exists {
		return errOperationConflict
	}
	row.ID = tx.nextID
	tx.nextID++
	stored := cloneOperation(row)
	tx.byOperation[row.OperationID] = stored
	tx.byApplication[row.ApplicationID] = append(tx.byApplication[row.ApplicationID], stored)
	return nil
}

func (tx *memoryOperationTransaction) Save(_ context.Context, row *models.AppsReleaseOperation) error {
	stored := tx.byOperation[row.OperationID]
	if stored == nil {
		return errOperationNotFound
	}
	*stored = *cloneOperation(row)
	return nil
}

func cloneOperation(row *models.AppsReleaseOperation) *models.AppsReleaseOperation {
	if row == nil {
		return nil
	}
	clone := *row
	if row.LeaseOwner != nil {
		value := *row.LeaseOwner
		clone.LeaseOwner = &value
	}
	if row.LeaseExpiresAt != nil {
		value := *row.LeaseExpiresAt
		clone.LeaseExpiresAt = &value
	}
	if row.ResultRevision != nil {
		value := *row.ResultRevision
		clone.ResultRevision = &value
	}
	if row.ResultDigest != nil {
		value := *row.ResultDigest
		clone.ResultDigest = &value
	}
	if row.FinishedAt != nil {
		value := *row.FinishedAt
		clone.FinishedAt = &value
	}
	return &clone
}

func requireBizError(t *testing.T, err error, code apperr.Code, key string) {
	t.Helper()
	require.Error(t, err)
	got, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, code, got.Code)
	require.Equal(t, key, got.MessageKey)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func derefInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
