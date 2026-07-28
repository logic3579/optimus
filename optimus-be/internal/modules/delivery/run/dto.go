package run

import (
	"time"

	"optimus-be/internal/models"
)

// CreateRequest selects one immutable artifact. RetryOfRunID is set only by
// the linked-retry path; it remains part of the server-computed fingerprint.
type CreateRequest struct {
	ChartRepoID  uint64  `json:"chart_repo_id" binding:"required"`
	ChartName    string  `json:"chart_name" binding:"required,max=128"`
	ChartVersion string  `json:"chart_version" binding:"required,max=128"`
	RetryOfRunID *uint64 `json:"-"`
}

// Application is the minimal safe P3 projection needed to freeze a run stage.
type Application struct {
	ID          uint64
	ChartRepoID uint64
	ChartName   string
	Installed   bool
	ClusterID   uint64
	Namespace   string
	ReleaseName string
}

type Stage struct {
	ID               uint64                    `json:"id"`
	EnvironmentID    uint64                    `json:"environment_id"`
	EnvironmentKey   string                    `json:"environment_key"`
	EnvironmentName  string                    `json:"environment_name"`
	ApplicationID    uint64                    `json:"application_id"`
	ClusterID        uint64                    `json:"cluster_id"`
	Namespace        string                    `json:"namespace"`
	ReleaseName      string                    `json:"release_name"`
	Order            int                       `json:"order"`
	Executor         models.DeliveryExecutor   `json:"executor"`
	ApprovalRequired bool                      `json:"approval_required"`
	Timeout          time.Duration             `json:"timeout" swaggertype:"string"`
	State            models.DeliveryStageState `json:"state"`
	OperationID      string                    `json:"operation_id"`
}

type Run struct {
	ID                 uint64                  `json:"id"`
	ProjectID          uint64                  `json:"project_id"`
	PipelineID         uint64                  `json:"pipeline_id"`
	PipelineVersion    int                     `json:"pipeline_version"`
	ChartRepoID        uint64                  `json:"chart_repo_id"`
	ChartName          string                  `json:"chart_name"`
	ChartVersion       string                  `json:"chart_version"`
	ChartDigest        string                  `json:"chart_digest"`
	InitiatorUserID    uint64                  `json:"initiator_user_id"`
	RequestFingerprint string                  `json:"request_fingerprint"`
	State              models.DeliveryRunState `json:"state"`
	RetryOfRunID       *uint64                 `json:"retry_of_run_id,omitempty"`
	CreatedAt          time.Time               `json:"created_at"`
	UpdatedAt          time.Time               `json:"updated_at"`
	Stages             []Stage                 `json:"stages"`
}
