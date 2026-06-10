// Package workload exposes read-only handlers for the 7 workload-style
// Kubernetes kinds the P2 catalogue calls out: Deployment, StatefulSet,
// DaemonSet, Job, CronJob, ReplicaSet, and Pod. Unlike the clusterscoped
// vertical (which has one handler per kind), all 7 are dispatched by the
// `kind` path parameter so the URL shape is uniform:
//
//	GET /k8s/clusters/:id/workloads/:kind
//	GET /k8s/clusters/:id/workloads/:kind/:ns/:name
//
// The generic ListResponse[T] envelope and ListQuery are aliased from the
// clusterscoped vertical so the FE sees the exact same shape regardless of
// which resource vertical produced the response.
package workload

import (
	"time"

	"optimus-be/internal/modules/k8s/clusterscoped"
)

// DeploymentSummary is the JSON projection of an appsv1.Deployment.
type DeploymentSummary struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	ReplicasDesired   int32             `json:"replicas_desired"`
	ReplicasReady     int32             `json:"replicas_ready"`
	ReplicasUpdated   int32             `json:"replicas_updated"`
	ReplicasAvailable int32             `json:"replicas_available"`
	Strategy          string            `json:"strategy"`
	Labels            map[string]string `json:"labels,omitempty"`
	Age               time.Time         `json:"age"`
}

// StatefulSetSummary is the JSON projection of an appsv1.StatefulSet.
type StatefulSetSummary struct {
	Name          string            `json:"name"`
	Namespace     string            `json:"namespace"`
	Replicas      int32             `json:"replicas"`
	ReadyReplicas int32             `json:"ready_replicas"`
	ServiceName   string            `json:"service_name"`
	Labels        map[string]string `json:"labels,omitempty"`
	Age           time.Time         `json:"age"`
}

// DaemonSetSummary is the JSON projection of an appsv1.DaemonSet.
type DaemonSetSummary struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	DesiredNumber   int32             `json:"desired_number"`
	CurrentNumber   int32             `json:"current_number"`
	ReadyNumber     int32             `json:"ready_number"`
	AvailableNumber int32             `json:"available_number"`
	Misscheduled    int32             `json:"misscheduled"`
	Labels          map[string]string `json:"labels,omitempty"`
	Age             time.Time         `json:"age"`
}

// JobSummary is the JSON projection of a batchv1.Job.
type JobSummary struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Completions int32             `json:"completions"`
	Succeeded   int32             `json:"succeeded"`
	Failed      int32             `json:"failed"`
	StartTime   *time.Time        `json:"start_time,omitempty"`
	EndTime     *time.Time        `json:"end_time,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Age         time.Time         `json:"age"`
}

// CronJobSummary is the JSON projection of a batchv1.CronJob.
type CronJobSummary struct {
	Name             string            `json:"name"`
	Namespace        string            `json:"namespace"`
	Schedule         string            `json:"schedule"`
	LastScheduleTime *time.Time        `json:"last_schedule_time,omitempty"`
	ActiveJobs       int               `json:"active_jobs"`
	Suspended        bool              `json:"suspended"`
	Labels           map[string]string `json:"labels,omitempty"`
	Age              time.Time         `json:"age"`
}

// ReplicaSetSummary is the JSON projection of an appsv1.ReplicaSet. The
// controller `OwnerKind`/`OwnerName` (first owner reference) is surfaced so
// the FE can group ReplicaSets under their Deployment without a second call.
type ReplicaSetSummary struct {
	Name          string            `json:"name"`
	Namespace     string            `json:"namespace"`
	Replicas      int32             `json:"replicas"`
	ReadyReplicas int32             `json:"ready_replicas"`
	OwnerKind     string            `json:"owner_kind,omitempty"`
	OwnerName     string            `json:"owner_name,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Age           time.Time         `json:"age"`
}

// PodSummary is the JSON projection of a corev1.Pod. ReadyContainers counts
// `cs.Ready == true` (not just len(ContainerStatuses)) and RestartCount is
// summed across containers — the FE displays both as "ready/total" and
// "restarts" columns.
type PodSummary struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	Phase           string            `json:"phase"`
	ReadyContainers int               `json:"ready_containers"`
	TotalContainers int               `json:"total_containers"`
	RestartCount    int32             `json:"restart_count"`
	NodeName        string            `json:"node_name"`
	PodIP           string            `json:"pod_ip"`
	StatusReason    string            `json:"status_reason,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Age             time.Time         `json:"age"`
}

// ListResponse aliases the generic envelope defined in the clusterscoped
// vertical so every resource vertical returns the exact same JSON shape.
type ListResponse[T any] = clusterscoped.ListResponse[T]

// ListQuery aliases the shared query-string struct — same form tags, same
// pagination semantics as the rest of the k8s read API.
type ListQuery = clusterscoped.ListQuery
