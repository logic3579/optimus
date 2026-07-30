package event

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"optimus-be/internal/models"
)

func TestStreamResumesFromLastEventIDAndWritesBoundedSSE(t *testing.T) {
	repo := &fakeRepository{exists: true, rows: []models.DeliveryRunEvent{eventRow(12)}}
	h := NewHandler(NewService(repo), 5*time.Millisecond, 1)
	h.pollInterval = 2 * time.Millisecond
	r := gin.New()
	r.GET("/delivery/runs/:id/events", h.Stream)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/delivery/runs/7/events", nil).WithContext(ctx)
	req.Header.Set("Last-Event-ID", "11")
	w := &cancelWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	r.ServeHTTP(w, req)

	require.Equal(t, uint64(11), repo.cursor)
	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	require.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	require.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))
	require.True(t, w.deadlineCleared)
	body := w.Body.String()
	require.Contains(t, body, "id: 12\nevent: delivery\ndata: ")
	require.NotContains(t, body, "values")
}

func TestStreamEmitsHeartbeatAndStopsOnCancellation(t *testing.T) {
	repo := &fakeRepository{exists: true}
	h := NewHandler(NewService(repo), time.Millisecond, 1)
	h.pollInterval = time.Millisecond
	r := gin.New()
	r.GET("/delivery/runs/:id/events", h.Stream)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/delivery/runs/7/events", nil).WithContext(ctx)
	w := &cancelOnHeartbeatWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	done := make(chan struct{})
	go func() { r.ServeHTTP(w, req); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not stop after request cancellation")
	}
	require.Contains(t, w.Body.String(), ": heartbeat\n\n")
}

func TestStreamRejectsInvalidCursorBeforeStartingSSE(t *testing.T) {
	h := NewHandler(NewService(&fakeRepository{exists: true}), time.Second, 1)
	r := gin.New()
	r.GET("/delivery/runs/:id/events", h.Stream)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/delivery/runs/7/events", nil)
	req.Header.Set("Last-Event-ID", "not-numeric")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.NotEqual(t, "text/event-stream", w.Header().Get("Content-Type"))
}

func TestStreamRejectsOversizedLastEventID(t *testing.T) {
	h := NewHandler(NewService(&fakeRepository{exists: true}), time.Hour, 1)
	r := gin.New()
	r.GET("/delivery/runs/:id/events", h.Stream)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/delivery/runs/7/events", nil)
	req.Header.Set("Last-Event-ID", strings.Repeat("9", maxCursorBytes+1))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMountAppliesRunReadPermissionAtRouteBoundary(t *testing.T) {
	var permission string
	r := gin.New()
	NewHandler(NewService(&fakeRepository{exists: true}), time.Second, 1).Mount(r.Group("/delivery"), func(code string) gin.HandlerFunc {
		permission = code
		return func(c *gin.Context) { c.AbortWithStatus(http.StatusForbidden) }
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/delivery/runs/7/events", nil))
	require.Equal(t, "delivery:run:read", permission)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestStreamSaturationUsesEnvelopeAndReleasesConnection(t *testing.T) {
	h := NewHandler(NewService(&fakeRepository{exists: true}), time.Hour, 1)
	r := gin.New()
	r.GET("/delivery/runs/:id/events", h.Stream)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		r.ServeHTTP(&signalWriter{ResponseRecorder: httptest.NewRecorder(), started: started}, httptest.NewRequest(http.MethodGet, "/delivery/runs/7/events", nil).WithContext(ctx))
		close(firstDone)
	}()
	<-started
	limited := httptest.NewRecorder()
	r.ServeHTTP(limited, httptest.NewRequest(http.MethodGet, "/delivery/runs/7/events", nil))
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	require.Contains(t, limited.Header().Get("Content-Type"), "application/json")
	cancel()
	<-firstDone

	released := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/delivery/runs/7/events", nil)
	req.Header.Set("Last-Event-ID", "bad")
	r.ServeHTTP(released, req)
	require.Equal(t, http.StatusBadRequest, released.Code)
}

func TestStreamInitialPageFailureReturnsGenericEnvelopeBeforeSSE(t *testing.T) {
	for name, repo := range map[string]*fakeRepository{
		"repository error": {exists: true, err: errors.New("credential=secret")},
		"unsafe row":       {exists: true, rows: []models.DeliveryRunEvent{{ID: 1, RunID: 7, Metadata: []byte(`{"kubeconfig":"secret"}`)}}},
	} {
		t.Run(name, func(t *testing.T) {
			h := NewHandler(NewService(repo), time.Hour, 1)
			r := gin.New()
			r.GET("/delivery/runs/:id/events", h.Stream)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/delivery/runs/7/events", nil))
			require.Equal(t, http.StatusInternalServerError, w.Code)
			require.NotEqual(t, "text/event-stream", w.Header().Get("Content-Type"))
			require.NotContains(t, w.Body.String(), "credential")
			require.NotContains(t, w.Body.String(), "kubeconfig")
			require.NotContains(t, w.Body.String(), "secret")
		})
	}
}

func TestStreamExpiresAndReleasesConnection(t *testing.T) {
	h := NewHandler(NewService(&fakeRepository{exists: true}), time.Hour, 1)
	h.streamDuration = 2 * time.Millisecond
	h.pollInterval = time.Millisecond
	r := gin.New()
	r.GET("/delivery/runs/:id/events", h.Stream)
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/delivery/runs/7/events", nil))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/delivery/runs/7/events", nil)
	req.Header.Set("Last-Event-ID", "bad")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

type cancelWriter struct {
	*httptest.ResponseRecorder
	cancel          context.CancelFunc
	once            sync.Once
	deadlineCleared bool
}

func (w *cancelWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadlineCleared = deadline.IsZero()
	return nil
}

func (w *cancelWriter) Flush() {
	w.ResponseRecorder.Flush()
	if strings.Contains(w.Body.String(), "id: ") {
		w.once.Do(w.cancel)
	}
}

type cancelOnHeartbeatWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	once   sync.Once
}

type signalWriter struct {
	*httptest.ResponseRecorder
	started chan struct{}
	once    sync.Once
}

func (w *signalWriter) Flush() {
	w.ResponseRecorder.Flush()
	w.once.Do(func() { close(w.started) })
}

func (w *cancelOnHeartbeatWriter) Flush() {
	w.ResponseRecorder.Flush()
	if strings.Contains(w.Body.String(), ": heartbeat") {
		w.once.Do(w.cancel)
	}
}

var _ http.Flusher = (*cancelWriter)(nil)
var _ io.Writer = (*cancelWriter)(nil)
