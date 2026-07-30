package event

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"optimus-be/internal/models"
)

const (
	maxPruneBatchSize = 500
	pruneBatchTimeout = 5 * time.Second
	pruneInterval     = 24 * time.Hour
)

type pruneStore interface {
	DeleteBeforeBatch(context.Context, time.Time, int) (int, error)
}

type prunerClock interface {
	Now() time.Time
	NewTimer(time.Duration) prunerTimer
	NewTicker(time.Duration) prunerTicker
}

type prunerTimer interface {
	C() <-chan time.Time
	Stop() bool
}

type prunerTicker interface {
	C() <-chan time.Time
	Stop()
}

type Pruner struct {
	store         pruneStore
	logger        *slog.Logger
	clock         prunerClock
	retentionDays int
	startupDelay  time.Duration
}

func NewPruner(db *gorm.DB, logger *slog.Logger, retentionDays int, startupDelay time.Duration) *Pruner {
	return newPruner(&gormPruneStore{db: db}, logger, realPrunerClock{}, retentionDays, startupDelay)
}

func newPruner(store pruneStore, logger *slog.Logger, clock prunerClock, retentionDays int, startupDelay time.Duration) *Pruner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pruner{store: store, logger: logger, clock: clock, retentionDays: retentionDays, startupDelay: startupDelay}
}

// Prune deletes detailed events in bounded, independently committed batches.
func (p *Pruner) Prune(ctx context.Context) (int, error) {
	if p == nil || p.store == nil || p.clock == nil || p.retentionDays <= 0 {
		return 0, errors.New("delivery event pruner is not configured")
	}
	cutoff := p.clock.Now().UTC().AddDate(0, 0, -p.retentionDays)
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		batchCtx, cancel := context.WithTimeout(ctx, pruneBatchTimeout)
		deleted, err := p.store.DeleteBeforeBatch(batchCtx, cutoff, maxPruneBatchSize)
		cancel()
		if err != nil {
			return total, err
		}
		total += deleted
		if deleted < maxPruneBatchSize {
			return total, nil
		}
	}
}

// Run waits for the shared startup delay, prunes once, then repeats daily.
func (p *Pruner) Run(ctx context.Context) {
	timer := p.clock.NewTimer(p.startupDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C():
	}
	p.runOnce(ctx)

	ticker := p.clock.NewTicker(pruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			p.runOnce(ctx)
		}
	}
}

func (p *Pruner) runOnce(ctx context.Context) {
	deleted, err := p.Prune(ctx)
	if err != nil {
		p.logger.Error("delivery event pruning failed", "category", pruneFailureCategory(err))
		return
	}
	p.logger.Info("delivery event pruning completed", "deleted", deleted)
}

func pruneFailureCategory(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "database"
	}
}

type gormPruneStore struct{ db *gorm.DB }

func (s *gormPruneStore) DeleteBeforeBatch(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("delivery event prune store is not configured")
	}
	deleted := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []uint64
		if err := tx.Model(&models.DeliveryRunEvent{}).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("occurred_at < ?", cutoff).
			Order("id ASC").
			Limit(limit).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		result := tx.Where("id IN ?", ids).Delete(&models.DeliveryRunEvent{})
		if result.Error != nil {
			return result.Error
		}
		deleted = int(result.RowsAffected)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, err
}

type realPrunerClock struct{}

func (realPrunerClock) Now() time.Time { return time.Now() }
func (realPrunerClock) NewTimer(delay time.Duration) prunerTimer {
	return realPrunerTimer{Timer: time.NewTimer(delay)}
}
func (realPrunerClock) NewTicker(interval time.Duration) prunerTicker {
	return realPrunerTicker{Ticker: time.NewTicker(interval)}
}

type realPrunerTicker struct{ *time.Ticker }

func (t realPrunerTicker) C() <-chan time.Time { return t.Ticker.C }

type realPrunerTimer struct{ *time.Timer }

func (t realPrunerTimer) C() <-chan time.Time { return t.Timer.C }
