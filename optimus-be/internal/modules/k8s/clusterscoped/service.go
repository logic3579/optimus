package clusterscoped

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	k8serrs "optimus-be/internal/modules/k8s"
)

// Clientsetter returns a fresh kubernetes.Interface for the given cluster.
// Defined locally (rather than importing client.Factory) so this package
// stays decoupled from the real wiring — tests inject an in-memory fake,
// the composition root in cmd/server/main.go injects *client.Factory.
type Clientsetter interface {
	Clientset(ctx context.Context, clusterID uint64, purpose string) (kubernetes.Interface, error)
}

// Service is the read-only API for the three cluster-scoped kinds. Holds no
// per-request state; safe to share across handlers.
type Service struct{ cs Clientsetter }

// NewService constructs a Service backed by the given Clientsetter.
func NewService(cs Clientsetter) *Service { return &Service{cs: cs} }

const (
	defaultLimit = 500
	maxLimit     = 2000

	nodeRolePrefix = "node-role.kubernetes.io/"
)

// clamp normalises a caller-supplied limit. Zero / negative / oversized
// values fall back to defaultLimit so a misbehaving client can't ask the
// apiserver for an unbounded page.
func clamp(limit int64) int64 {
	if limit <= 0 || limit > maxLimit {
		return defaultLimit
	}
	return limit
}

// listOpts builds the metav1.ListOptions used by every list call — passes
// the apiserver continue token straight through for cursor pagination.
func listOpts(q ListQuery) metav1.ListOptions {
	return metav1.ListOptions{Limit: clamp(q.Limit), Continue: q.Continue}
}

// ---- Namespace -------------------------------------------------------------

// ListNamespaces returns every Namespace in the cluster, paginated by the
// apiserver continue token.
func (s *Service) ListNamespaces(ctx context.Context, clusterID uint64, q ListQuery) (*ListResponse[NamespaceSummary], error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.cluster_resource.list")
	if err != nil {
		return nil, err
	}
	out, err := cs.CoreV1().Namespaces().List(ctx, listOpts(q))
	if err != nil {
		return nil, k8serrs.MapAPIError(err)
	}
	items := make([]NamespaceSummary, 0, len(out.Items))
	for _, n := range out.Items {
		items = append(items, NamespaceSummary{
			Name:   n.Name,
			Phase:  string(n.Status.Phase),
			Labels: n.Labels,
			Age:    n.CreationTimestamp.Time,
		})
	}
	return &ListResponse[NamespaceSummary]{
		Items:     items,
		Continue:  out.Continue,
		Truncated: out.Continue != "",
	}, nil
}

// ---- Node ------------------------------------------------------------------

// ListNodes returns every Node in the cluster.
func (s *Service) ListNodes(ctx context.Context, clusterID uint64, q ListQuery) (*ListResponse[NodeSummary], error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.cluster_resource.list")
	if err != nil {
		return nil, err
	}
	out, err := cs.CoreV1().Nodes().List(ctx, listOpts(q))
	if err != nil {
		return nil, k8serrs.MapAPIError(err)
	}
	items := make([]NodeSummary, 0, len(out.Items))
	for _, n := range out.Items {
		items = append(items, toNodeSummary(n))
	}
	return &ListResponse[NodeSummary]{
		Items:     items,
		Continue:  out.Continue,
		Truncated: out.Continue != "",
	}, nil
}

// GetNode returns the single named Node.
func (s *Service) GetNode(ctx context.Context, clusterID uint64, name string) (*NodeSummary, error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.cluster_resource.get")
	if err != nil {
		return nil, err
	}
	n, err := cs.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, k8serrs.MapAPIError(err)
	}
	out := toNodeSummary(*n)
	return &out, nil
}

// toNodeSummary projects a corev1.Node into the FE-facing summary. Ready is
// derived from the NodeReady condition; roles are extracted by walking the
// standard `node-role.kubernetes.io/<role>` label prefix (no special-case
// for control-plane vs master — both surface as whatever the cluster set).
func toNodeSummary(n corev1.Node) NodeSummary {
	ready := false
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			ready = true
			break
		}
	}
	roles := []string{}
	for k := range n.Labels {
		if strings.HasPrefix(k, nodeRolePrefix) {
			roles = append(roles, strings.TrimPrefix(k, nodeRolePrefix))
		}
	}
	return NodeSummary{
		Name:           n.Name,
		Ready:          ready,
		Schedulable:    !n.Spec.Unschedulable,
		Roles:          roles,
		KubeletVersion: n.Status.NodeInfo.KubeletVersion,
		CPUCapacity:    n.Status.Capacity.Cpu().String(),
		MemCapacity:    n.Status.Capacity.Memory().String(),
		Labels:         n.Labels,
		Age:            n.CreationTimestamp.Time,
	}
}

// ---- Event -----------------------------------------------------------------

// ListEvents returns Events from the requested namespace; empty Namespace
// lists across all namespaces (matches the typed clientset semantics).
func (s *Service) ListEvents(ctx context.Context, clusterID uint64, q ListQuery) (*ListResponse[EventSummary], error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.cluster_resource.list")
	if err != nil {
		return nil, err
	}
	out, err := cs.CoreV1().Events(q.Namespace).List(ctx, listOpts(q))
	if err != nil {
		return nil, k8serrs.MapAPIError(err)
	}
	items := make([]EventSummary, 0, len(out.Items))
	for _, e := range out.Items {
		items = append(items, EventSummary{
			Namespace:      e.Namespace,
			Type:           e.Type,
			Reason:         e.Reason,
			Message:        e.Message,
			InvolvedKind:   e.InvolvedObject.Kind,
			InvolvedName:   e.InvolvedObject.Name,
			Count:          e.Count,
			FirstTimestamp: e.FirstTimestamp.Time,
			LastTimestamp:  e.LastTimestamp.Time,
		})
	}
	return &ListResponse[EventSummary]{
		Items:     items,
		Continue:  out.Continue,
		Truncated: out.Continue != "",
	}, nil
}
