package credentials

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWipeCloudKey(t *testing.T) {
	require.NotPanics(t, func() { Wipe(nil) })
	key := &CloudKey{
		Name: "production", Provider: "aws", Region: "us-east-1",
		AccessKeyID: "AKIATEST", SecretAccessKey: "secret",
	}
	Wipe(key)
	require.Empty(t, key.Provider)
	require.Empty(t, key.AccessKeyID)
	require.Empty(t, key.SecretAccessKey)
	require.Equal(t, "production", key.Name)
	require.Equal(t, "us-east-1", key.Region)
}
