package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type DeliveryRunState string

const (
	DeliveryRunQueued          DeliveryRunState = "queued"
	DeliveryRunRunning         DeliveryRunState = "running"
	DeliveryRunWaitingApproval DeliveryRunState = "waiting_approval"
	DeliveryRunCancelRequested DeliveryRunState = "cancel_requested"
	DeliveryRunReconciling     DeliveryRunState = "reconciling"
	DeliveryRunSucceeded       DeliveryRunState = "succeeded"
	DeliveryRunFailed          DeliveryRunState = "failed"
	DeliveryRunRejected        DeliveryRunState = "rejected"
	DeliveryRunCanceled        DeliveryRunState = "canceled"
	DeliveryRunTimedOut        DeliveryRunState = "timed_out"
	DeliveryRunOutcomeUnknown  DeliveryRunState = "outcome_unknown"
)

type DeliveryStageState string

const (
	DeliveryStagePending         DeliveryStageState = "pending"
	DeliveryStageWaitingApproval DeliveryStageState = "waiting_approval"
	DeliveryStageQueued          DeliveryStageState = "queued"
	DeliveryStageRunning         DeliveryStageState = "running"
	DeliveryStageReconciling     DeliveryStageState = "reconciling"
	DeliveryStageSucceeded       DeliveryStageState = "succeeded"
	DeliveryStageFailed          DeliveryStageState = "failed"
	DeliveryStageRejected        DeliveryStageState = "rejected"
	DeliveryStageCanceled        DeliveryStageState = "canceled"
	DeliveryStageTimedOut        DeliveryStageState = "timed_out"
	DeliveryStageOutcomeUnknown  DeliveryStageState = "outcome_unknown"
)

type DeliveryApprovalDecision string

const (
	DeliveryApprovalPending  DeliveryApprovalDecision = "pending"
	DeliveryApprovalApproved DeliveryApprovalDecision = "approved"
	DeliveryApprovalRejected DeliveryApprovalDecision = "rejected"
)

type AppsReleaseOperationState string

const (
	AppsReleaseOperationActive      AppsReleaseOperationState = "active"
	AppsReleaseOperationSucceeded   AppsReleaseOperationState = "succeeded"
	AppsReleaseOperationFailed      AppsReleaseOperationState = "failed"
	AppsReleaseOperationReconciling AppsReleaseOperationState = "reconciling"
)

type DeliveryExecutor string

const DeliveryExecutorHelmUpgradeExistingRelease DeliveryExecutor = "helm_upgrade_existing_release"

type DeliveryEventActorType string

const (
	DeliveryEventActorSystem DeliveryEventActorType = "system"
	DeliveryEventActorUser   DeliveryEventActorType = "user"
)

type DeliveryProject struct {
	ID          uint64 `gorm:"primaryKey"`
	Name        string `gorm:"size:128;not null"`
	Description string `gorm:"type:text;not null;default:''"`
	OwnerUserID *uint64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (DeliveryProject) TableName() string { return "delivery_projects" }

type DeliveryEnvironment struct {
	ID             uint64 `gorm:"primaryKey"`
	ProjectID      uint64 `gorm:"not null;index"`
	EnvironmentKey string `gorm:"size:64;not null"`
	DisplayName    string `gorm:"size:128;not null"`
	ApplicationID  uint64 `gorm:"not null;index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (DeliveryEnvironment) TableName() string { return "delivery_environments" }

type DeliveryPipeline struct {
	ID              uint64 `gorm:"primaryKey"`
	ProjectID       uint64 `gorm:"not null;index"`
	Version         int    `gorm:"not null"`
	CreatedByUserID uint64 `gorm:"not null"`
	PublishedAt     time.Time
	IsCurrent       bool `gorm:"not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (DeliveryPipeline) TableName() string { return "delivery_pipelines" }

type DeliveryPipelineStage struct {
	ID               uint64 `gorm:"primaryKey"`
	PipelineID       uint64 `gorm:"not null;index"`
	EnvironmentID    uint64 `gorm:"not null;index"`
	StageOrder       int    `gorm:"not null"`
	Executor         DeliveryExecutor
	ApprovalRequired bool `gorm:"not null"`
	TimeoutSeconds   int  `gorm:"not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (DeliveryPipelineStage) TableName() string { return "delivery_pipeline_stages" }

type DeliveryRun struct {
	ID                 uint64 `gorm:"primaryKey"`
	ProjectID          uint64 `gorm:"not null;index"`
	PipelineID         uint64 `gorm:"not null;index"`
	PipelineVersion    int    `gorm:"not null"`
	ChartRepoID        uint64 `gorm:"not null"`
	ChartName          string `gorm:"size:128;not null"`
	ChartVersion       string `gorm:"size:128;not null"`
	ChartDigest        string `gorm:"size:128;not null"`
	InitiatorUserID    uint64 `gorm:"not null"`
	IdempotencyKey     string `gorm:"size:128;not null"`
	RequestFingerprint string `gorm:"size:128;not null"`
	State              DeliveryRunState
	RetryOfRunID       *uint64
	StartedAt          *time.Time
	FinishedAt         *time.Time
	ErrorCode          *int
	ErrorMessageKey    *string
	CorrelationID      *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (DeliveryRun) TableName() string { return "delivery_runs" }

type DeliveryRunStage struct {
	ID               uint64 `gorm:"primaryKey"`
	RunID            uint64 `gorm:"not null;index"`
	EnvironmentID    uint64 `gorm:"not null"`
	EnvironmentKey   string `gorm:"size:64;not null"`
	EnvironmentName  string `gorm:"size:128;not null"`
	ApplicationID    uint64 `gorm:"not null;index"`
	ClusterID        uint64 `gorm:"not null"`
	Namespace        string `gorm:"size:63;not null"`
	ReleaseName      string `gorm:"size:53;not null"`
	StageOrder       int    `gorm:"not null"`
	Executor         DeliveryExecutor
	ApprovalRequired bool `gorm:"not null"`
	TimeoutSeconds   int  `gorm:"not null"`
	State            DeliveryStageState
	OperationID      string `gorm:"size:64;not null"`
	LeaseOwner       *string
	LeaseExpiresAt   *time.Time
	ResultRevision   *int64
	ResultDigest     *string
	StartedAt        *time.Time
	FinishedAt       *time.Time
	ErrorCode        *int
	ErrorMessageKey  *string
	CorrelationID    *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (DeliveryRunStage) TableName() string { return "delivery_run_stages" }

type DeliveryApproval struct {
	ID              uint64 `gorm:"primaryKey"`
	RunID           uint64 `gorm:"not null;index"`
	RunStageID      uint64 `gorm:"not null;unique"`
	RequestedAt     time.Time
	Decision        DeliveryApprovalDecision
	DecidedByUserID *uint64
	Comment         string `gorm:"size:512;not null;default:''"`
	DecidedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (DeliveryApproval) TableName() string { return "delivery_approvals" }

type DeliveryRunEvent struct {
	ID              uint64 `gorm:"primaryKey"`
	RunID           uint64 `gorm:"not null;index"`
	RunStageID      *uint64
	EventType       string `gorm:"size:64;not null"`
	OldState        *string
	NewState        *string
	ActorType       DeliveryEventActorType
	ActorID         *uint64
	OccurredAt      time.Time
	ErrorCode       *int
	ErrorMessageKey *string
	CorrelationID   *string
	Metadata        datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
}

func (DeliveryRunEvent) TableName() string { return "delivery_run_events" }

type AppsReleaseOperation struct {
	ID             uint64 `gorm:"primaryKey"`
	ApplicationID  uint64 `gorm:"not null;index"`
	OperationID    string `gorm:"size:64;not null"`
	Kind           string `gorm:"size:64;not null"`
	State          AppsReleaseOperationState
	LeaseOwner     *string
	LeaseExpiresAt *time.Time
	ResultRevision *int64
	ResultDigest   *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	FinishedAt     *time.Time
}

func (AppsReleaseOperation) TableName() string { return "apps_release_operations" }
