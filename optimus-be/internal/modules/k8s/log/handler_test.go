package log_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	logh "optimus-be/internal/modules/k8s/log"
)

// fakeCS adapts a kubernetes.Interface to the StreamClientsetter interface
// expected by the handler.
type fakeCS struct{ cs kubernetes.Interface }

func (f *fakeCS) ClientsetForStream(_ context.Context, _ uint64, _ string) (kubernetes.Interface, error) {
	return f.cs, nil
}

// TestStream_PumpsLines asserts the SSE wire format and ensures no goroutines
// leak from the keepalive ticker / scanner pump after the handler returns.
//
// The default `fake.NewSimpleClientset().CoreV1().Pods(ns).GetLogs(...)`
// returns the literal string "fake logs" — enough to verify the SSE envelope.
func TestStream_PumpsLines(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreCurrent(),
		goleak.IgnoreTopFunction("net/http.(*Server).Serve"),
	)

	cs := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "n"},
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := logh.NewHandler(&fakeCS{cs: cs})
	r.GET("/c/:id/pods/:ns/:name/log", h.Stream())

	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		srv.URL+"/c/1/pods/n/p/log?follow=false", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	require.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
	require.Equal(t, "no", resp.Header.Get("X-Accel-Buffering"))

	body, _ := io.ReadAll(resp.Body)
	// The fake clientset's GetLogs returns "fake logs" as a single line.
	// Wrapped as SSE: "data: fake logs\n\n".
	require.True(t, strings.Contains(string(body), "data: "), "no SSE data lines in %q", string(body))
}

// TestStream_InvalidClusterID verifies the cluster-id guard returns 400
// before we touch the clientset factory.
func TestStream_InvalidClusterID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := logh.NewHandler(&fakeCS{cs: fake.NewSimpleClientset()})
	r.GET("/c/:id/pods/:ns/:name/log", h.Stream())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/c/0/pods/n/p/log", nil))
	require.Equal(t, 400, w.Code)
}
