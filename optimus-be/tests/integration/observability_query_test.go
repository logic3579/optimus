//go:build dbtest

package integration_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/credentials/httpcredential"
	"optimus-be/internal/modules/credentials/vault"
	"optimus-be/internal/modules/observability/prometheus"
	"optimus-be/internal/modules/observability/query"
)

type fixedResolver struct{ address netip.Addr }

func (r fixedResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{address}, nil
	}
	return []netip.Addr{r.address}, nil
}

func TestObservabilityQueryHandlerEndToEnd(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "migrations"))
	defer teardown()
	actor := &models.User{Username: "query-actor", Email: "query@example.test", PasswordHash: "hash"}
	require.NoError(t, gdb.Create(actor).Error)
	recorder := audit.NewRecorder(gdb)
	cipher, err := vault.NewCipher(bytes.Repeat([]byte{0x51}, vault.KeyLen))
	require.NoError(t, err)
	httpService := httpcredential.NewService(httpcredential.NewRepo(gdb), cipher, recorder)
	username := "metrics"
	basic, err := httpService.Create(t.Context(), actor.ID, "", "", httpcredential.CreateRequest{Name: "query-basic", AuthType: "basic", Username: &username, Secret: "basic-secret"})
	require.NoError(t, err)
	bearer, err := httpService.Create(t.Context(), actor.ID, "", "", httpcredential.CreateRequest{Name: "query-bearer", AuthType: "bearer", Secret: "bearer-secret"})
	require.NoError(t, err)
	consumer := credentials.NewConsumer(nil, nil, nil, httpService)

	var mu sync.Mutex
	seenAuth := map[string]string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		q := r.Form.Get("query")
		mu.Lock()
		seenAuth[q] = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch q {
		case "bad_expression":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"parse failed"}`))
		case "slow_query":
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
			}
		case "huge_response":
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"string","result":[1,"` + strings.Repeat("x", 2048) + `"]}}`))
		default:
			if strings.HasSuffix(r.URL.Path, "/query_range") {
				_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"job":"api"},"values":[[1,"2"],[2,"3"]]}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"api"},"value":[1,"2"]}]}}`))
		}
	}))
	defer upstream.Close()
	serverURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	alias := netip.MustParseAddr("10.77.0.9")
	policy, err := prometheus.NewPolicy([]string{alias.String() + "/32"}, fixedResolver{address: alias})
	require.NoError(t, err)
	dialer := &net.Dialer{}
	transport := prometheus.NewTransportFactory(policy, func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, upstream.Listener.Addr().String())
	})
	baseURL := "http://prometheus.integration.test:" + serverURL.Port()
	basicID, bearerID := basic.ID, bearer.ID
	require.NoError(t, gdb.Create(&models.ObservabilityDatasource{Name: "query-basic", BaseURL: baseURL, AuthType: "basic", HTTPCredentialID: &basicID}).Error)
	require.NoError(t, gdb.Create(&models.ObservabilityDatasource{Name: "query-bearer", BaseURL: baseURL, AuthType: "bearer", HTTPCredentialID: &bearerID}).Error)
	var sources []models.ObservabilityDatasource
	require.NoError(t, gdb.Order("id").Find(&sources).Error)
	require.Len(t, sources, 2)

	loader := query.DatasourceLoaderFunc(func(ctx context.Context, id uint64) (query.Datasource, error) {
		var row models.ObservabilityDatasource
		err := gdb.WithContext(ctx).First(&row, id).Error
		return query.Datasource{ID: row.ID, BaseURL: row.BaseURL, AuthType: row.AuthType, CredentialID: row.HTTPCredentialID}, err
	})
	limits := query.Limits{MaxBatch: 12, MaxConcurrent: 4, MaxPromQLBytes: 8192, MaxRange: 24 * time.Hour, MinStep: 15 * time.Second, Timeout: 500 * time.Millisecond, MaxPoints: 1000, MaxEnrichmentIPs: 100}
	newEngine := func(maxBody int64, timeout time.Duration) *gin.Engine {
		l := limits
		l.Timeout = timeout
		svc := query.NewService(loader, consumer, query.PrometheusClientFactory{Transport: transport, MaxBody: maxBody, MaxSeries: 100}, nil, l)
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set(middleware.CtxKeyUserID, actor.ID); c.Next() })
		query.NewHandler(svc).Mount(r.Group("/observability"), func(string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } })
		return r
	}
	doJSON := func(r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, mustJSONBody(t, body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	instant := func(id uint64, queries ...map[string]string) map[string]any {
		return map[string]any{"datasource_id": id, "time": time.Unix(1, 0).UTC(), "queries": queries}
	}

	router := newEngine(16<<20, 500*time.Millisecond)
	beforeConsume := auditActionCount(t, gdb, "credentials.consume.http_credential")
	w := doJSON(router, "/observability/query", instant(sources[0].ID, map[string]string{"ref_id": "a", "promql": "basic_one"}, map[string]string{"ref_id": "b", "promql": "basic_two"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := bodyMap(t, w)["data"].(map[string]any)
	require.Len(t, data["results"], 2)
	require.Equal(t, beforeConsume+1, auditActionCount(t, gdb, "credentials.consume.http_credential"), "one credential consume per batch")
	w = doJSON(router, "/observability/query", instant(sources[1].ID, map[string]string{"ref_id": "bearer", "promql": "bearer_one"}, map[string]string{"ref_id": "bad", "promql": "bad_expression"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	results := bodyMap(t, w)["data"].(map[string]any)["results"].([]any)
	require.NotNil(t, results[0].(map[string]any)["result"])
	require.EqualValues(t, 44104, results[1].(map[string]any)["error"].(map[string]any)["code"])

	start := time.Unix(1, 0).UTC()
	w = doJSON(router, "/observability/query-range", map[string]any{"datasource_id": sources[1].ID, "start": start, "end": start.Add(time.Minute), "step": "15s", "queries": []map[string]string{{"ref_id": "range", "promql": "range_query"}}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	rangeResult := bodyMap(t, w)["data"].(map[string]any)["results"].([]any)[0].(map[string]any)["result"].(map[string]any)
	require.Equal(t, "matrix", rangeResult["result_type"])

	mu.Lock()
	require.Equal(t, "Basic bWV0cmljczpiYXNpYy1zZWNyZXQ=", seenAuth["basic_one"])
	require.Equal(t, "Bearer bearer-secret", seenAuth["bearer_one"])
	mu.Unlock()
	w = doJSON(newEngine(16<<20, 30*time.Millisecond), "/observability/query", instant(sources[1].ID, map[string]string{"ref_id": "slow", "promql": "slow_query"}))
	require.EqualValues(t, 44103, bodyMap(t, w)["code"])
	w = doJSON(newEngine(128, 500*time.Millisecond), "/observability/query", instant(sources[1].ID, map[string]string{"ref_id": "huge", "promql": "huge_response"}))
	require.EqualValues(t, 44105, bodyMap(t, w)["code"])

	denied := &models.ObservabilityDatasource{Name: "metadata-denied", BaseURL: "http://169.254.169.254", AuthType: "none"}
	require.NoError(t, gdb.Create(denied).Error)
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/observability/datasources/"+strconv.FormatUint(denied.ID, 10)+"/labels", nil)
	router.ServeHTTP(w, req)
	require.EqualValues(t, 44101, bodyMap(t, w)["code"])
	require.Zero(t, auditActionCount(t, gdb, "observability.query.instant"))
	require.Zero(t, auditActionCount(t, gdb, "observability.query.range"))
	require.Zero(t, auditActionCount(t, gdb, "observability.query.metadata"))
}

func auditActionCount(t *testing.T, db *gorm.DB, action string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.AuditLog{}).Where("action = ?", action).Count(&n).Error)
	return n
}
