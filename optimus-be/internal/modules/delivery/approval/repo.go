package approval

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/delivery/errs"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// PendingRow is the persistence projection for an actor's actionable queue.
type PendingRow struct {
	ApprovalID      uint64
	RunID           uint64
	RunStageID      uint64
	ProjectID       uint64
	ProjectName     string
	EnvironmentKey  string
	EnvironmentName string
	StageOrder      int
	ChartName       string
	ChartVersion    string
	ChartDigest     string
	InitiatorUserID uint64
	RequestedAt     time.Time
}

func (r *Repo) Transaction(ctx context.Context, fn func(repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repo{db: tx})
	})
}

func (r *Repo) ListPending(ctx context.Context, actor uint64) ([]PendingRow, error) {
	var rows []PendingRow
	err := r.db.WithContext(ctx).Table("delivery_approvals AS a").
		Select(`a.id AS approval_id, a.run_id, a.run_stage_id, r.project_id,
			p.name AS project_name, s.environment_key, s.environment_name, s.stage_order,
			r.chart_name, r.chart_version, r.chart_digest, r.initiator_user_id, a.requested_at`).
		Joins("JOIN delivery_runs AS r ON r.id = a.run_id").
		Joins("JOIN delivery_run_stages AS s ON s.id = a.run_stage_id AND s.run_id = a.run_id").
		Joins("JOIN delivery_projects AS p ON p.id = r.project_id AND p.deleted_at IS NULL").
		Where("a.decision = ?", models.DeliveryApprovalPending).
		Where("r.state = ? AND s.state = ?", models.DeliveryRunWaitingApproval, models.DeliveryStageWaitingApproval).
		Where("r.initiator_user_id <> ?", actor).
		Order("a.requested_at ASC, a.id ASC").Scan(&rows).Error
	return rows, err
}

func (r *Repo) LockForDecision(ctx context.Context, stageID uint64) (*models.DeliveryApproval, *models.DeliveryRunStage, *models.DeliveryRun, error) {
	var approval models.DeliveryApproval
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("run_stage_id = ?", stageID).First(&approval).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, approvalNotFoundError()
	}
	if err != nil {
		return nil, nil, nil, err
	}
	var stage models.DeliveryRunStage
	err = r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND run_id = ?", stageID, approval.RunID).First(&stage).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, approvalNotFoundError()
	}
	if err != nil {
		return nil, nil, nil, err
	}
	var run models.DeliveryRun
	err = r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, approval.RunID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, approvalNotFoundError()
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return &approval, &stage, &run, nil
}

func (r *Repo) DecideApproval(ctx context.Context, id, actor uint64, decision models.DeliveryApprovalDecision, comment string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&models.DeliveryApproval{}).
		Where("id = ? AND decision = ?", id, models.DeliveryApprovalPending).
		Updates(map[string]any{"decision": decision, "decided_by_user_id": actor, "comment": comment, "decided_at": now, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return approvalAlreadyDecidedError()
	}
	return nil
}

func (r *Repo) TransitionStage(ctx context.Context, id uint64, from, to models.DeliveryStageState, now time.Time, finishedAt *time.Time) error {
	fields := map[string]any{"state": to, "updated_at": now}
	if finishedAt != nil {
		fields["finished_at"] = *finishedAt
	}
	result := r.db.WithContext(ctx).Model(&models.DeliveryRunStage{}).Where("id = ? AND state = ?", id, from).Updates(fields)
	return mapTransitionResult(result)
}

func (r *Repo) TransitionRun(ctx context.Context, id uint64, from, to models.DeliveryRunState, now time.Time, finishedAt *time.Time) error {
	fields := map[string]any{"state": to, "updated_at": now}
	if finishedAt != nil {
		fields["finished_at"] = *finishedAt
	}
	result := r.db.WithContext(ctx).Model(&models.DeliveryRun{}).Where("id = ? AND state = ?", id, from).Updates(fields)
	return mapTransitionResult(result)
}

func (r *Repo) AppendEvent(ctx context.Context, event *models.DeliveryRunEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func mapTransitionResult(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "delivery approval state changed concurrently")
	}
	return nil
}

func approvalNotFoundError() error {
	return apperr.New(errs.CodeApprovalNotFound, errs.KeyApprovalNotFound, "delivery approval not found")
}
