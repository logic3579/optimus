package sync

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/assets/errs"
	"optimus-be/internal/modules/credentials"
)

type recordingConsumer struct {
	key      *credentials.CloudKey
	purposes []string
}

func (c *recordingConsumer) GetCloudKey(_ context.Context, _ uint64, purpose string) (*credentials.CloudKey, error) {
	c.purposes = append(c.purposes, purpose)
	return c.key, nil
}

type recordingFactory struct {
	accessKey string
	secret    string
	calls     int
}

func (f *recordingFactory) For(_ context.Context, key *credentials.CloudKey, _ string, _ time.Duration) (*Clients, error) {
	f.calls++
	f.accessKey = key.AccessKeyID
	f.secret = key.SecretAccessKey
	return &Clients{}, nil
}

func TestClientsForSweepUsesDetailedPurposeAndWipesKey(t *testing.T) {
	tests := []struct {
		trigger      string
		systemCaller bool
		prefix       string
	}{
		{trigger: "cron", systemCaller: true, prefix: "system:assets.sync.cron."},
		{trigger: "manual", systemCaller: false, prefix: "assets.sync.manual."},
	}
	for _, tt := range tests {
		t.Run(tt.trigger, func(t *testing.T) {
			key := &credentials.CloudKey{Provider: "aws", AccessKeyID: "AKIA-sensitive", SecretAccessKey: "secret-sensitive"}
			consumer := &recordingConsumer{key: key}
			factory := &recordingFactory{}
			engine := NewEngine(nil, nil, factory, nil, consumer, time.Second)

			_, err := engine.clientsForSweep(context.Background(), 42, 7, "us-east-1", "instance", tt.trigger, tt.systemCaller)
			require.NoError(t, err)
			require.Equal(t, "AKIA-sensitive", factory.accessKey)
			require.Equal(t, "secret-sensitive", factory.secret)
			require.Len(t, consumer.purposes, 1)
			require.True(t, strings.HasPrefix(consumer.purposes[0], tt.prefix), consumer.purposes[0])
			require.Contains(t, consumer.purposes[0], "account-42.us-east-1.instance")
			require.Empty(t, key.Provider)
			require.Empty(t, key.AccessKeyID)
			require.Empty(t, key.SecretAccessKey)
		})
	}
}

func TestSaturatingCountsNeverWrap(t *testing.T) {
	require.Equal(t, int32(0), saturatingInt32(-1))
	require.Equal(t, int32(math.MaxInt32), saturatingInt32(math.MaxInt64))
	require.Equal(t, int64(math.MaxInt64), saturatingAdd(math.MaxInt64-2, 10))
}

func TestSafeSyncRunErrorPreservesWhitelistedCodeWithoutCauseText(t *testing.T) {
	cause := apperr.Wrap(errors.New("secret-config-cause"), errs.CodeAssetsAWSConfig, errs.KeyAWSConfig, "unsafe caller message")
	code, message := safeSyncRunError(cause)
	require.Equal(t, errs.CodeAssetsAWSConfig, code)
	require.Equal(t, "AWS SDK configuration failed", message)
	require.NotContains(t, message, "secret-config-cause")
	require.NotContains(t, message, "unsafe caller message")
}

func TestEngineTryLockGatesConcurrentRuns(t *testing.T) {
	engine := &Engine{}
	require.True(t, engine.TryLock(9))
	require.False(t, engine.TryLock(9))
	engine.Unlock(9)
	require.True(t, engine.TryLock(9))
}
