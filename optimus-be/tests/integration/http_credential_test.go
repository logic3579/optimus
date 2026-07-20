//go:build dbtest

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/credentials/httpcredential"
	"optimus-be/internal/modules/credentials/vault"
	dsinuse "optimus-be/internal/modules/observability/datasource/inuse"
)

func TestHTTPCredentialHTTPIntegration(t *testing.T) {
	r, db := setupServer(t)
	token := login(t, r, "admin", "S3cret-Pass!")

	create := func(name, authType string, username *string, secret string) (uint64, map[string]any) {
		t.Helper()
		body := map[string]any{"name": name, "auth_type": authType, "secret": secret}
		if username != nil {
			body["username"] = *username
		}
		rec := assetsRequest(t, r, token, http.MethodPost, "/api/v1/credentials/http-credentials", body)
		require.Equalf(t, http.StatusOK, rec.Code, "create credential: %s", rec.Body.String())
		response := bodyMap(t, rec)
		data := response["data"].(map[string]any)
		return uint64(data["id"].(float64)), response
	}

	username := "metrics-reader"
	basicID, basicResponse := create("http-basic-integration", "basic", &username, "basic-password")
	bearerID, bearerResponse := create("http-bearer-integration", "bearer", nil, "bearer-token")
	for _, response := range []map[string]any{basicResponse, bearerResponse} {
		encoded, err := json.Marshal(response)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "secret")
		require.NotContains(t, string(encoded), "password")
		require.NotContains(t, string(encoded), "token")
		require.NotContains(t, string(encoded), "ciphertext")
	}

	var basicRow models.HTTPCredential
	require.NoError(t, db.First(&basicRow, basicID).Error)
	require.NotEqual(t, []byte("basic-password"), basicRow.SecretCiphertext)
	require.NotContains(t, string(basicRow.SecretCiphertext), "basic-password")
	cipher, err := vault.NewCipher(bytes.Repeat([]byte{0x42}, vault.KeyLen))
	require.NoError(t, err)
	plaintext, err := cipher.Open(basicRow.SecretCiphertext)
	require.NoError(t, err)
	require.Equal(t, "basic-password", string(plaintext))

	get := assetsRequest(t, r, token, http.MethodGet, fmt.Sprintf("/api/v1/credentials/http-credentials/%d", basicID), nil)
	require.Equal(t, http.StatusOK, get.Code)
	require.Equal(t, username, bodyMap(t, get)["data"].(map[string]any)["username"])
	require.NotContains(t, get.Body.String(), "basic-password")

	replacement := "rotated-password"
	update := assetsRequest(t, r, token, http.MethodPut, fmt.Sprintf("/api/v1/credentials/http-credentials/%d", basicID), map[string]any{
		"name": "http-basic-renamed", "secret": replacement,
	})
	require.Equalf(t, http.StatusOK, update.Code, "update credential: %s", update.Body.String())
	require.NotContains(t, update.Body.String(), replacement)
	bearerGet := assetsRequest(t, r, token, http.MethodGet, fmt.Sprintf("/api/v1/credentials/http-credentials/%d", bearerID), nil)
	require.Equal(t, http.StatusOK, bearerGet.Code)
	require.Equal(t, "bearer", bodyMap(t, bearerGet)["data"].(map[string]any)["auth_type"])
	require.NotContains(t, bearerGet.Body.String(), "bearer-token")
	rotatedToken := "rotated-bearer-token"
	bearerUpdate := assetsRequest(t, r, token, http.MethodPut, fmt.Sprintf("/api/v1/credentials/http-credentials/%d", bearerID), map[string]any{
		"name": "http-bearer-renamed", "secret": rotatedToken,
	})
	require.Equalf(t, http.StatusOK, bearerUpdate.Code, "update bearer: %s", bearerUpdate.Body.String())
	require.NotContains(t, bearerUpdate.Body.String(), rotatedToken)
	var bearerRow models.HTTPCredential
	require.NoError(t, db.First(&bearerRow, bearerID).Error)
	require.NotContains(t, string(bearerRow.SecretCiphertext), rotatedToken)
	bearerPlaintext, err := cipher.Open(bearerRow.SecretCiphertext)
	require.NoError(t, err)
	require.Equal(t, rotatedToken, string(bearerPlaintext))
	bearerList := assetsRequest(t, r, token, http.MethodGet, "/api/v1/credentials/http-credentials?q=http-bearer&auth_type=bearer", nil)
	require.Equal(t, http.StatusOK, bearerList.Code)
	require.EqualValues(t, 1, bodyMap(t, bearerList)["data"].(map[string]any)["total"])
	require.NotContains(t, bearerList.Body.String(), rotatedToken)

	recorder := audit.NewRecorder(db)
	httpSvc := httpcredential.NewService(httpcredential.NewRepo(db), cipher, recorder)
	consumer := credentials.NewConsumer(nil, nil, nil, httpSvc)
	admin := models.User{}
	require.NoError(t, db.Where("username = ?", "admin").First(&admin).Error)
	consumed, err := consumer.GetHTTPCredential(credentials.WithActor(context.Background(), admin.ID), bearerID, "observability.query.instant")
	require.NoError(t, err)
	require.Equal(t, rotatedToken, string(consumed.Secret))
	credentials.WipeHTTPCredential(consumed)

	var consumeAudit models.AuditLog
	require.NoError(t, db.Where("action = ? AND target_id = ?", "credentials.consume.http_credential", fmt.Sprint(bearerID)).Order("id DESC").First(&consumeAudit).Error)
	require.Equal(t, &admin.ID, consumeAudit.UserID)
	require.Contains(t, string(consumeAudit.Payload), "observability.query.instant")
	require.NotContains(t, string(consumeAudit.Payload), rotatedToken)

	httpSvc.SetInUseCounter(dsinuse.New(db))
	datasource := &models.ObservabilityDatasource{Name: "http-credential-delete-guard", BaseURL: "https://prometheus.example.test", AuthType: "bearer", HTTPCredentialID: &bearerID}
	require.NoError(t, db.Create(datasource).Error)
	err = httpSvc.Delete(t.Context(), admin.ID, "127.0.0.1", "integration", bearerID)
	require.Error(t, err)
	biz, ok := apperr.AsBiz(err)
	require.True(t, ok)
	require.Equal(t, apperr.CodeConflict, biz.Code)

	require.NoError(t, db.Delete(datasource).Error)
	bearerDelete := assetsRequest(t, r, token, http.MethodDelete, fmt.Sprintf("/api/v1/credentials/http-credentials/%d", bearerID), nil)
	require.Equalf(t, http.StatusOK, bearerDelete.Code, "delete bearer: %s", bearerDelete.Body.String())
	var deleted models.HTTPCredential
	require.NoError(t, db.Unscoped().First(&deleted, bearerID).Error)
	require.True(t, deleted.DeletedAt.Valid)

	list := assetsRequest(t, r, token, http.MethodGet, "/api/v1/credentials/http-credentials?q=http-basic&auth_type=basic", nil)
	require.Equal(t, http.StatusOK, list.Code)
	require.EqualValues(t, 1, bodyMap(t, list)["data"].(map[string]any)["total"])
	require.False(t, strings.Contains(list.Body.String(), replacement))
	basicDelete := assetsRequest(t, r, token, http.MethodDelete, fmt.Sprintf("/api/v1/credentials/http-credentials/%d", basicID), nil)
	require.Equalf(t, http.StatusOK, basicDelete.Code, "delete basic: %s", basicDelete.Body.String())
	deleted = models.HTTPCredential{}
	require.NoError(t, db.Unscoped().First(&deleted, basicID).Error)
	require.True(t, deleted.DeletedAt.Valid)
}
