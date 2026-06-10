package network

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

// Service is the read-only API for the 2 network kinds (Service + Ingress).
// Holds no per-request state; safe to share across handlers.
type Service struct{ cs Clientsetter }

// NewService constructs a Service backed by the given Clientsetter.
func NewService(cs Clientsetter) *Service { return &Service{cs: cs} }

const (
	defaultLimit = 500
	maxLimit     = 2000
)

// opts builds the metav1.ListOptions used by every list call. Mirrors the
// workload vertical: zero / negative / oversized limits fall back to
// defaultLimit so a misbehaving client can't ask the apiserver for an
// unbounded page.
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
// CodeBadRequest with the "k8s.network.unsupported_kind" i18n key.
func (s *Service) List(ctx context.Context, clusterID uint64, kind string, q ListQuery) (any, error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.network.list")
	if err != nil {
		return nil, err
	}
	o := opts(q)
	switch kind {
	case "services":
		out, err := cs.CoreV1().Services(q.Namespace).List(ctx, o)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		items := make([]ServiceSummary, 0, len(out.Items))
		for _, x := range out.Items {
			items = append(items, toService(x))
		}
		return &ListResponse[ServiceSummary]{Items: items, Continue: out.Continue, Truncated: out.Continue != ""}, nil
	case "ingresses":
		out, err := cs.NetworkingV1().Ingresses(q.Namespace).List(ctx, o)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		items := make([]IngressSummary, 0, len(out.Items))
		for _, x := range out.Items {
			items = append(items, toIngress(x))
		}
		return &ListResponse[IngressSummary]{Items: items, Continue: out.Continue, Truncated: out.Continue != ""}, nil
	default:
		return nil, apperr.New(apperr.CodeBadRequest, "k8s.network.unsupported_kind",
			fmt.Sprintf("unsupported network kind %q", kind))
	}
}

// Get fetches a single named resource of the requested kind from the given
// namespace. Same `any` return contract as List — handler doesn't care.
// Unsupported kinds return CodeBadRequest + "k8s.network.unsupported_kind".
func (s *Service) Get(ctx context.Context, clusterID uint64, kind, ns, name string) (any, error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.network.get")
	if err != nil {
		return nil, err
	}
	g := metav1.GetOptions{}
	switch kind {
	case "services":
		// `x` (not `s`) — `s` would shadow the *Service receiver.
		x, err := cs.CoreV1().Services(ns).Get(ctx, name, g)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		out := toService(*x)
		return &out, nil
	case "ingresses":
		x, err := cs.NetworkingV1().Ingresses(ns).Get(ctx, name, g)
		if err != nil {
			return nil, k8serrs.MapAPIError(err)
		}
		out := toIngress(*x)
		return &out, nil
	default:
		return nil, apperr.New(apperr.CodeBadRequest, "k8s.network.unsupported_kind",
			fmt.Sprintf("unsupported network kind %q", kind))
	}
}
