//go:build dbtest

package repo_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/modules/apps/repo"
)

// setupHTTP builds an isolated gin engine that injects a stub actor id (1)
// and mounts the five CRUD handlers under /apps/repos. Auth and RBAC are
// intentionally bypassed — those are exercised by tests/integration/.
func setupHTTP(t *testing.T) (*gin.Engine, *repo.Handler) {
	t.Helper()
	svc, _ := setupSvc(t)
	h := repo.NewHandler(svc)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.CtxKeyUserID, uint64(1))
		c.Next()
	})
	g := r.Group("/apps/repos")
	g.GET("", h.HandleList())
	g.GET("/:id", h.HandleGet())
	g.POST("", h.HandleCreate())
	g.PUT("/:id", h.HandleUpdate())
	g.DELETE("/:id", h.HandleDelete())
	return r, h
}

func TestHTTP_CreateAndList_NeverLeaksPassword(t *testing.T) {
	r, _ := setupHTTP(t)
	body, _ := json.Marshal(map[string]any{
		"name": "demo", "type": "http", "url": "https://x.example.com",
		"username": "u", "password": "secret",
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/apps/repos", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/apps/repos", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Items, 1)
	require.Equal(t, true, resp.Data.Items[0]["has_password"])
	_, hasPwd := resp.Data.Items[0]["password"]
	require.False(t, hasPwd, "password must not be returned to clients")
	_, hasEnc := resp.Data.Items[0]["encrypted_password"]
	require.False(t, hasEnc, "encrypted_password must not be returned to clients")
}

func TestHTTP_Update_NullPassword_Clears(t *testing.T) {
	r, _ := setupHTTP(t)
	// create with password
	body, _ := json.Marshal(map[string]any{
		"name": "p", "type": "http", "url": "x", "username": "u", "password": "secret",
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/apps/repos", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code)
	var created struct {
		Data repo.Detail `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	require.True(t, created.Data.HasPassword)

	// PUT with explicit null -> clears.
	patch := []byte(`{"password": null}`)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("PUT", "/apps/repos/"+itoa(created.Data.ID), bytes.NewReader(patch)))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	// GET back.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/apps/repos/"+itoa(created.Data.ID), nil))
	require.Equal(t, http.StatusOK, w.Code)
	var got struct {
		Data repo.Detail `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	require.False(t, got.Data.HasPassword, "explicit null must clear the ciphertext")
}

func TestHTTP_Update_OmittedPassword_Keeps(t *testing.T) {
	r, _ := setupHTTP(t)
	// create with password
	body, _ := json.Marshal(map[string]any{
		"name": "k", "type": "http", "url": "x", "username": "u", "password": "secret",
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/apps/repos", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code)
	var created struct {
		Data repo.Detail `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))

	// PUT with only description -> password preserved.
	patch := []byte(`{"description": "updated"}`)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("PUT", "/apps/repos/"+itoa(created.Data.ID), bytes.NewReader(patch)))
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/apps/repos/"+itoa(created.Data.ID), nil))
	require.Equal(t, http.StatusOK, w.Code)
	var got struct {
		Data repo.Detail `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	require.True(t, got.Data.HasPassword, "omitted password must keep the ciphertext")
	require.Equal(t, "updated", got.Data.Description)
}

func itoa(i uint64) string { return fmt.Sprintf("%d", i) }
