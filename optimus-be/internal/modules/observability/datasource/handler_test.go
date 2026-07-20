package datasource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/middleware"
)

type fakeHandlerService struct {
	actor, id uint64
	ip, ua    string
}

func (*fakeHandlerService) List(context.Context, ListQuery) (*ListResponse, error) {
	return &ListResponse{}, nil
}
func (*fakeHandlerService) Get(context.Context, uint64) (*Detail, error) { return &Detail{}, nil }
func (*fakeHandlerService) Create(context.Context, uint64, string, string, CreateRequest) (*Detail, error) {
	return &Detail{}, nil
}
func (*fakeHandlerService) Update(context.Context, uint64, string, string, uint64, UpdateRequest) (*Detail, error) {
	return &Detail{}, nil
}
func (*fakeHandlerService) Delete(context.Context, uint64, string, string, uint64) error { return nil }
func (f *fakeHandlerService) TestConnection(_ context.Context, actor uint64, ip, ua string, id uint64) (*TestResponse, error) {
	f.actor, f.id, f.ip, f.ua = actor, id, ip, ua
	return &TestResponse{Reachable: true}, nil
}

func TestParseIDRejectsInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, raw := range []string{"0", "x", "-1"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Params = gin.Params{{Key: "id", Value: raw}}
		_, ok := parseID(c)
		require.False(t, ok)
	}
}
func TestHandlerTestPassesActorAndRequestMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &fakeHandlerService{}
	h := &Handler{svc: svc}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/observability/datasources/9/test", nil)
	c.Request.RemoteAddr = "192.0.2.4:1234"
	c.Request.Header.Set("User-Agent", "task6-test")
	c.Params = gin.Params{{Key: "id", Value: "9"}}
	c.Set(middleware.CtxKeyUserID, uint64(42))
	h.Test(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.EqualValues(t, 42, svc.actor)
	require.EqualValues(t, 9, svc.id)
	require.Equal(t, "192.0.2.4", svc.ip)
	require.Equal(t, "task6-test", svc.ua)
}
