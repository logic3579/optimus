//go:build dbtest

package audit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/modules/audit"
)

func TestHandler_List(t *testing.T) {
	gdb, td := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer td()
	rec := audit.NewRecorder(gdb)
	require.NoError(t, rec.Record(context.Background(), audit.Event{Action: "user.create"}))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	audit.NewHandler(audit.NewService(audit.NewRepo(gdb))).Register(api.Group("/audit-logs"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "user.create")
}
