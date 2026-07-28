package event

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestPrunerDeletesOnlyEventsStrictlyOlderThanCutoff(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -180)
	store := &memoryPruneStore{events: []pruneEvent{
		{id: 1, occurredAt: cutoff.Add(-time.Nanosecond)},
		{id: 2, occurredAt: cutoff},
		{id: 3, occurredAt: cutoff.Add(time.Nanosecond)},
	}, runSummaries: 1, stageSummaries: 1, approvalSummaries: 1}
	pruner := newPruner(store, discardLogger(), fixedClock{now: now}, 180, 0)

	deleted, err := pruner.Prune(context.Background())
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if deleted != 1 || store.hasEvent(1) || !store.hasEvent(2) || !store.hasEvent(3) {
		t.Fatalf("Prune() deleted=%d events=%v", deleted, store.events)
	}
	if store.runSummaries != 1 || store.stageSummaries != 1 || store.approvalSummaries != 1 {
		t.Fatal("Prune() changed retained summaries")
	}
}

func TestPrunerBoundsBatchesAndIsIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	store := &memoryPruneStore{}
	for id := uint64(1); id <= 501; id++ {
		store.events = append(store.events, pruneEvent{id: id, occurredAt: now.AddDate(-1, 0, 0)})
	}
	pruner := newPruner(store, discardLogger(), fixedClock{now: now}, 180, 0)

	deleted, err := pruner.Prune(context.Background())
	if err != nil || deleted != 501 {
		t.Fatalf("first Prune() = (%d, %v), want (501, nil)", deleted, err)
	}
	if len(store.limits) != 2 || store.limits[0] != maxPruneBatchSize || store.limits[1] != maxPruneBatchSize {
		t.Fatalf("batch limits = %v", store.limits)
	}
	if !store.allCallsBounded {
		t.Fatal("prune batch did not receive a bounded context")
	}
	deleted, err = pruner.Prune(context.Background())
	if err != nil || deleted != 0 {
		t.Fatalf("second Prune() = (%d, %v), want (0, nil)", deleted, err)
	}
}

func TestPrunerLogsOnlySafeFailureCategory(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	store := &memoryPruneStore{err: errors.New("postgres password=do-not-log")}
	pruner := newPruner(store, logger, fixedClock{now: time.Now()}, 180, 0)

	pruner.runOnce(context.Background())
	logged := output.String()
	if !bytes.Contains(output.Bytes(), []byte(`"category":"database"`)) {
		t.Fatalf("safe failure category missing from log: %s", logged)
	}
	if bytes.Contains(output.Bytes(), []byte("do-not-log")) || bytes.Contains(output.Bytes(), []byte("password")) {
		t.Fatalf("raw store error leaked to log: %s", logged)
	}
}

func TestPrunerCancellationStopsBetweenBatches(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	store := &memoryPruneStore{afterBatch: cancel}
	for id := uint64(1); id <= 501; id++ {
		store.events = append(store.events, pruneEvent{id: id, occurredAt: now.AddDate(-1, 0, 0)})
	}
	pruner := newPruner(store, discardLogger(), fixedClock{now: now}, 180, 0)

	deleted, err := pruner.Prune(ctx)
	if !errors.Is(err, context.Canceled) || deleted != maxPruneBatchSize {
		t.Fatalf("Prune() = (%d, %v), want (%d, canceled)", deleted, err, maxPruneBatchSize)
	}
	if len(store.limits) != 1 {
		t.Fatalf("batch calls = %d, want 1", len(store.limits))
	}
}

func TestPrunerDoesNotCountRolledBackBatch(t *testing.T) {
	store := &memoryPruneStore{deletedOnError: maxPruneBatchSize, err: errors.New("commit failed")}
	pruner := newPruner(store, discardLogger(), fixedClock{now: time.Now()}, 180, 0)
	deleted, err := pruner.Prune(context.Background())
	if err == nil || deleted != 0 {
		t.Fatalf("Prune() = (%d, %v), want (0, error)", deleted, err)
	}
}

func TestPrunerRunWaitsForStartupDelayThenRunsDaily(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	clock := newManualClock(now)
	store := &memoryPruneStore{}
	pruner := newPruner(store, discardLogger(), clock, 180, 15*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); pruner.Run(ctx) }()

	clock.waitForAfter(t)
	if store.calls() != 0 {
		t.Fatal("pruner ran before startup delay")
	}
	clock.fireAfter()
	store.waitForCalls(t, 1)
	clock.waitForTicker(t)
	clock.tick()
	store.waitForCalls(t, 2)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}
	if !clock.tickerStopped() {
		t.Fatal("Run() did not stop its ticker")
	}
}

func TestPrunerRunStopsStartupTimerOnEarlyShutdown(t *testing.T) {
	clock := newManualClock(time.Now())
	store := &memoryPruneStore{}
	pruner := newPruner(store, discardLogger(), clock, 180, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); pruner.Run(ctx) }()
	clock.waitForTimer(t)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop during startup delay")
	}
	if !clock.timerStopped() || store.calls() != 0 {
		t.Fatalf("timer stopped=%v store calls=%d", clock.timerStopped(), store.calls())
	}
}

type pruneEvent struct {
	id         uint64
	occurredAt time.Time
}

type memoryPruneStore struct {
	mu                sync.Mutex
	events            []pruneEvent
	limits            []int
	afterBatch        func()
	runSummaries      int
	stageSummaries    int
	approvalSummaries int
	err               error
	deletedOnError    int
	allCallsBounded   bool
}

func (s *memoryPruneStore) DeleteBeforeBatch(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, bounded := ctx.Deadline()
	if len(s.limits) == 0 {
		s.allCallsBounded = bounded
	} else {
		s.allCallsBounded = s.allCallsBounded && bounded
	}
	s.limits = append(s.limits, limit)
	if s.err != nil {
		return s.deletedOnError, s.err
	}
	deleted := 0
	kept := s.events[:0]
	for _, event := range s.events {
		if event.occurredAt.Before(cutoff) && deleted < limit {
			deleted++
			continue
		}
		kept = append(kept, event)
	}
	s.events = kept
	if s.afterBatch != nil {
		s.afterBatch()
		s.afterBatch = nil
	}
	return deleted, nil
}

func (s *memoryPruneStore) hasEvent(id uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range s.events {
		if event.id == id {
			return true
		}
	}
	return false
}

func (s *memoryPruneStore) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.limits)
}

func (s *memoryPruneStore) waitForCalls(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.calls() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("store calls = %d, want at least %d", s.calls(), want)
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time                     { return c.now }
func (fixedClock) NewTimer(time.Duration) prunerTimer   { return &manualTimer{ch: make(chan time.Time)} }
func (fixedClock) NewTicker(time.Duration) prunerTicker { panic("unexpected ticker") }

type manualClock struct {
	now       time.Time
	timer     *manualTimer
	timerSeen chan struct{}
	ticker    *manualTicker
	tickSeen  chan struct{}
}

func newManualClock(now time.Time) *manualClock {
	return &manualClock{now: now, timerSeen: make(chan struct{}), tickSeen: make(chan struct{})}
}

func (c *manualClock) Now() time.Time { return c.now }
func (c *manualClock) NewTimer(time.Duration) prunerTimer {
	c.timer = &manualTimer{ch: make(chan time.Time, 1)}
	close(c.timerSeen)
	return c.timer
}
func (c *manualClock) NewTicker(time.Duration) prunerTicker {
	c.ticker = &manualTicker{ch: make(chan time.Time, 1)}
	close(c.tickSeen)
	return c.ticker
}
func (c *manualClock) waitForTimer(t *testing.T) {
	t.Helper()
	select {
	case <-c.timerSeen:
	case <-time.After(time.Second):
		t.Fatal("startup timer was not created")
	}
}
func (c *manualClock) waitForAfter(t *testing.T) { c.waitForTimer(t) }
func (c *manualClock) fireAfter()                { c.timer.ch <- c.now }
func (c *manualClock) waitForTicker(t *testing.T) {
	t.Helper()
	select {
	case <-c.tickSeen:
	case <-time.After(time.Second):
		t.Fatal("daily ticker was not created")
	}
}
func (c *manualClock) tick()               { c.ticker.ch <- c.now.Add(24 * time.Hour) }
func (c *manualClock) tickerStopped() bool { return c.ticker != nil && c.ticker.stopped }
func (c *manualClock) timerStopped() bool  { return c.timer != nil && c.timer.stopped }

type manualTimer struct {
	ch      chan time.Time
	stopped bool
}

func (t *manualTimer) C() <-chan time.Time { return t.ch }
func (t *manualTimer) Stop() bool          { t.stopped = true; return true }

type manualTicker struct {
	ch      chan time.Time
	stopped bool
}

func (t *manualTicker) C() <-chan time.Time { return t.ch }
func (t *manualTicker) Stop()               { t.stopped = true }

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
