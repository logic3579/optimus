package awsclient

import (
	"context"
	"net/http"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscredentials "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/assets/errs"
	"optimus-be/internal/modules/credentials"
)

const defaultRequestTimeout = 30 * time.Second

var loadDefaultConfig = awsconfig.LoadDefaultConfig

// Clients contains the AWS service clients used by one discovery sweep.
// Instances are constructed fresh for every call to For and are never cached.
type Clients struct {
	EC2 *ec2.Client
	RDS *rds.Client
}

// For builds AWS clients for one cloud key and region. The caller owns the
// returned clients for the duration of a single sweep.
func For(ctx context.Context, cloudKey *credentials.CloudKey, region string, timeout time.Duration) (*Clients, error) {
	if cloudKey == nil || cloudKey.Provider != "aws" {
		return nil, apperr.New(errs.CodeAssetsProviderUnsupported, errs.KeyProviderUnsupported, "cloud provider not supported")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		return nil, apperr.New(errs.CodeAssetsRegionInvalid, errs.KeyRegionInvalid, "AWS region is required")
	}
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	config, err := loadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithSharedConfigFiles([]string{}),
		awsconfig.WithSharedCredentialsFiles([]string{}),
		awsconfig.WithCredentialsProvider(awscredentials.NewStaticCredentialsProvider(
			cloudKey.AccessKeyID,
			cloudKey.SecretAccessKey,
			"",
		)),
		awsconfig.WithRetryMaxAttempts(3),
		awsconfig.WithHTTPClient(&http.Client{Timeout: timeout}),
	)
	if err != nil {
		return nil, apperr.Wrap(err, errs.CodeAssetsAWSConfig, errs.KeyAWSConfig, "failed to load AWS SDK configuration")
	}
	return &Clients{
		EC2: ec2.NewFromConfig(config),
		RDS: rds.NewFromConfig(config),
	}, nil
}
