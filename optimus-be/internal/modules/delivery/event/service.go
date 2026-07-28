package event

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/delivery/errs"
)

const (
	pageLimit     = 100
	maxEventBytes = 16 << 10
)

var safeMetadataKeys = map[string]struct{}{
	"application_id": {}, "approval_id": {}, "attempt": {}, "chart_digest": {}, "chart_name": {},
	"chart_repo_id": {}, "chart_version": {}, "decision": {}, "environment_id": {}, "environment_key": {},
	"operation_id": {}, "pipeline_version": {}, "project_id": {}, "reason": {}, "release_revision": {},
	"release_status": {}, "retry_of_run_id": {}, "run_id": {}, "run_stage_id": {}, "stage_order": {},
	"timeout_seconds": {},
}

type repository interface {
	RunExists(context.Context, uint64) (bool, error)
	ListAfter(context.Context, uint64, uint64, int) ([]models.DeliveryRunEvent, error)
}

type Service struct{ repo repository }

func NewService(repo repository) *Service { return &Service{repo: repo} }

type Event struct {
	ID              uint64                        `json:"id"`
	RunID           uint64                        `json:"run_id"`
	RunStageID      *uint64                       `json:"run_stage_id,omitempty"`
	EventType       string                        `json:"event_type"`
	OldState        *string                       `json:"old_state,omitempty"`
	NewState        *string                       `json:"new_state,omitempty"`
	ActorType       models.DeliveryEventActorType `json:"actor_type"`
	ActorID         *uint64                       `json:"actor_id,omitempty"`
	OccurredAt      time.Time                     `json:"occurred_at"`
	ErrorCode       *int                          `json:"error_code,omitempty"`
	ErrorMessageKey *string                       `json:"error_message_key,omitempty"`
	CorrelationID   *string                       `json:"correlation_id,omitempty"`
	Metadata        json.RawMessage               `json:"metadata"`
}

func (s *Service) ValidateRun(ctx context.Context, runID uint64) error {
	if runID == 0 || s == nil || s.repo == nil {
		return runNotFoundError()
	}
	exists, err := s.repo.RunExists(ctx, runID)
	if err != nil {
		return err
	}
	if !exists {
		return runNotFoundError()
	}
	return nil
}

func (s *Service) ReadAfter(ctx context.Context, runID, cursor uint64) ([]Event, error) {
	rows, err := s.repo.ListAfter(ctx, runID, cursor, pageLimit)
	if err != nil {
		return nil, err
	}
	result := make([]Event, 0, len(rows))
	var previous uint64 = cursor
	for i := range rows {
		if rows[i].RunID != runID || rows[i].ID <= previous {
			return nil, errors.New("delivery event repository returned an invalid cursor page")
		}
		event, err := safeEvent(rows[i])
		if err != nil {
			return nil, err
		}
		result = append(result, event)
		previous = rows[i].ID
	}
	return result, nil
}

func safeEvent(row models.DeliveryRunEvent) (Event, error) {
	metadata := json.RawMessage(row.Metadata)
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	var decoded any
	if err := json.Unmarshal(metadata, &decoded); err != nil {
		return Event{}, errors.New("delivery event contains unsafe metadata")
	}
	if _, object := decoded.(map[string]any); !object || hasUnknownKey(decoded) {
		return Event{}, errors.New("delivery event contains unsafe metadata")
	}
	event := Event{ID: row.ID, RunID: row.RunID, RunStageID: row.RunStageID, EventType: row.EventType,
		OldState: row.OldState, NewState: row.NewState, ActorType: row.ActorType, ActorID: row.ActorID,
		OccurredAt: row.OccurredAt, ErrorCode: row.ErrorCode, ErrorMessageKey: row.ErrorMessageKey,
		CorrelationID: row.CorrelationID, Metadata: metadata}
	encoded, err := json.Marshal(event)
	if err != nil || len(encoded) > maxEventBytes {
		return Event{}, errors.New("delivery event exceeds serialized size limit")
	}
	return event, nil
}

func hasUnknownKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if _, allowed := safeMetadataKeys[strings.ToLower(key)]; !allowed || hasUnknownKey(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if hasUnknownKey(nested) {
				return true
			}
		}
	}
	return false
}

func runNotFoundError() error {
	return apperr.New(errs.CodeRunNotFound, errs.KeyRunNotFound, "delivery run not found")
}
