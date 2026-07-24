package query

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type errorSourceLister struct{ err error }

func (f errorSourceLister) ListQuerySources(context.Context) ([]QuerySource, error) {
	return nil, f.err
}

func TestFunctionalAdaptersAndNoTimeoutBranch(t *testing.T) {
	loaded := false
	loader := DatasourceLoaderFunc(func(context.Context, uint64) (Datasource, error) {
		loaded = true
		return Datasource{ID: 7}, nil
	})
	row, err := loader.Load(t.Context(), 7)
	require.NoError(t, err)
	require.True(t, loaded)
	require.EqualValues(t, 7, row.ID)

	listed := false
	lister := SourceListerFunc(func(context.Context) ([]QuerySource, error) {
		listed = true
		return []QuerySource{{ID: 7, Name: "prom"}}, nil
	})
	sources, err := lister.ListQuerySources(t.Context())
	require.NoError(t, err)
	require.True(t, listed)
	require.Equal(t, "prom", sources[0].Name)

	s := &Service{limits: Limits{}}
	ctx, cancel := s.withTimeout(t.Context())
	cancel()
	require.Same(t, t.Context(), ctx)
}

func TestHandlerMetadataRoutesAndSourceFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &fakeHandlerService{}
	r := gin.New()
	NewHandler(svc, errorSourceLister{err: errors.New("database unavailable")}).
		Mount(r.Group("/observability"), func(string) gin.HandlerFunc {
			return func(c *gin.Context) { c.Next() }
		})
	for _, path := range []string{
		"/observability/datasources/1/labels",
		"/observability/datasources/1/label-values?label=job",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code)
	}
	for _, path := range []string{
		"/observability/datasources/0/labels",
		"/observability/datasources/nope/label-values?label=job",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusBadRequest, w.Code)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/observability/query-sources", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	NewHandler(svc).QuerySources(testContext(w, "/observability/query-sources"))
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func testContext(w *httptest.ResponseRecorder, path string) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	return c
}
