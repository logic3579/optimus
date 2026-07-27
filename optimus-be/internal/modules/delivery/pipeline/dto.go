package pipeline

import "time"

type PublishRequest struct {
	Stages []StageInput `json:"stages" binding:"required,min=1,max=20,dive"`
}

type StageInput struct {
	EnvironmentID    uint64        `json:"environment_id" binding:"required"`
	ApprovalRequired bool          `json:"approval_required"`
	Timeout          time.Duration `json:"timeout"`
}

type Stage struct {
	ID               uint64        `json:"id"`
	EnvironmentID    uint64        `json:"environment_id"`
	Order            int           `json:"order"`
	ApprovalRequired bool          `json:"approval_required"`
	Timeout          time.Duration `json:"timeout"`
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
