// Package secret exposes the read-only Secret vertical of the k8s module.
// Unlike config, Secret has three methods: List + Get (name + meta only, no
// values) and Data (base64-decoded values). The wiring task gates Data behind
// a separate k8s:secret:reveal permission so a viewer can browse the surface
// without ever seeing the payload.
package secret

import (
	"time"

	"optimus-be/internal/modules/k8s/clusterscoped"
)

// Summary is the row projection used by both List and Get. It deliberately
// carries only names, type and key-list — never the raw values. Named
// Summary rather than SecretSummary to avoid the secret.SecretSummary stutter
// that revive flags.
type Summary struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Type      string            `json:"type"`
	DataKeys  []string          `json:"data_keys"`
	DataCount int               `json:"data_count"`
	Labels    map[string]string `json:"labels,omitempty"`
	Age       time.Time         `json:"age"`
}

// Detail is the same shape as Summary — Get does not reveal any extra fields
// vs. List for Secrets (values only ever surface via Data). Aliased rather
// than embedded so the FE can use the same renderer.
type Detail = Summary

// DataResponse carries the base64-decoded Secret payload. UTF-8 values are
// emitted as plain strings; non-UTF-8 values are wrapped as
// {"value": "<base64>", "base64": true} so the FE can render them as binary
// without us speculatively decoding bytes that aren't actually text.
type DataResponse struct {
	Data map[string]any `json:"data"`
}

// ListResponse aliases the shared envelope from clusterscoped so the FE can
// use a single generic table component across every vertical.
type ListResponse[T any] = clusterscoped.ListResponse[T]

// ListQuery is the shared query-string struct (namespace + limit + continue).
type ListQuery = clusterscoped.ListQuery
