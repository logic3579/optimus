//go:build dbtest

package credentials_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/credentials/cloudkey"
	"optimus-be/internal/modules/credentials/httpcredential"
	"optimus-be/internal/modules/credentials/kubeconfig"
	"optimus-be/internal/modules/credentials/sshkey"
	"optimus-be/internal/modules/credentials/vault"
)

const validKubeconfigYAML = `apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c1
  cluster: {server: https://127.0.0.1:6443, insecure-skip-tls-verify: true}
contexts:
- name: ctx
  context: {cluster: c1, user: u1, namespace: default}
users:
- name: u1
  user: {token: abc}
`

func setup(t *testing.T) (credentials.Consumer, *sshkey.Service, *kubeconfig.Service, *cloudkey.Service, *httpcredential.Service, func()) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	cipher, err := vault.NewCipher(key)
	require.NoError(t, err)
	rec := audit.NewRecorder(gdb)
	ssvc := sshkey.NewService(sshkey.NewRepo(gdb), cipher, rec)
	ksvc := kubeconfig.NewService(kubeconfig.NewRepo(gdb), cipher, rec)
	csvc := cloudkey.NewService(cloudkey.NewRepo(gdb), cipher, rec)
	hsvc := httpcredential.NewService(httpcredential.NewRepo(gdb), cipher, rec)
	return credentials.NewConsumer(ssvc, ksvc, csvc, hsvc), ssvc, ksvc, csvc, hsvc, td
}

func genPEM(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	blk, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)
	return string(pem.EncodeToMemory(blk))
}

func TestSmoke_SSH_RoundTrip(t *testing.T) {
	c, ssvc, _, _, _, td := setup(t)
	defer td()
	ctx := context.Background()
	pemStr := genPEM(t)

	d, err := ssvc.Create(ctx, 0, "", "", sshkey.CreateRequest{
		Name: "smoke-ssh", Username: "ops", PrivateKey: pemStr,
	})
	require.NoError(t, err)

	got, err := c.GetSSHKey(ctx, d.ID, "system:smoke.test")
	require.NoError(t, err)
	require.Equal(t, "smoke-ssh", got.Name)
	require.Equal(t, "ops", got.Username)
	require.Equal(t, pemStr, string(got.PrivateKey))
}

func TestSmoke_Kubeconfig_RoundTrip(t *testing.T) {
	c, _, ksvc, _, _, td := setup(t)
	defer td()
	ctx := context.Background()

	d, err := ksvc.Create(ctx, 0, "", "", kubeconfig.CreateRequest{
		Name: "smoke-kc", DefaultNamespace: "default", Kubeconfig: validKubeconfigYAML,
	})
	require.NoError(t, err)

	got, err := c.GetKubeconfig(ctx, d.ID, "system:smoke.test")
	require.NoError(t, err)
	require.Equal(t, "smoke-kc", got.Name)
	require.Equal(t, "default", got.DefaultNamespace)
	require.Equal(t, validKubeconfigYAML, string(got.YAML))
}

func TestSmoke_CloudKey_RoundTrip(t *testing.T) {
	c, _, _, csvc, _, td := setup(t)
	defer td()
	ctx := context.Background()

	d, err := csvc.Create(ctx, 0, "", "", cloudkey.CreateRequest{
		Name: "smoke-aws", Provider: "aws", Region: "us-east-1",
		AccessKeyID: "AKIA1", SecretAccessKey: "verysecret",
	})
	require.NoError(t, err)

	got, err := c.GetCloudKey(ctx, d.ID, "system:smoke.test")
	require.NoError(t, err)
	require.Equal(t, "aws", got.Provider)
	require.Equal(t, "us-east-1", got.Region)
	require.Equal(t, "AKIA1", got.AccessKeyID)
	require.Equal(t, "verysecret", got.SecretAccessKey)
}

func TestSmoke_SystemPurposeEnforced(t *testing.T) {
	c, _, _, csvc, _, td := setup(t)
	defer td()
	ctx := context.Background()

	d, err := csvc.Create(ctx, 0, "", "", cloudkey.CreateRequest{
		Name: "smoke-2", Provider: "aws", AccessKeyID: "k", SecretAccessKey: "s",
	})
	require.NoError(t, err)

	// System caller (no actor in ctx) without "system:" prefix → rejected.
	_, err = c.GetCloudKey(ctx, d.ID, "naked-purpose")
	require.Error(t, err)
}

func TestSmoke_WithActor_ContextPropagates(t *testing.T) {
	c, _, _, csvc, _, td := setup(t)
	defer td()
	// Use actorID=0 on Create (no FK to users), but inject an actor in the
	// ctx for Consume so the non-system purpose check passes.
	d, err := csvc.Create(context.Background(), 0, "", "", cloudkey.CreateRequest{
		Name: "smoke-actor", Provider: "gcp", AccessKeyID: "k", SecretAccessKey: "s",
	})
	require.NoError(t, err)

	ctxWithActor := credentials.WithActor(context.Background(), 1)
	// Actor present → non-system purpose accepted.
	got, err := c.GetCloudKey(ctxWithActor, d.ID, "any-purpose")
	require.NoError(t, err)
	require.Equal(t, "smoke-actor", got.Name)
}

func TestSmoke_HTTPCredential_RoundTripAndWipe(t *testing.T) {
	c, _, _, _, svc, td := setup(t)
	defer td()
	u := "metrics"
	d, err := svc.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "prom-basic", AuthType: "basic", Username: &u, Secret: "password"})
	require.NoError(t, err)
	got, err := c.GetHTTPCredential(t.Context(), d.ID, "system:observability.query")
	require.NoError(t, err)
	require.Equal(t, "basic", got.AuthType)
	require.Equal(t, "metrics", got.Username)
	require.Equal(t, []byte("password"), got.Secret)
	alias := got.Secret
	credentials.WipeHTTPCredential(got)
	require.Nil(t, got.Secret)
	require.Equal(t, make([]byte, len(alias)), alias)
	credentials.WipeHTTPCredential(nil)
}

func TestSmoke_HTTPCredential_PurposeAndCancellation(t *testing.T) {
	c, _, _, _, svc, td := setup(t)
	defer td()
	d, err := svc.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{Name: "prom-token", AuthType: "bearer", Secret: "token"})
	require.NoError(t, err)
	_, err = c.GetHTTPCredential(t.Context(), d.ID, "query")
	require.Error(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = c.GetHTTPCredential(ctx, d.ID, "system:query")
	require.ErrorIs(t, err, context.Canceled)
}

func TestSmoke_HTTPCredential_BearerRoundTripAndWipe(t *testing.T) {
	c, _, _, _, svc, td := setup(t)
	defer td()
	d, err := svc.Create(t.Context(), 0, "", "", httpcredential.CreateRequest{
		Name: "prom-bearer", AuthType: "bearer", Secret: "bearer-token",
	})
	require.NoError(t, err)

	got, err := c.GetHTTPCredential(t.Context(), d.ID, "system:observability.query")
	require.NoError(t, err)
	require.Equal(t, "prom-bearer", got.Name)
	require.Equal(t, "bearer", got.AuthType)
	require.Empty(t, got.Username)
	require.Equal(t, []byte("bearer-token"), got.Secret)

	alias := got.Secret
	credentials.WipeHTTPCredential(got)
	require.Nil(t, got.Secret)
	require.Equal(t, make([]byte, len(alias)), alias)
}
