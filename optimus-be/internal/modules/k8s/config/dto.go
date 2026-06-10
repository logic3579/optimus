// Package config exposes the read-only ConfigMap vertical of the k8s module.
// Unlike the workload / network verticals this is a single-kind surface — the
// handlers do not switch on a path :kind segment. The DTO shape mirrors the
// other verticals (Summary on list, Detail on get) so the shared FE table /
// drawer components can render it without per-kind branches.
package config

import (
	"time"

	"optimus-be/internal/modules/k8s/clusterscoped"
)

// MapSummary is the row projection returned by List. DataKeys and DataCount
// cover both string Data and BinaryData (binary entries appear in the key
// list but never expose their payload here — Detail surfaces those via
// BinaryKeys instead). Named MapSummary rather than ConfigMapSummary to
// avoid the config.ConfigMap... stutter that revive flags.
type MapSummary struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	DataKeys  []string          `json:"data_keys"`
	DataCount int               `json:"data_count"`
	Labels    map[string]string `json:"labels,omitempty"`
	Age       time.Time         `json:"age"`
}

// MapDetail is returned by Get. Data carries the full string payload
// (ConfigMaps are not secrets — their content is fine to return on a read).
// BinaryKeys lists the names of any BinaryData entries so the FE can render
// a "binary, not shown" placeholder without us having to base64 the bytes.
type MapDetail struct {
	MapSummary
	Data       map[string]string `json:"data"`
	BinaryKeys []string          `json:"binary_keys,omitempty"`
}

// ListResponse and ListQuery alias the shared envelope from clusterscoped so
// the FE can use a single generic table component across every vertical.
type ListResponse[T any] = clusterscoped.ListResponse[T]

// ListQuery is the shared query-string struct (namespace + limit + continue).
type ListQuery = clusterscoped.ListQuery
