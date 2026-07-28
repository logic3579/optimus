package orchestrator

import (
	"context"
	"time"
)

// Executor is intentionally closed to the sole P6 MVP operation. Implementations
// must observe ctx promptly. The worker nevertheless stops waiting after context
// cancellation or lease loss and records reconciliation before returning.
type Executor interface {
	UpgradeExisting(context.Context, UpgradeRequest) (UpgradeResult, error)
}

type UpgradeRequest struct {
	ApplicationID uint64
	RepoID        uint64
	InitiatorID   uint64
	OperationID   string
	ChartName     string
	ChartVersion  string
	Digest        string
	Purpose       string
	Timeout       time.Duration
}

type UpgradeResult struct {
	Revision int64
	Digest   string
}
