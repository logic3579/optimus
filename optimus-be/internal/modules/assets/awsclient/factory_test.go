package awsclient

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/assets/errs"
	"optimus-be/internal/modules/credentials"
)

func TestFor_RejectsInvalidProviderAndRegion(t *testing.T) {
	tests := []struct {
		name    string
		key     *credentials.CloudKey
		code    apperr.Code
		keyName string
	}{
		{"nil", nil, errs.CodeAssetsProviderUnsupported, errs.KeyProviderUnsupported},
		{"non-AWS", &credentials.CloudKey{Provider: "gcp"}, errs.CodeAssetsProviderUnsupported, errs.KeyProviderUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := For(context.Background(), tt.key, "us-east-1", time.Second)
			assertBizError(t, err, tt.code, tt.keyName)
		})
	}
	_, err := For(context.Background(), validCloudKey(), "", time.Second)
	assertBizError(t, err, errs.CodeAssetsRegionInvalid, errs.KeyRegionInvalid)
}

func TestFor_BuildsFreshConfiguredClients(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	first, err := For(context.Background(), validCloudKey(), "us-east-1", 5*time.Second)
	require.NoError(t, err)
	second, err := For(context.Background(), validCloudKey(), "us-east-1", 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, first.EC2)
	require.NotNil(t, first.RDS)
	require.NotSame(t, first.EC2, second.EC2)
	require.NotSame(t, first.RDS, second.RDS)

	ec2Options := first.EC2.Options()
	rdsOptions := first.RDS.Options()
	require.Equal(t, "us-east-1", ec2Options.Region)
	require.Equal(t, "us-east-1", rdsOptions.Region)
	require.Equal(t, 3, ec2Options.RetryMaxAttempts)
	require.Equal(t, 3, rdsOptions.RetryMaxAttempts)
	httpClient, ok := ec2Options.HTTPClient.(*http.Client)
	require.True(t, ok)
	require.Equal(t, 5*time.Second, httpClient.Timeout)

	creds, err := ec2Options.Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, "AKIATEST", creds.AccessKeyID)
	require.Equal(t, "test-secret", creds.SecretAccessKey)
}

func TestFor_NonPositiveTimeoutUsesDefault(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	clients, err := For(context.Background(), validCloudKey(), "us-west-2", 0)
	require.NoError(t, err)
	httpClient, ok := clients.EC2.Options().HTTPClient.(*http.Client)
	require.True(t, ok)
	require.Equal(t, defaultRequestTimeout, httpClient.Timeout)
}

func TestFor_ConfigErrorIsWrapped(t *testing.T) {
	wantErr := errors.New("config failure")
	original := loadDefaultConfig
	loadDefaultConfig = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, wantErr
	}
	t.Cleanup(func() { loadDefaultConfig = original })
	_, err := For(context.Background(), validCloudKey(), "us-east-1", time.Second)
	assertBizError(t, err, errs.CodeAssetsAWSConfig, errs.KeyAWSConfig)
	require.ErrorIs(t, err, wantErr)
}

func TestFor_RejectsEndpointOverrideEnvironment(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	for _, envName := range endpointOverrideEnvNames {
		t.Run(envName, func(t *testing.T) {
			for _, name := range endpointOverrideEnvNames {
				t.Setenv(name, "")
			}
			hostile := "https://SECRET.invalid/" + envName
			t.Setenv(envName, hostile)
			_, err := For(context.Background(), validCloudKey(), "us-east-1", time.Second)
			assertBizError(t, err, errs.CodeAssetsAWSConfig, errs.KeyAWSConfig)
			var bizErr *apperr.BizError
			require.ErrorAs(t, err, &bizErr)
			require.NotContains(t, bizErr.Message, hostile)
			require.NotContains(t, bizErr.Message, "SECRET")
		})
	}
}

func TestFor_ForcesStandardRetryMode(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	for _, name := range endpointOverrideEnvNames {
		t.Setenv(name, "")
	}
	t.Setenv("AWS_RETRY_MODE", "adaptive")
	clients, err := For(context.Background(), validCloudKey(), "us-east-1", time.Second)
	require.NoError(t, err)
	require.Equal(t, aws.RetryModeStandard, clients.EC2.Options().RetryMode)
	require.Equal(t, aws.RetryModeStandard, clients.RDS.Options().RetryMode)
	require.Equal(t, 3, clients.EC2.Options().RetryMaxAttempts)
	require.Equal(t, 3, clients.RDS.Options().RetryMaxAttempts)
}

func validCloudKey() *credentials.CloudKey {
	return &credentials.CloudKey{
		Provider: "aws", AccessKeyID: "AKIATEST", SecretAccessKey: "test-secret",
	}
}

func assertBizError(t *testing.T, err error, code apperr.Code, key string) {
	t.Helper()
	var bizErr *apperr.BizError
	require.Error(t, err)
	require.True(t, errors.As(err, &bizErr))
	require.Equal(t, code, bizErr.Code)
	require.Equal(t, key, bizErr.MessageKey)
}
