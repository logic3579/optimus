package config

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	k8serrs "optimus-be/internal/modules/k8s"
)

// Clientsetter is the seam between this vertical and the k8s client.Factory.
// In production the implementation routes through audit + permission checks
// (see Task 15 wiring); in tests it is replaced with a fake that returns a
// preloaded fake.Clientset.
type Clientsetter interface {
	Clientset(ctx context.Context, clusterID uint64, purpose string) (kubernetes.Interface, error)
}

// Service implements the read-only ConfigMap surface. No state beyond the
// Clientsetter — caching and audit happen one layer down inside the factory.
type Service struct{ cs Clientsetter }

// NewService constructs a Service bound to the given Clientsetter.
func NewService(cs Clientsetter) *Service { return &Service{cs: cs} }

// defaultLimit caps the default page size at 500 (apiserver soft default).
// A higher cap (2000) is honoured if the caller asks for it but anything
// out of range falls back to the default to keep large clusters responsive.
const defaultLimit = 500

func opts(q ListQuery) metav1.ListOptions {
	l := q.Limit
	if l <= 0 || l > 2000 {
		l = defaultLimit
	}
	return metav1.ListOptions{Limit: l, Continue: q.Continue}
}

// List returns a page of ConfigMaps in the requested namespace (or all
// namespaces if Namespace is empty). The apiserver Continue token is
// forwarded verbatim on the response so the FE can resume paging.
func (s *Service) List(ctx context.Context, clusterID uint64, q ListQuery) (*ListResponse[MapSummary], error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.config.list")
	if err != nil {
		return nil, err
	}
	out, err := cs.CoreV1().ConfigMaps(q.Namespace).List(ctx, opts(q))
	if err != nil {
		return nil, k8serrs.MapAPIError(err)
	}
	items := make([]MapSummary, 0, len(out.Items))
	for _, x := range out.Items {
		items = append(items, toSummary(x))
	}
	return &ListResponse[MapSummary]{
		Items:     items,
		Continue:  out.Continue,
		Truncated: out.Continue != "",
	}, nil
}

// Get returns the full ConfigMap including string Data. Binary entries are
// surfaced as a key list only (see toDetail).
func (s *Service) Get(ctx context.Context, clusterID uint64, ns, name string) (*MapDetail, error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.config.get")
	if err != nil {
		return nil, err
	}
	x, err := cs.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, k8serrs.MapAPIError(err)
	}
	out := toDetail(*x)
	return &out, nil
}
