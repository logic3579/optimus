package event

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
)

const readPermission = "delivery:run:read"

const (
	defaultPollInterval   = time.Second
	defaultStreamDuration = 24 * time.Hour
	maxCursorBytes        = 20
)

type Handler struct {
	service        *Service
	heartbeat      time.Duration
	pollInterval   time.Duration
	streamDuration time.Duration
	connections    chan struct{}
}

func NewHandler(service *Service, heartbeat time.Duration, maxConnections int) *Handler {
	if heartbeat <= 0 {
		heartbeat = 20 * time.Second
	}
	if maxConnections <= 0 {
		maxConnections = 1
	}
	return &Handler{service: service, heartbeat: heartbeat, pollInterval: defaultPollInterval,
		streamDuration: defaultStreamDuration, connections: make(chan struct{}, maxConnections)}
}

// Mount keeps ownership of the global run-read permission at the route
// boundary. The event service deliberately does not invent a resource ACL.
func (h *Handler) Mount(group *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	read := group.Group("", permission(readPermission))
	read.GET("/runs/:id/events", h.Stream)
}

// Stream godoc
// @Summary Stream delivery run events
// @Tags delivery
// @Security BearerAuth
// @Produce text/event-stream
// @Param id path int true "run ID"
// @Param cursor query int false "event cursor"
// @Param Last-Event-ID header string false "event cursor"
// @Success 200 {string} string "authenticated delivery event stream"
// @Router /delivery/runs/{id}/events [get]
func (h *Handler) Stream(c *gin.Context) {
	select {
	case h.connections <- struct{}{}:
		defer func() { <-h.connections }()
	default:
		response.Error(c, apperr.New(apperr.CodeRateLimited, "common.rate_limited", "delivery event stream limit reached"))
		return
	}
	runID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || runID == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid run id"))
		return
	}
	cursor, err := parseCursor(c)
	if err != nil {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid event cursor"))
		return
	}
	if err := h.service.ValidateRun(c.Request.Context(), runID); err != nil {
		response.Error(c, err)
		return
	}
	prefetched, err := h.service.ReadAfter(c.Request.Context(), runID, cursor)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "delivery sse: initial event page unavailable")
		response.Error(c, apperr.New(apperr.CodeInternal, "common.internal", "delivery event stream unavailable"))
		return
	}

	controller := http.NewResponseController(c.Writer)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil && !strings.Contains(err.Error(), "not supported") {
		slog.WarnContext(c.Request.Context(), "delivery sse: cannot clear write deadline", "err", err)
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	heartbeat := time.NewTicker(h.heartbeat)
	poll := time.NewTicker(h.pollInterval)
	expires := time.NewTimer(h.streamDuration)
	defer heartbeat.Stop()
	defer poll.Stop()
	defer expires.Stop()
	if next, ok := emitEvents(c, cursor, prefetched); !ok {
		return
	} else {
		cursor = next
	}
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-expires.C:
			return
		case <-poll.C:
			next, ok := h.emitPage(c, runID, cursor)
			if !ok {
				return
			}
			cursor = next
		case <-heartbeat.C:
			if _, err := fmt.Fprint(c.Writer, ": heartbeat\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}

func (h *Handler) emitPage(c *gin.Context, runID, cursor uint64) (uint64, bool) {
	events, err := h.service.ReadAfter(c.Request.Context(), runID, cursor)
	if err != nil {
		return cursor, false
	}
	return emitEvents(c, cursor, events)
}

func emitEvents(c *gin.Context, cursor uint64, events []Event) (uint64, bool) {
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil || len(data) > maxEventBytes {
			return cursor, false
		}
		if _, err := fmt.Fprintf(c.Writer, "id: %d\nevent: delivery\ndata: %s\n\n", event.ID, data); err != nil {
			return cursor, false
		}
		c.Writer.Flush()
		cursor = event.ID
	}
	return cursor, true
}

func parseCursor(c *gin.Context) (uint64, error) {
	raw := strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("cursor"))
	}
	if raw == "" {
		return 0, nil
	}
	if len(raw) > maxCursorBytes {
		return 0, strconv.ErrRange
	}
	return strconv.ParseUint(raw, 10, 64)
}
