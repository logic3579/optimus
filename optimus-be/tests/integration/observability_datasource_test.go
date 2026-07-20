//go:build dbtest

package integration_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/httpcredential"
	"optimus-be/internal/modules/credentials/vault"
	"optimus-be/internal/modules/k8s/cluster"
	"optimus-be/internal/modules/observability/datasource"
	dsinuse "optimus-be/internal/modules/observability/datasource/inuse"
	"optimus-be/tests/dbtest"
)

func TestObservabilityDatasourceIntegration(t *testing.T) {
	_, db := setupServer(t)
	ctx := context.Background()
	admin := models.User{}
	require.NoError(t, db.Where("username = ?", "admin").First(&admin).Error)
	cipher, err := vault.NewCipher(bytes.Repeat([]byte{0x42}, vault.KeyLen))
	require.NoError(t, err)
	recorder := audit.NewRecorder(db)
	httpSvc := httpcredential.NewService(httpcredential.NewRepo(db), cipher, recorder)
	username := "reader"
	basic, err := httpSvc.Create(ctx, admin.ID, "", "", httpcredential.CreateRequest{Name: "ds-basic", AuthType: "basic", Username: &username, Secret: "secret"})
	require.NoError(t, err)
	bearer, err := httpSvc.Create(ctx, admin.ID, "", "", httpcredential.CreateRequest{Name: "ds-bearer", AuthType: "bearer", Secret: "token"})
	require.NoError(t, err)
	clusterOne := dbtest.SeedCluster(t, db, "observability-cluster-one")
	clusterTwo := dbtest.SeedCluster(t, db, "observability-cluster-two")
	repo := datasource.NewRepo(db)
	usage := dsinuse.New(db)
	svc := datasource.NewService(repo, repo, repo, usage, nil, nil, recorder)

	create := func(name, auth string, credentialID, clusterID *uint64) *datasource.Detail {
		t.Helper()
		row, createErr := svc.Create(ctx, admin.ID, "127.0.0.1", "integration", datasource.CreateRequest{
			Name: name, BaseURL: "https://prometheus.example.test", AuthType: auth,
			HTTPCredentialID: credentialID, ClusterID: clusterID,
		})
		require.NoError(t, createErr)
		return row
	}

	one := create("prod-prometheus", "basic", &basic.ID, &clusterOne.ID)
	two := create("dev-prometheus", "bearer", &bearer.ID, &clusterTwo.ID)
	create("standalone-metrics", "none", nil, nil)

	listed, err := svc.List(ctx, datasource.ListQuery{Page: 1, PageSize: 20, Q: "prometheus", AuthType: "basic", ClusterID: &clusterOne.ID})
	require.NoError(t, err)
	require.EqualValues(t, 1, listed.Total)
	require.Equal(t, one.ID, listed.Items[0].ID)
	require.Equal(t, basic.ID, listed.Items[0].HTTPCredential.ID)
	require.Equal(t, clusterOne.ID, listed.Items[0].Cluster.ID)

	_, err = svc.Create(ctx, admin.ID, "", "", datasource.CreateRequest{Name: one.Name, BaseURL: "https://duplicate.example.test", AuthType: "none"})
	requireBizCode(t, err, apperr.CodeObservabilityDatasourceNameTaken)
	require.NoError(t, svc.Delete(ctx, admin.ID, "", "", one.ID))
	recreated := create(one.Name, "none", nil, nil)
	require.NotEqual(t, one.ID, recreated.ID)

	_, err = svc.Create(ctx, admin.ID, "", "", datasource.CreateRequest{Name: "none-with-credential", BaseURL: "https://example.test", AuthType: "none", HTTPCredentialID: &basic.ID})
	requireBizCode(t, err, apperr.CodeObservabilityDatasourceAuthMismatch)
	_, err = svc.Create(ctx, admin.ID, "", "", datasource.CreateRequest{Name: "basic-without-credential", BaseURL: "https://example.test", AuthType: "basic"})
	requireBizCode(t, err, apperr.CodeObservabilityDatasourceAuthMismatch)
	_, err = svc.Create(ctx, admin.ID, "", "", datasource.CreateRequest{Name: "wrong-credential-type", BaseURL: "https://example.test", AuthType: "basic", HTTPCredentialID: &bearer.ID})
	requireBizCode(t, err, apperr.CodeObservabilityDatasourceAuthMismatch)
	missingCredential := uint64(99999999)
	_, err = svc.Create(ctx, admin.ID, "", "", datasource.CreateRequest{Name: "missing-credential", BaseURL: "https://example.test", AuthType: "bearer", HTTPCredentialID: &missingCredential})
	requireBizCode(t, err, apperr.CodeObservabilityDatasourceAuthMismatch)
	missingCluster := uint64(99999999)
	_, err = svc.Create(ctx, admin.ID, "", "", datasource.CreateRequest{Name: "missing-cluster", BaseURL: "https://example.test", AuthType: "none", ClusterID: &missingCluster})
	requireBizCode(t, err, apperr.CodeValidation)

	httpSvc.SetInUseCounter(usage)
	err = httpSvc.Delete(ctx, admin.ID, "", "", bearer.ID)
	requireBizCode(t, err, apperr.CodeConflict)

	clusterSvc := cluster.NewService(cluster.NewRepo(db), nil, nil, recorder)
	clusterSvc.SetObservabilityCounter(usage)
	err = clusterSvc.Delete(ctx, admin.ID, "", "", clusterTwo.ID)
	requireBizCode(t, err, apperr.CodeConflict)

	require.NoError(t, svc.Delete(ctx, admin.ID, "", "", two.ID))
	require.NoError(t, httpSvc.Delete(ctx, admin.ID, "", "", bearer.ID))
	require.NoError(t, clusterSvc.Delete(ctx, admin.ID, "", "", clusterTwo.ID))

	var active int64
	require.NoError(t, db.Model(&models.ObservabilityDatasource{}).Where("name = ?", one.Name).Count(&active).Error)
	require.EqualValues(t, 1, active)
}

func requireBizCode(t *testing.T, err error, want apperr.Code) {
	t.Helper()
	require.Error(t, err)
	biz, ok := apperr.AsBiz(err)
	require.True(t, ok, "expected business error, got %v", err)
	require.Equal(t, want, biz.Code)
}
