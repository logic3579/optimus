//go:build dbtest

package sync

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/assets/errs"
	"optimus-be/internal/modules/assets/sync/runs"
	"optimus-be/internal/modules/credentials"
	"optimus-be/tests/dbtest"
)

type fakeFetcher struct {
	db            *gorm.DB
	accountID     uint64
	disableOnCall bool
	instances     []models.AWSInstance
	instanceErr   error
	cancel        context.CancelFunc
}

func (f *fakeFetcher) FetchInstances(context.Context, *Clients) ([]models.AWSInstance, error) {
	if f.cancel != nil {
		f.cancel()
	}
	if f.disableOnCall {
		if err := f.db.Model(&models.CloudAccount{}).Where("id = ?", f.accountID).Update("enabled", false).Error; err != nil {
			return nil, err
		}
	}
	return f.instances, f.instanceErr
}

func (*fakeFetcher) FetchVPCsAndSubnets(context.Context, *Clients) ([]models.AWSVPC, []models.AWSSubnet, error) {
	return nil, nil, nil
}

func (*fakeFetcher) FetchDatabases(context.Context, *Clients) ([]models.AWSDatabase, error) {
	return nil, nil
}

type countingConsumer struct {
	mu       sync.Mutex
	purposes []string
	keys     []*credentials.CloudKey
}

func (c *countingConsumer) GetCloudKey(_ context.Context, _ uint64, purpose string) (*credentials.CloudKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := &credentials.CloudKey{Provider: "aws", AccessKeyID: "access", SecretAccessKey: "secret"}
	c.purposes = append(c.purposes, purpose)
	c.keys = append(c.keys, key)
	return key, nil
}

type countingFactory struct {
	calls int
	err   error
}

func (f *countingFactory) For(context.Context, *credentials.CloudKey, string, time.Duration) (*Clients, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &Clients{}, nil
}

func setupEngine(t *testing.T, fetcher Fetcher) (*gorm.DB, *models.CloudAccount, *Engine, *countingConsumer, *countingFactory) {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "engine-key-"+t.Name())
	account := dbtest.SeedCloudAccount(t, gdb, key.ID, "engine-account-"+t.Name(), "us-east-1")
	consumer := &countingConsumer{}
	factory := &countingFactory{}
	engine := NewEngine(gdb, runs.NewRepo(gdb), factory, fetcher, consumer, time.Second)
	return gdb, account, engine, consumer, factory
}

func TestEngineRunAccountUsesFreshCredentialsAndClientsPerSweep(t *testing.T) {
	fetcher := &fakeFetcher{instances: []models.AWSInstance{{InstanceID: "i-a"}}}
	gdb, account, engine, consumer, factory := setupEngine(t, fetcher)

	require.NoError(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))
	require.Equal(t, 3, factory.calls)
	require.Len(t, consumer.purposes, 3)
	for _, purpose := range consumer.purposes {
		require.True(t, strings.HasPrefix(purpose, "system:assets.sync.cron.account-"), purpose)
	}
	for _, key := range consumer.keys {
		require.Empty(t, key.AccessKeyID)
		require.Empty(t, key.SecretAccessKey)
	}
	var runsFound []models.AssetsSyncRun
	require.NoError(t, gdb.Order("id").Find(&runsFound).Error)
	require.Len(t, runsFound, 3)
	for _, run := range runsFound {
		require.Equal(t, "success", run.Status)
	}
}

func TestEngineFetcherFailureIsSanitizedAndDoesNotSoftDelete(t *testing.T) {
	secretError := errors.New("upstream failed with secret-access-material")
	fetcher := &fakeFetcher{instanceErr: secretError}
	gdb, account, engine, _, _ := setupEngine(t, fetcher)
	dbtest.SeedAWSInstance(t, gdb, account.ID, "us-east-1", "i-old")

	require.NoError(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))
	var run models.AssetsSyncRun
	require.NoError(t, gdb.Where("resource_type = ?", "instance").First(&run).Error)
	require.Equal(t, "failed", run.Status)
	require.NotContains(t, run.Error, "secret-access-material")
	var alive int64
	require.NoError(t, gdb.Model(&models.AWSInstance{}).Where("instance_id = ?", "i-old").Count(&alive).Error)
	require.EqualValues(t, 1, alive)
}

func TestEngineMarksRunSkippedWhenPersistenceGateClosesAfterFetch(t *testing.T) {
	fetcher := &fakeFetcher{disableOnCall: true, instances: []models.AWSInstance{{InstanceID: "i-new"}}}
	gdb, account, engine, _, _ := setupEngine(t, fetcher)
	fetcher.db, fetcher.accountID = gdb, account.ID

	require.NoError(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))
	var run models.AssetsSyncRun
	require.NoError(t, gdb.Where("resource_type = ?", "instance").First(&run).Error)
	require.Equal(t, "skipped", run.Status)
	var count int64
	require.NoError(t, gdb.Unscoped().Model(&models.AWSInstance{}).Where("instance_id = ?", "i-new").Count(&count).Error)
	require.Zero(t, count)
}

func TestEngineLockedRunWritesOneSkippedRowWithoutCredentialAudit(t *testing.T) {
	gdb, account, engine, consumer, _ := setupEngine(t, &fakeFetcher{})
	require.True(t, engine.TryLock(account.ID))
	defer engine.Unlock(account.ID)

	require.NoError(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))
	var rows []models.AssetsSyncRun
	require.NoError(t, gdb.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "skipped", rows[0].Status)
	require.Empty(t, consumer.purposes)
}

func TestEngineCancellationFinalizesRunningRunAsFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fetcher := &fakeFetcher{instanceErr: context.Canceled, cancel: cancel}
	gdb, account, engine, _, _ := setupEngine(t, fetcher)

	err := engine.RunAccount(ctx, account.ID, "cron", nil)
	require.ErrorIs(t, err, context.Canceled)

	var run models.AssetsSyncRun
	require.NoError(t, gdb.Where("resource_type = ?", "instance").First(&run).Error)
	require.Equal(t, "failed", run.Status)
	require.NotNil(t, run.FinishedAt)
	var running int64
	require.NoError(t, gdb.Model(&models.AssetsSyncRun{}).Where("status = ?", "running").Count(&running).Error)
	require.Zero(t, running)
}

func TestEngineFactoryConfigErrorKeepsSafe43107Code(t *testing.T) {
	gdb, account, engine, _, factory := setupEngine(t, &fakeFetcher{})
	factory.err = apperr.Wrap(errors.New("secret-config-cause"), errs.CodeAssetsAWSConfig, errs.KeyAWSConfig, "failed to load AWS SDK configuration")

	require.NoError(t, engine.RunAccount(context.Background(), account.ID, "cron", nil))
	var found []models.AssetsSyncRun
	require.NoError(t, gdb.Order("id").Find(&found).Error)
	require.Len(t, found, 3)
	for _, run := range found {
		require.Equal(t, "failed", run.Status)
		require.EqualValues(t, errs.CodeAssetsAWSConfig, run.ErrorCode)
		require.NotContains(t, run.Error, "secret-config-cause")
	}
}

func TestEngineRunAllContinuesAfterAccountErrorAndReturnsAggregate(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "..", "migrations"))
	t.Cleanup(teardown)
	key := dbtest.SeedCloudKey(t, gdb, "run-all-key-"+t.Name())
	broken := dbtest.SeedCloudAccount(t, gdb, key.ID, "run-all-broken-"+t.Name(), strings.Repeat("r", 33))
	healthy := dbtest.SeedCloudAccount(t, gdb, key.ID, "run-all-healthy-"+t.Name(), "us-east-1")
	consumer := &countingConsumer{}
	engine := NewEngine(gdb, runs.NewRepo(gdb), &countingFactory{}, &fakeFetcher{}, consumer, time.Second)

	err := engine.RunAll(context.Background(), "cron")
	require.Error(t, err)
	var healthyRuns int64
	require.NoError(t, gdb.Model(&models.AssetsSyncRun{}).Where("cloud_account_id = ? AND status = ?", healthy.ID, "success").Count(&healthyRuns).Error)
	require.EqualValues(t, 3, healthyRuns)
	var brokenRuns int64
	require.NoError(t, gdb.Model(&models.AssetsSyncRun{}).Where("cloud_account_id = ?", broken.ID).Count(&brokenRuns).Error)
	require.Zero(t, brokenRuns)
}
