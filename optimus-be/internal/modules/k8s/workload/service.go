package workload

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	apperr "optimus-be/internal/infra/errors"
	k8serrs "optimus-be/internal/modules/k8s/apierr"
)

// Clientsetter returns a fresh kubernetes.Interface for the given cluster.
// Defined locally (rather than importing client.Factory) so this package
// stays decoupled from the real wiring — tests inject an in-memory fake,
// the composition root in cmd/server/main.go injects *client.Factory.
type Clientsetter interface {
	Clientset(ctx context.Context, clusterID uint64, purpose string) (kubernetes.Interface, error)
}

// Service is the read-only API for the 7 workload kinds. Holds no per-request
// state; safe to share across handlers.
type Service struct{ cs Clientsetter }

// NewService constructs a Service backed by the given Clientsetter.
func NewService(cs Clientsetter) *Service { return &Service{cs: cs} }

const (
	defaultLimit = 500
	maxLimit     = 2000
)

// opts builds the metav1.ListOptions used by every list call. Mirrors the
// clusterscoped vertical's listOpts: zero / negative / oversized limits fall
// back to defaultLimit so a misbehaving client can't ask the apiserver for
// an unbounded page.
func opts(q ListQuery) metav1.ListOptions {
	l := q.Limit
	if l <= 0 || l > maxLimit {
		l = defaultLimit
	}
	return metav1.ListOptions{Limit: l, Continue: q.Continue}
}

// List is the kind-dispatching entry point. Returns one of the
// *ListResponse[T] types wrapped in `any` so the handler can encode without
// per-kind dispatch — the FE sees the same {items,continue,truncated}
// envelope regardless of which kind was requested. Unsupported kinds return
// CodeBadRequest with the "k8s.workload.unsupported_kind" i18n key.
func (s *Service) List(ctx context.Context, clusterID uint64, kind string, q ListQuery) (any, error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.workload.list")
	if err != nil {
		return nil, err
	}
	o := opts(q)
	switch kind {
	case "deployments":
		out, err := cs.AppsV1().Deployments(q.Namespace).List(ctx, o)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		items := make([]DeploymentSummary, 0, len(out.Items))
		for _, x := range out.Items {
			items = append(items, toDeployment(x))
		}
		return &ListResponse[DeploymentSummary]{Items: items, Continue: out.Continue, Truncated: out.Continue != ""}, nil
	case "statefulsets":
		out, err := cs.AppsV1().StatefulSets(q.Namespace).List(ctx, o)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		items := make([]StatefulSetSummary, 0, len(out.Items))
		for _, x := range out.Items {
			items = append(items, toStatefulSet(x))
		}
		return &ListResponse[StatefulSetSummary]{Items: items, Continue: out.Continue, Truncated: out.Continue != ""}, nil
	case "daemonsets":
		out, err := cs.AppsV1().DaemonSets(q.Namespace).List(ctx, o)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		items := make([]DaemonSetSummary, 0, len(out.Items))
		for _, x := range out.Items {
			items = append(items, toDaemonSet(x))
		}
		return &ListResponse[DaemonSetSummary]{Items: items, Continue: out.Continue, Truncated: out.Continue != ""}, nil
	case "jobs":
		out, err := cs.BatchV1().Jobs(q.Namespace).List(ctx, o)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		items := make([]JobSummary, 0, len(out.Items))
		for _, x := range out.Items {
			items = append(items, toJob(x))
		}
		return &ListResponse[JobSummary]{Items: items, Continue: out.Continue, Truncated: out.Continue != ""}, nil
	case "cronjobs":
		out, err := cs.BatchV1().CronJobs(q.Namespace).List(ctx, o)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		items := make([]CronJobSummary, 0, len(out.Items))
		for _, x := range out.Items {
			items = append(items, toCronJob(x))
		}
		return &ListResponse[CronJobSummary]{Items: items, Continue: out.Continue, Truncated: out.Continue != ""}, nil
	case "replicasets":
		out, err := cs.AppsV1().ReplicaSets(q.Namespace).List(ctx, o)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		items := make([]ReplicaSetSummary, 0, len(out.Items))
		for _, x := range out.Items {
			items = append(items, toReplicaSet(x))
		}
		return &ListResponse[ReplicaSetSummary]{Items: items, Continue: out.Continue, Truncated: out.Continue != ""}, nil
	case "pods":
		out, err := cs.CoreV1().Pods(q.Namespace).List(ctx, o)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		items := make([]PodSummary, 0, len(out.Items))
		for _, x := range out.Items {
			items = append(items, toPod(x))
		}
		return &ListResponse[PodSummary]{Items: items, Continue: out.Continue, Truncated: out.Continue != ""}, nil
	default:
		return nil, apperr.New(apperr.CodeBadRequest, "k8s.workload.unsupported_kind",
			fmt.Sprintf("unsupported workload kind %q", kind))
	}
}

// Get fetches a single named resource of the requested kind from the given
// namespace. Same `any` return contract as List — handler doesn't care.
// Unsupported kinds return CodeBadRequest + "k8s.workload.unsupported_kind".
func (s *Service) Get(ctx context.Context, clusterID uint64, kind, ns, name string) (any, error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.workload.get")
	if err != nil {
		return nil, err
	}
	g := metav1.GetOptions{}
	switch kind {
	case "deployments":
		x, err := cs.AppsV1().Deployments(ns).Get(ctx, name, g)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		out := toDeployment(*x)
		return &out, nil
	case "statefulsets":
		x, err := cs.AppsV1().StatefulSets(ns).Get(ctx, name, g)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		out := toStatefulSet(*x)
		return &out, nil
	case "daemonsets":
		x, err := cs.AppsV1().DaemonSets(ns).Get(ctx, name, g)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		out := toDaemonSet(*x)
		return &out, nil
	case "jobs":
		x, err := cs.BatchV1().Jobs(ns).Get(ctx, name, g)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		out := toJob(*x)
		return &out, nil
	case "cronjobs":
		x, err := cs.BatchV1().CronJobs(ns).Get(ctx, name, g)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		out := toCronJob(*x)
		return &out, nil
	case "replicasets":
		x, err := cs.AppsV1().ReplicaSets(ns).Get(ctx, name, g)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		out := toReplicaSet(*x)
		return &out, nil
	case "pods":
		x, err := cs.CoreV1().Pods(ns).Get(ctx, name, g)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		out := toPod(*x)
		return &out, nil
	default:
		return nil, apperr.New(apperr.CodeBadRequest, "k8s.workload.unsupported_kind",
			fmt.Sprintf("unsupported workload kind %q", kind))
	}
}
