package pipeline

import (
	"encoding/json"
	"errors"
	"time"
)

type PublishRequest struct {
	Stages []StageInput `json:"stages" binding:"required,min=1,max=20,dive"`
}

type StageInput struct {
	EnvironmentID    uint64        `json:"environment_id" binding:"required"`
	ApprovalRequired bool          `json:"approval_required"`
	Timeout          time.Duration `json:"timeout" swaggertype:"string" example:"10m"`
}

func (s StageInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(stageInputWire{
		EnvironmentID: s.EnvironmentID, ApprovalRequired: s.ApprovalRequired,
		Timeout: durationString(s.Timeout),
	})
}

func (s *StageInput) UnmarshalJSON(data []byte) error {
	var wire stageInputDecodeWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	timeout, err := decodeDurationString(wire.Timeout)
	if err != nil {
		return err
	}
	*s = StageInput{
		EnvironmentID: wire.EnvironmentID, ApprovalRequired: wire.ApprovalRequired,
		Timeout: timeout,
	}
	return nil
}

type Stage struct {
	ID               uint64        `json:"id"`
	EnvironmentID    uint64        `json:"environment_id"`
	Order            int           `json:"order"`
	ApprovalRequired bool          `json:"approval_required"`
	Timeout          time.Duration `json:"timeout" swaggertype:"string" example:"10m"`
}

func (s Stage) MarshalJSON() ([]byte, error) {
	return json.Marshal(stageWire{
		ID: s.ID, EnvironmentID: s.EnvironmentID, Order: s.Order,
		ApprovalRequired: s.ApprovalRequired, Timeout: durationString(s.Timeout),
	})
}

type Pipeline struct {
	ID              uint64    `json:"id"`
	ProjectID       uint64    `json:"project_id"`
	Version         int       `json:"version"`
	CreatedByUserID uint64    `json:"created_by_user_id"`
	PublishedAt     time.Time `json:"published_at"`
	IsCurrent       bool      `json:"is_current"`
	Stages          []Stage   `json:"stages"`
}

type durationString time.Duration

func (d durationString) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

type stageInputWire struct {
	EnvironmentID    uint64         `json:"environment_id"`
	ApprovalRequired bool           `json:"approval_required"`
	Timeout          durationString `json:"timeout"`
}

type stageInputDecodeWire struct {
	EnvironmentID    uint64          `json:"environment_id"`
	ApprovalRequired bool            `json:"approval_required"`
	Timeout          json.RawMessage `json:"timeout"`
}

type stageWire struct {
	ID               uint64         `json:"id"`
	EnvironmentID    uint64         `json:"environment_id"`
	Order            int            `json:"order"`
	ApprovalRequired bool           `json:"approval_required"`
	Timeout          durationString `json:"timeout"`
}

func decodeDurationString(data json.RawMessage) (time.Duration, error) {
	if len(data) == 0 {
		return 0, nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil || raw == "" {
		return 0, errors.New("timeout must be a duration string")
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.New("invalid timeout duration")
	}
	return timeout, nil
}
