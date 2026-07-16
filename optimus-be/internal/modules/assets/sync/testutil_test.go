package sync

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

func httptestNewServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// awsTestConfig is deliberately constructed without the default config loader
// so tests cannot read host profiles or AWS endpoint environment variables.
func awsTestConfig() aws.Config {
	return aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(awscreds.NewStaticCredentialsProvider("AKIATEST", "secret", "")),
		Retryer: func() aws.Retryer {
			return aws.NopRetryer{}
		},
	}
}

func newEC2Client(t *testing.T, handler http.Handler) *ec2.Client {
	t.Helper()
	server := httptestNewServer(t, handler)
	return ec2.NewFromConfig(awsTestConfig(), func(options *ec2.Options) {
		options.BaseEndpoint = aws.String(server.URL)
	})
}
