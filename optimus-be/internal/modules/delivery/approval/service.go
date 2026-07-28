package approval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/delivery/errs"
	deliveryrun "optimus-be/internal/modules/delivery/run"
)

const decisionPermission = "delivery:approval:decide"

type repository interface {
	ListPending(context.Context, uint64) ([]PendingRow, error)
	Transaction(context.Context, func(repository, *gorm.DB) error) error
	FindDecisionIdentity(context.Context, uint64) (*DecisionIdentity, error)
	LockRun(context.Context, uint64) (*models.DeliveryRun, error)
	LockStage(context.Context, uint64) (*models.DeliveryRunStage, error)
	LockApproval(context.Context, uint64) (*models.DeliveryApproval, error)
	DecideApproval(context.Context, uint64, uint64, models.DeliveryApprovalDecision, string, time.Time) error
	TransitionStage(context.Context, uint64, models.DeliveryStageState, models.DeliveryStageState, time.Time, *time.Time) error
	TransitionRun(context.Context, uint64, models.DeliveryRunState, models.DeliveryRunState, time.Time, *time.Time) error
	AppendEvent(context.Context, *models.DeliveryRunEvent) error
}

type PermissionChecker interface {
	Has(context.Context, uint64, string) (bool, error)
}

type Recorder interface {
	Record(context.Context, audit.Event) error
}

type Service struct {
	repo        repository
	permissions PermissionChecker
	auditTx     func(*gorm.DB) Recorder
	now         func() time.Time
}

func NewService(repo repository, permissions PermissionChecker, recorder Recorder) *Service {
	auditTx := func(*gorm.DB) Recorder { return recorder }
	if concrete, ok := recorder.(*audit.Recorder); ok {
		auditTx = func(tx *gorm.DB) Recorder { return concrete.WithTx(tx) }
	}
	return &Service{repo: repo, permissions: permissions, auditTx: auditTx, now: time.Now}
}

func (s *Service) Approve(ctx context.Context, actor uint64, ip, userAgent string, stageID uint64, req DecisionRequest) (*Decision, error) {
	return s.decide(ctx, actor, ip, userAgent, stageID, models.DeliveryApprovalApproved, req)
}

func (s *Service) Reject(ctx context.Context, actor uint64, ip, userAgent string, stageID uint64, req DecisionRequest) (*Decision, error) {
	return s.decide(ctx, actor, ip, userAgent, stageID, models.DeliveryApprovalRejected, req)
}

func (s *Service) decide(ctx context.Context, actor uint64, ip, userAgent string, stageID uint64, wanted models.DeliveryApprovalDecision, req DecisionRequest) (*Decision, error) {
	comment, err := validateComment(req.Comment)
	if err != nil {
		return nil, err
	}
	var result *Decision
	err = s.repo.Transaction(ctx, func(tx repository, dbtx *gorm.DB) error {
		identity, err := tx.FindDecisionIdentity(ctx, stageID)
		if err != nil {
			return err
		}
		runRow, err := tx.LockRun(ctx, identity.RunID)
		if err != nil {
			return err
		}
		stageRow, err := tx.LockStage(ctx, identity.RunStageID)
		if err != nil {
			return err
		}
		approvalRow, err := tx.LockApproval(ctx, identity.ApprovalID)
		if err != nil {
			return err
		}
		if !validLockedIdentity(identity, stageID, approvalRow, stageRow, runRow) {
			return approvalNotFoundError()
		}
		if err := s.requireDecisionPermission(ctx, actor); err != nil {
			return err
		}
		if approvalRow.Decision != models.DeliveryApprovalPending {
			if identicalDecision(approvalRow, actor, wanted, comment) {
				result = decisionFrom(approvalRow)
				return nil
			}
			if approvalRow.Decision != wanted {
				return approvalDecisionConflictError()
			}
			return approvalAlreadyDecidedError()
		}
		if runRow.InitiatorUserID == actor {
			return apperr.New(errs.CodeApprovalSelfApproval, errs.KeyApprovalSelfApproval, "run initiator cannot decide this approval")
		}
		if stageRow.State != models.DeliveryStageWaitingApproval || runRow.State != models.DeliveryRunWaitingApproval ||
			!deliveryrun.CanTransitionStage(stageRow.State, decisionStageState(wanted)) ||
			!deliveryrun.CanTransitionRun(runRow.State, decisionRunState(wanted)) {
			return apperr.New(errs.CodeRunInvalidState, errs.KeyRunInvalidState, "approval stage is not waiting for a decision")
		}

		now := s.now().UTC()
		if err := tx.DecideApproval(ctx, approvalRow.ID, actor, wanted, comment, now); err != nil {
			return err
		}
		finishedAt := (*time.Time)(nil)
		if wanted == models.DeliveryApprovalRejected {
			finishedAt = &now
		}
		stageNext, runNext := decisionStageState(wanted), decisionRunState(wanted)
		stageOld, runOld := stageRow.State, runRow.State
		if err := tx.TransitionStage(ctx, stageRow.ID, stageOld, stageNext, now, finishedAt); err != nil {
			return err
		}
		if err := tx.TransitionRun(ctx, runRow.ID, runOld, runNext, now, finishedAt); err != nil {
			return err
		}
		oldState, newState := string(stageOld), string(stageNext)
		stageIDCopy := stageRow.ID
		if err := tx.AppendEvent(ctx, &models.DeliveryRunEvent{
			RunID: runRow.ID, RunStageID: &stageIDCopy, EventType: "stage." + string(wanted),
			OldState: &oldState, NewState: &newState, ActorType: models.DeliveryEventActorUser,
			ActorID: &actor, OccurredAt: now, Metadata: datatypes.JSON([]byte(`{}`)),
		}); err != nil {
			return err
		}
		runOldState, runNewState := string(runOld), string(runNext)
		if err := tx.AppendEvent(ctx, &models.DeliveryRunEvent{
			RunID: runRow.ID, EventType: "run." + string(wanted), OldState: &runOldState,
			NewState: &runNewState, ActorType: models.DeliveryEventActorUser,
			ActorID: &actor, OccurredAt: now, Metadata: datatypes.JSON([]byte(`{}`)),
		}); err != nil {
			return err
		}
		approvalRow.Decision, approvalRow.Comment, approvalRow.DecidedByUserID = wanted, comment, &actor
		approvalRow.DecidedAt, approvalRow.UpdatedAt = &now, now
		stageRow.State, runRow.State = stageNext, runNext
		result = decisionFrom(approvalRow)
		if err := recordDecision(ctx, s.auditTx(dbtx), actor, ip, userAgent, result); err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "common.internal", "approval audit write failed")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validLockedIdentity(identity *DecisionIdentity, stageID uint64, approval *models.DeliveryApproval, stage *models.DeliveryRunStage, run *models.DeliveryRun) bool {
	return identity != nil && approval != nil && stage != nil && run != nil &&
		identity.ApprovalID != 0 && identity.RunID != 0 && identity.RunStageID == stageID &&
		run.ID == identity.RunID && stage.ID == identity.RunStageID && stage.RunID == identity.RunID &&
		approval.ID == identity.ApprovalID && approval.RunID == identity.RunID && approval.RunStageID == identity.RunStageID
}

func decisionStageState(decision models.DeliveryApprovalDecision) models.DeliveryStageState {
	if decision == models.DeliveryApprovalRejected {
		return models.DeliveryStageRejected
	}
	return models.DeliveryStageQueued
}

func decisionRunState(decision models.DeliveryApprovalDecision) models.DeliveryRunState {
	if decision == models.DeliveryApprovalRejected {
		return models.DeliveryRunRejected
	}
	return models.DeliveryRunQueued
}

func identicalDecision(row *models.DeliveryApproval, actor uint64, decision models.DeliveryApprovalDecision, comment string) bool {
	return row != nil && row.Decision == decision && row.DecidedByUserID != nil && *row.DecidedByUserID == actor && row.Comment == comment
}

func decisionFrom(row *models.DeliveryApproval) *Decision {
	if row == nil || row.DecidedByUserID == nil || row.DecidedAt == nil {
		return nil
	}
	return &Decision{ID: row.ID, RunID: row.RunID, RunStageID: row.RunStageID, Decision: row.Decision,
		DecidedByUserID: *row.DecidedByUserID, Comment: row.Comment, DecidedAt: *row.DecidedAt}
}

func recordDecision(ctx context.Context, recorder Recorder, actor uint64, ip, userAgent string, decision *Decision) error {
	if recorder == nil || decision == nil {
		return nil
	}
	sum := sha256.Sum256([]byte(decision.Comment))
	action := "approve"
	if decision.Decision == models.DeliveryApprovalRejected {
		action = "reject"
	}
	return recorder.Record(ctx, audit.Event{
		UserID: &actor, Action: "delivery.approval." + action, TargetType: "delivery.approval",
		TargetID: strconv.FormatUint(decision.ID, 10), IP: ip, UserAgent: userAgent,
		Payload: map[string]any{
			"decision": string(decision.Decision), "comment_present": decision.Comment != "",
			"comment_sha256": hex.EncodeToString(sum[:]),
		},
	})
}

func approvalAlreadyDecidedError() error {
	return apperr.New(errs.CodeApprovalAlreadyDecided, errs.KeyApprovalAlreadyDecided, "approval was already decided")
}

func approvalDecisionConflictError() error {
	return apperr.New(errs.CodeApprovalDecisionConflict, errs.KeyApprovalDecisionConflict, "approval decision conflicts with the existing decision")
}

func validateComment(comment string) (string, error) {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return "", apperr.New(errs.CodeApprovalCommentRequired, errs.KeyApprovalCommentRequired, "approval comment is required")
	}
	if !utf8.ValidString(comment) || utf8.RuneCountInString(comment) > 512 {
		return "", apperr.New(errs.CodeApprovalCommentInvalid, errs.KeyApprovalCommentInvalid, "approval comment must not exceed 512 characters")
	}
	return comment, nil
}

func (s *Service) requireDecisionPermission(ctx context.Context, actor uint64) error {
	if s.permissions == nil {
		return apperr.New(apperr.CodePermissionDenied, "auth.permission_denied", "permission denied")
	}
	allowed, err := s.permissions.Has(ctx, actor, decisionPermission)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "common.internal", "approval permission lookup failed")
	}
	if !allowed {
		return apperr.New(apperr.CodePermissionDenied, "auth.permission_denied", "permission denied")
	}
	return nil
}

func (s *Service) ListPending(ctx context.Context, actor uint64) ([]PendingApproval, error) {
	rows, err := s.repo.ListPending(ctx, actor)
	if err != nil {
		return nil, err
	}
	items := make([]PendingApproval, len(rows))
	for i := range rows {
		items[i] = PendingApproval{
			ID: rows[i].ApprovalID, RunID: rows[i].RunID, RunStageID: rows[i].RunStageID,
			ProjectID: rows[i].ProjectID, ProjectName: rows[i].ProjectName,
			EnvironmentKey: rows[i].EnvironmentKey, EnvironmentName: rows[i].EnvironmentName,
			StageOrder: rows[i].StageOrder, ChartName: rows[i].ChartName,
			ChartVersion: rows[i].ChartVersion, ChartDigest: rows[i].ChartDigest,
			InitiatorUserID: rows[i].InitiatorUserID, RequestedAt: rows[i].RequestedAt,
		}
	}
	return items, nil
}
