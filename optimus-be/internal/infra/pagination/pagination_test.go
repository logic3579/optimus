package pagination_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/pagination"
)

func TestParse_Defaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?", nil)
	p := pagination.Parse(c)
	require.Equal(t, 1, p.Page)
	require.Equal(t, 20, p.PageSize)
	require.Equal(t, 0, p.Offset())
	require.Equal(t, 20, p.Limit())
}

func TestParse_RespectsValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?page=3&page_size=50", nil)
	p := pagination.Parse(c)
	require.Equal(t, 3, p.Page)
	require.Equal(t, 50, p.PageSize)
	require.Equal(t, 100, p.Offset())
}

func TestParse_ClampsToBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?page=0&page_size=999", nil)
	p := pagination.Parse(c)
	require.Equal(t, 1, p.Page)
	require.Equal(t, 100, p.PageSize)
}

func TestPageOf_BuildsEnvelope(t *testing.T) {
	out := pagination.Of([]int{1, 2, 3}, 27, pagination.Params{Page: 2, PageSize: 10})
	require.Equal(t, []int{1, 2, 3}, out.Items)
	require.EqualValues(t, 27, out.Total)
	require.Equal(t, 2, out.Page)
	require.Equal(t, 10, out.PageSize)
}
