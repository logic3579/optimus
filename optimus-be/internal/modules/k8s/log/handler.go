// Package log implements the SSE pod-log streaming endpoint.
//
// The handler proxies `kubectl logs -f` bytes from the apiserver to the HTTP
// client wrapped in Server-Sent Events. The per-request server write deadline
// (gin/main.go sets 15s) is cleared via http.NewResponseController so the
// stream can live for the duration of the follow.
package log

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
	k8serrs "optimus-be/internal/modules/k8s/apierr"
)

// StreamClientsetter is the narrow surface this handler needs from
// k8s/client.Factory — exposed as an interface so tests can inject a fake.
type StreamClientsetter interface {
	ClientsetForStream(ctx context.Context, clusterID uint64, purpose string) (kubernetes.Interface, error)
}

type Handler struct{ cs StreamClientsetter }

func NewHandler(cs StreamClientsetter) *Handler { return &Handler{cs: cs} }

func ptrInt64(v int64) *int64 { return &v }

// Stream returns the gin handler that pumps pod logs to the client as SSE.
func (h *Handler) Stream() gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || clusterID == 0 {
			response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid cluster id"))
			return
		}
		ns, pod := c.Param("ns"), c.Param("name")
		tail := int64(200)
		if t, err := strconv.ParseInt(c.Query("tailLines"), 10, 64); err == nil && t > 0 {
			tail = t
		}
		opts := corev1.PodLogOptions{
			Container:  c.Query("container"),
			Follow:     c.Query("follow") == "true",
			TailLines:  ptrInt64(tail),
			Previous:   c.Query("previous") == "true",
			Timestamps: true,
		}

		cs, err := h.cs.ClientsetForStream(c.Request.Context(), clusterID, "k8s.log.stream")
		if err != nil {
			response.Error(c, err)
			return
		}

		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		rc, err := cs.CoreV1().Pods(ns).GetLogs(pod, &opts).Stream(ctx)
		if err != nil {
			response.Error(c, mapLogErr(err))
			return
		}
		defer func() { _ = rc.Close() }()

		// Clear the per-request write deadline so the 15s server WriteTimeout
		// doesn't terminate long-lived follows. Go 1.20+: no Hijacker needed.
		if rcCtl := http.NewResponseController(c.Writer); rcCtl != nil {
			if e := rcCtl.SetWriteDeadline(time.Time{}); e != nil {
				slog.WarnContext(ctx, "sse: cannot clear write deadline", "err", e)
			}
		}

		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.Flush()

		sc := bufio.NewScanner(rc)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		done := make(chan struct{})
		go func() {
			defer close(done)
			for sc.Scan() {
				if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", sc.Text()); err != nil {
					return // client gone
				}
				c.Writer.Flush()
			}
		}()

		ka := time.NewTicker(30 * time.Second)
		defer ka.Stop()
		for {
			select {
			case <-c.Request.Context().Done():
				return
			case <-done:
				return
			case <-ka.C:
				if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
					return
				}
				c.Writer.Flush()
			}
		}
	}
}

// mapLogErr maps "container not yet running"-style apiserver responses to
// CodeLogUnavailable so the FE can show a friendly "logs not ready yet"
// banner instead of a generic error toast. Everything else flows through the
// shared MapAPIError mapper.
func mapLogErr(err error) error {
	s := err.Error()
	for _, sub := range []string{
		"is waiting to start",
		"ContainerCreating",
		"PodInitializing",
		"previous terminated container",
	} {
		if strings.Contains(s, sub) {
			return apperr.New(apperr.CodeLogUnavailable, "k8s.log.unavailable", s)
		}
	}
	return k8serrs.MapAPIError(err)
}
