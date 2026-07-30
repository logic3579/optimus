package approval

import (
	"time"

	"optimus-be/internal/models"
)

// PendingApproval is the safe actionable-queue projection. Approval comments
// are absent because pending rows have no decision comment to disclose.
type PendingApproval struct {
	ID              uint64    `json:"id"`
	RunID           uint64    `json:"run_id"`
	RunStageID      uint64    `json:"run_stage_id"`
	ProjectID       uint64    `json:"project_id"`
	ProjectName     string    `json:"project_name"`
	EnvironmentKey  string    `json:"environment_key"`
	EnvironmentName string    `json:"environment_name"`
	StageOrder      int       `json:"stage_order"`
	ChartName       string    `json:"chart_name"`
	ChartVersion    string    `json:"chart_version"`
	ChartDigest     string    `json:"chart_digest"`
	InitiatorUserID uint64    `json:"initiator_user_id"`
	RequestedAt     time.Time `json:"requested_at"`
}

type DecisionRequest struct {
	Comment string `json:"comment" binding:"required,max=512"`
}

type Decision struct {
	ID              uint64                          `json:"id"`
	RunID           uint64                          `json:"run_id"`
	RunStageID      uint64                          `json:"run_stage_id"`
	Decision        models.DeliveryApprovalDecision `json:"decision"`
	DecidedByUserID uint64                          `json:"decided_by_user_id"`
	Comment         string                          `json:"comment"`
	DecidedAt       time.Time                       `json:"decided_at"`
}
