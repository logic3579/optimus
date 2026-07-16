package sync

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/assets/awsclient"
	"optimus-be/internal/modules/assets/errs"
	"optimus-be/internal/modules/assets/sync/runs"
	"optimus-be/internal/modules/credentials"
)

type Clients = awsclient.Clients

type ClientFactory interface {
	For(context.Context, *credentials.CloudKey, string, time.Duration) (*Clients, error)
}

type ClientFactoryFunc func(context.Context, *credentials.CloudKey, string, time.Duration) (*Clients, error)

func (f ClientFactoryFunc) For(ctx context.Context, key *credentials.CloudKey, region string, timeout time.Duration) (*Clients, error) {
	return f(ctx, key, region, timeout)
}

type CloudKeyConsumer interface {
	GetCloudKey(context.Context, uint64, string) (*credentials.CloudKey, error)
}

type Fetcher interface {
	FetchInstances(context.Context, *Clients) ([]models.AWSInstance, error)
	FetchVPCsAndSubnets(context.Context, *Clients) ([]models.AWSVPC, []models.AWSSubnet, error)
	FetchDatabases(context.Context, *Clients) ([]models.AWSDatabase, error)
}

type Engine struct {
	db       *gorm.DB
	runs     *runs.Repo
	factory  ClientFactory
	fetcher  Fetcher
	consumer CloudKeyConsumer
	timeout  time.Duration
	locks    sync.Map
}

const syncRunFinishTimeout = 5 * time.Second

func NewEngine(db *gorm.DB, runRepo *runs.Repo, factory ClientFactory, fetcher Fetcher, consumer CloudKeyConsumer, timeout time.Duration) *Engine {
	return &Engine{db: db, runs: runRepo, factory: factory, fetcher: fetcher, consumer: consumer, timeout: timeout}
}

func (e *Engine) TryLock(accountID uint64) bool {
	_, loaded := e.locks.LoadOrStore(accountID, struct{}{})
	return !loaded
}

func (e *Engine) Unlock(accountID uint64) { e.locks.Delete(accountID) }

func (e *Engine) RunAll(ctx context.Context, trigger string) error {
	var accounts []models.CloudAccount
	if err := e.db.WithContext(ctx).Where("enabled = ?", true).Find(&accounts).Error; err != nil {
		return safeDatabaseError()
	}
	var accountErrors []error
	for i := range accounts {
		if err := ctx.Err(); err != nil {
			accountErrors = append(accountErrors, err)
			break
		}
		if err := e.RunAccount(ctx, accounts[i].ID, trigger, nil); err != nil {
			// A failed account cannot prevent the remaining eligible accounts from
			// receiving their scheduled sweep, but the scheduler must still be
			// told that this RunAll invocation was incomplete.
			accountErrors = append(accountErrors, fmt.Errorf("cloud account %d: %w", accounts[i].ID, err))
		}
	}
	return errors.Join(accountErrors...)
}

func (e *Engine) RunAccount(ctx context.Context, accountID uint64, trigger string, triggeredBy *uint64) error {
	if !e.TryLock(accountID) {
		return e.recordLockedSkip(ctx, accountID, trigger, triggeredBy)
	}
	defer e.Unlock(accountID)
	return e.RunAccountLocked(ctx, accountID, trigger, triggeredBy)
}

// RunAccountLocked runs an account whose lock is already owned by the caller.
// The asynchronous manual-sync handler uses this entry point so the worker,
// rather than the HTTP handler, owns release of the account lock.
func (e *Engine) RunAccountLocked(ctx context.Context, accountID uint64, trigger string, triggeredBy *uint64) error {
	if triggeredBy != nil {
		ctx = credentials.WithActor(ctx, *triggeredBy)
	}
	var account models.CloudAccount
	if err := e.db.WithContext(ctx).First(&account, accountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(errs.CodeAssetsCloudAccountNotFound, errs.KeyCloudAccountNotFound, "cloud account not found")
		}
		return safeDatabaseError()
	}
	for _, region := range account.EnabledRegions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.sweepInstances(ctx, account, region, trigger, triggeredBy); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.sweepNetwork(ctx, account, region, trigger, triggeredBy); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.sweepDatabases(ctx, account, region, trigger, triggeredBy); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) sweepInstances(ctx context.Context, account models.CloudAccount, region, trigger string, triggeredBy *uint64) error {
	runID, err := e.insertRun(ctx, account.ID, region, "instance", trigger, triggeredBy)
	if err != nil {
		return err
	}
	clients, err := e.clientsForSweep(ctx, account.ID, account.CloudKeyID, region, "instance", trigger, triggeredBy == nil)
	if err != nil {
		return e.finishUpstreamFailure(ctx, runID, err)
	}
	started := time.Now()
	items, err := e.fetcher.FetchInstances(ctx, clients)
	if err != nil {
		return e.finishUpstreamFailure(ctx, runID, err)
	}
	softDeleted, err := UpsertInstances(ctx, e.db, account.ID, region, started, items)
	if err != nil {
		return e.finishPersistence(ctx, runID, err)
	}
	return e.finishRun(ctx, runID, runs.FinishRequest{Status: "success", ItemsSeen: saturatingInt32(int64(len(items))), ItemsSoftDeleted: saturatingInt32(softDeleted)})
}

func (e *Engine) sweepNetwork(ctx context.Context, account models.CloudAccount, region, trigger string, triggeredBy *uint64) error {
	runID, err := e.insertRun(ctx, account.ID, region, "network", trigger, triggeredBy)
	if err != nil {
		return err
	}
	clients, err := e.clientsForSweep(ctx, account.ID, account.CloudKeyID, region, "network", trigger, triggeredBy == nil)
	if err != nil {
		return e.finishUpstreamFailure(ctx, runID, err)
	}
	started := time.Now()
	vpcs, subnets, err := e.fetcher.FetchVPCsAndSubnets(ctx, clients)
	if err != nil {
		return e.finishUpstreamFailure(ctx, runID, err)
	}
	vpcDeleted, subnetDeleted, err := UpsertVPCsAndSubnets(ctx, e.db, account.ID, region, started, vpcs, subnets)
	if err != nil {
		return e.finishPersistence(ctx, runID, err)
	}
	seen := saturatingAdd(int64(len(vpcs)), int64(len(subnets)))
	deleted := saturatingAdd(vpcDeleted, subnetDeleted)
	return e.finishRun(ctx, runID, runs.FinishRequest{Status: "success", ItemsSeen: saturatingInt32(seen), ItemsSoftDeleted: saturatingInt32(deleted)})
}

func (e *Engine) sweepDatabases(ctx context.Context, account models.CloudAccount, region, trigger string, triggeredBy *uint64) error {
	runID, err := e.insertRun(ctx, account.ID, region, "database", trigger, triggeredBy)
	if err != nil {
		return err
	}
	clients, err := e.clientsForSweep(ctx, account.ID, account.CloudKeyID, region, "database", trigger, triggeredBy == nil)
	if err != nil {
		return e.finishUpstreamFailure(ctx, runID, err)
	}
	started := time.Now()
	items, err := e.fetcher.FetchDatabases(ctx, clients)
	if err != nil {
		return e.finishUpstreamFailure(ctx, runID, err)
	}
	softDeleted, err := UpsertDatabases(ctx, e.db, account.ID, region, started, items)
	if err != nil {
		return e.finishPersistence(ctx, runID, err)
	}
	return e.finishRun(ctx, runID, runs.FinishRequest{Status: "success", ItemsSeen: saturatingInt32(int64(len(items))), ItemsSoftDeleted: saturatingInt32(softDeleted)})
}

func (e *Engine) clientsForSweep(ctx context.Context, accountID, cloudKeyID uint64, region, resourceType, trigger string, systemCaller bool) (*Clients, error) {
	purpose := syncPurpose(accountID, region, resourceType, trigger, systemCaller)
	key, err := e.consumer.GetCloudKey(ctx, cloudKeyID, purpose)
	if err != nil {
		return nil, err
	}
	defer credentials.Wipe(key)
	return e.factory.For(ctx, key, region, e.timeout)
}

func syncPurpose(accountID uint64, region, resourceType, trigger string, systemCaller bool) string {
	prefix := ""
	if systemCaller {
		prefix = "system:"
	}
	return fmt.Sprintf("%sassets.sync.%s.account-%d.%s.%s", prefix, trigger, accountID, region, resourceType)
}

func (e *Engine) insertRun(ctx context.Context, accountID uint64, region, resourceType, trigger string, triggeredBy *uint64) (uint64, error) {
	id, err := e.runs.Insert(ctx, runs.InsertRequest{CloudAccountID: accountID, Region: region, ResourceType: resourceType, Trigger: trigger, TriggeredByUserID: triggeredBy})
	if err != nil {
		return 0, safeDatabaseError()
	}
	return id, nil
}

func (e *Engine) finishRun(ctx context.Context, runID uint64, request runs.FinishRequest) error {
	// The request/fetch context commonly carries the cancellation that caused
	// the sweep to fail. Finalization gets a small independent budget so a run
	// does not remain "running" forever after cancellation or timeout.
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	finishCtx, cancel := context.WithTimeout(base, syncRunFinishTimeout)
	defer cancel()
	if err := e.runs.Finish(finishCtx, runID, request); err != nil {
		return safeDatabaseError()
	}
	return nil
}

func (e *Engine) finishUpstreamFailure(ctx context.Context, runID uint64, cause error) error {
	code, message := safeSyncRunError(cause)
	return e.finishRun(ctx, runID, runs.FinishRequest{Status: "failed", Error: message, ErrorCode: int32(code)})
}

func safeSyncRunError(cause error) (apperr.Code, string) {
	if biz, ok := apperr.AsBiz(cause); ok {
		switch biz.Code {
		case errs.CodeAssetsRegionInvalid:
			return biz.Code, "AWS region is invalid"
		case errs.CodeAssetsProviderUnsupported:
			return biz.Code, "cloud provider not supported"
		case errs.CodeAssetsCloudKeyNotFound:
			return biz.Code, "cloud key not found"
		case errs.CodeAssetsAWSUnauthorized:
			return biz.Code, "AWS credentials invalid or expired"
		case errs.CodeAssetsAWSForbidden:
			return biz.Code, "AWS API access denied"
		case errs.CodeAssetsAWSUnreachable:
			return biz.Code, "AWS API unreachable"
		case errs.CodeAssetsAWSThrottled:
			return biz.Code, "AWS API request throttled"
		case errs.CodeAssetsAWSOther:
			return biz.Code, "AWS API request failed"
		case errs.CodeAssetsAWSConfig:
			return biz.Code, "AWS SDK configuration failed"
		}
	}
	code, _, message := awsclient.MapError(cause)
	return code, message
}

func (e *Engine) finishPersistence(ctx context.Context, runID uint64, cause error) error {
	if errors.Is(cause, ErrSweepIneligible) {
		return e.finishRun(ctx, runID, runs.FinishRequest{Status: "skipped"})
	}
	return e.finishRun(ctx, runID, runs.FinishRequest{Status: "failed", Error: "asset persistence failed", ErrorCode: int32(apperr.CodeDBError)})
}

func (e *Engine) recordLockedSkip(ctx context.Context, accountID uint64, trigger string, triggeredBy *uint64) error {
	var account models.CloudAccount
	if err := e.db.WithContext(ctx).First(&account, accountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return safeDatabaseError()
	}
	region := ""
	if len(account.EnabledRegions) > 0 {
		region = account.EnabledRegions[0]
	}
	runID, err := e.insertRun(ctx, accountID, region, "instance", trigger, triggeredBy)
	if err != nil {
		return err
	}
	return e.finishRun(ctx, runID, runs.FinishRequest{Status: "skipped"})
}

func (e *Engine) PruneSyncRuns(ctx context.Context, days int) (int64, error) {
	count, err := e.runs.Prune(ctx, days)
	if err != nil {
		return 0, safeDatabaseError()
	}
	return count, nil
}

func safeDatabaseError() error {
	return apperr.New(apperr.CodeDBError, "common.database_error", "database operation failed")
}

func saturatingInt32(value int64) int32 {
	if value <= 0 {
		return 0
	}
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(value)
}

func saturatingAdd(left, right int64) int64 {
	if left < 0 {
		left = 0
	}
	if right < 0 {
		right = 0
	}
	if left > math.MaxInt64-right {
		return math.MaxInt64
	}
	return left + right
}
