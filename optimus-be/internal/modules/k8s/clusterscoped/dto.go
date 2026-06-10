// Package clusterscoped exposes read-only handlers for the three
// cluster-scoped Kubernetes kinds the P2 catalogue calls out: Namespace,
// Node, and Event. It is the canonical resource-vertical shape — subsequent
// verticals (workload, network, config, secret) follow the same DTO →
// Service → Handler layout and reuse the generic ListResponse[T] envelope
// defined here.
package clusterscoped

import "time"

// NamespaceSummary is the JSON projection of a corev1.Namespace returned to
// the FE list view.
type NamespaceSummary struct {
	Name   string            `json:"name"`
	Phase  string            `json:"phase"`
	Labels map[string]string `json:"labels,omitempty"`
	Age    time.Time         `json:"age"`
}

// NodeSummary is the JSON projection of a corev1.Node for list + detail.
type NodeSummary struct {
	Name           string            `json:"name"`
	Ready          bool              `json:"ready"`
	Schedulable    bool              `json:"schedulable"`
	Roles          []string          `json:"roles"`
	KubeletVersion string            `json:"kubelet_version"`
	CPUCapacity    string            `json:"cpu_capacity"`
	MemCapacity    string            `json:"mem_capacity"`
	Labels         map[string]string `json:"labels,omitempty"`
	Age            time.Time         `json:"age"`
}

// EventSummary is the JSON projection of a corev1.Event.
type EventSummary struct {
	Namespace      string    `json:"namespace,omitempty"`
	Type           string    `json:"type"`
	Reason         string    `json:"reason"`
	Message        string    `json:"message"`
	InvolvedKind   string    `json:"involved_kind"`
	InvolvedName   string    `json:"involved_name"`
	Count          int32     `json:"count"`
	FirstTimestamp time.Time `json:"first_timestamp"`
	LastTimestamp  time.Time `json:"last_timestamp"`
}

// ListResponse is the generic list envelope used by every resource vertical
// in P2. See spec §5.3. Continue is the apiserver opaque pagination token
// (empty when the listing is complete); Truncated mirrors Continue != "" so
// the FE can render a "load more" affordance without re-checking the token.
type ListResponse[T any] struct {
	Items     []T    `json:"items"`
	Continue  string `json:"continue,omitempty"`
	Truncated bool   `json:"truncated"`
}

// ListQuery is the shared query-string struct bound by every list handler.
// Namespace is only honoured by namespaced kinds (e.g. Events here, and the
// workload / network / config / secret verticals downstream).
type ListQuery struct {
	Namespace string `form:"namespace"`
	Limit     int64  `form:"limit"`
	Continue  string `form:"continue"`
}
