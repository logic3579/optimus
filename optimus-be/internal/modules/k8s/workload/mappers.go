package workload

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// toDeployment projects an appsv1.Deployment into the FE-facing summary.
// Spec.Replicas may be nil (server-side defaulting) — default to 0 rather
// than panicking; status fields are non-pointer int32 so they're safe.
func toDeployment(d appsv1.Deployment) DeploymentSummary {
	desired := int32(0)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	return DeploymentSummary{
		Name:              d.Name,
		Namespace:         d.Namespace,
		ReplicasDesired:   desired,
		ReplicasReady:     d.Status.ReadyReplicas,
		ReplicasUpdated:   d.Status.UpdatedReplicas,
		ReplicasAvailable: d.Status.AvailableReplicas,
		Strategy:          string(d.Spec.Strategy.Type),
		Labels:            d.Labels,
		Age:               d.CreationTimestamp.Time,
	}
}

// toStatefulSet projects an appsv1.StatefulSet into the summary.
func toStatefulSet(s appsv1.StatefulSet) StatefulSetSummary {
	rep := int32(0)
	if s.Spec.Replicas != nil {
		rep = *s.Spec.Replicas
	}
	return StatefulSetSummary{
		Name:          s.Name,
		Namespace:     s.Namespace,
		Replicas:      rep,
		ReadyReplicas: s.Status.ReadyReplicas,
		ServiceName:   s.Spec.ServiceName,
		Labels:        s.Labels,
		Age:           s.CreationTimestamp.Time,
	}
}

// toDaemonSet projects an appsv1.DaemonSet into the summary. All counters
// live on .Status (no .Spec.Replicas equivalent for DaemonSets).
func toDaemonSet(d appsv1.DaemonSet) DaemonSetSummary {
	return DaemonSetSummary{
		Name:            d.Name,
		Namespace:       d.Namespace,
		DesiredNumber:   d.Status.DesiredNumberScheduled,
		CurrentNumber:   d.Status.CurrentNumberScheduled,
		ReadyNumber:     d.Status.NumberReady,
		AvailableNumber: d.Status.NumberAvailable,
		Misscheduled:    d.Status.NumberMisscheduled,
		Labels:          d.Labels,
		Age:             d.CreationTimestamp.Time,
	}
}

// toJob projects a batchv1.Job into the summary. StartTime/CompletionTime
// are *metav1.Time on the upstream type — funnel through timePtr so a zero
// or nil value becomes a JSON-omitted nil pointer.
func toJob(j batchv1.Job) JobSummary {
	comp := int32(0)
	if j.Spec.Completions != nil {
		comp = *j.Spec.Completions
	}
	return JobSummary{
		Name:        j.Name,
		Namespace:   j.Namespace,
		Completions: comp,
		Succeeded:   j.Status.Succeeded,
		Failed:      j.Status.Failed,
		StartTime:   timePtr(j.Status.StartTime),
		EndTime:     timePtr(j.Status.CompletionTime),
		Labels:      j.Labels,
		Age:         j.CreationTimestamp.Time,
	}
}

// toCronJob projects a batchv1.CronJob into the summary. Suspend is a
// *bool — treat nil as "not suspended" (the apiserver default).
func toCronJob(c batchv1.CronJob) CronJobSummary {
	return CronJobSummary{
		Name:             c.Name,
		Namespace:        c.Namespace,
		Schedule:         c.Spec.Schedule,
		LastScheduleTime: timePtr(c.Status.LastScheduleTime),
		ActiveJobs:       len(c.Status.Active),
		Suspended:        c.Spec.Suspend != nil && *c.Spec.Suspend,
		Labels:           c.Labels,
		Age:              c.CreationTimestamp.Time,
	}
}

// toReplicaSet projects an appsv1.ReplicaSet. The first OwnerReference (if
// any) is surfaced as OwnerKind/OwnerName so the FE can roll ReplicaSets up
// under their parent Deployment without a separate lookup.
func toReplicaSet(r appsv1.ReplicaSet) ReplicaSetSummary {
	rep := int32(0)
	if r.Spec.Replicas != nil {
		rep = *r.Spec.Replicas
	}
	out := ReplicaSetSummary{
		Name:          r.Name,
		Namespace:     r.Namespace,
		Replicas:      rep,
		ReadyReplicas: r.Status.ReadyReplicas,
		Labels:        r.Labels,
		Age:           r.CreationTimestamp.Time,
	}
	if len(r.OwnerReferences) > 0 {
		out.OwnerKind = r.OwnerReferences[0].Kind
		out.OwnerName = r.OwnerReferences[0].Name
	}
	return out
}

// toPod projects a corev1.Pod. ReadyContainers is the count of statuses
// with Ready==true (not len(ContainerStatuses)); RestartCount is the sum
// across all container statuses — both match `kubectl get pods` output.
func toPod(p corev1.Pod) PodSummary {
	ready := 0
	restarts := int32(0)
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
		restarts += cs.RestartCount
	}
	return PodSummary{
		Name:            p.Name,
		Namespace:       p.Namespace,
		Phase:           string(p.Status.Phase),
		ReadyContainers: ready,
		TotalContainers: len(p.Status.ContainerStatuses),
		RestartCount:    restarts,
		NodeName:        p.Spec.NodeName,
		PodIP:           p.Status.PodIP,
		StatusReason:    p.Status.Reason,
		Labels:          p.Labels,
		Age:             p.CreationTimestamp.Time,
	}
}

// timePtr accepts either a metav1.Time or *metav1.Time and returns a
// *time.Time (nil when the source is zero/nil). Job and CronJob status
// fields surface both shapes — funneling through one helper keeps the
// mappers terse and avoids per-call nil dances.
func timePtr(t any) *time.Time {
	switch v := t.(type) {
	case *metav1.Time:
		if v == nil || v.IsZero() {
			return nil
		}
		out := v.Time
		return &out
	case metav1.Time:
		if v.IsZero() {
			return nil
		}
		out := v.Time
		return &out
	}
	return nil
}
