package secret

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	k8serrs "optimus-be/internal/modules/k8s/apierr"
)

// Clientsetter is the seam between this vertical and the k8s client.Factory.
// In production the implementation routes through audit + permission checks
// (see Task 15 wiring); in tests it is replaced with a fake that returns a
// preloaded fake.Clientset. The purpose string differs per method so audit
// can distinguish list/get (metadata-only) from reveal (payload disclosure).
type Clientsetter interface {
	Clientset(ctx context.Context, clusterID uint64, purpose string) (kubernetes.Interface, error)
}

// Service implements the Secret surface. No state beyond the Clientsetter —
// caching and audit happen one layer down inside the factory.
type Service struct{ cs Clientsetter }

// NewService constructs a Service bound to the given Clientsetter.
func NewService(cs Clientsetter) *Service { return &Service{cs: cs} }

// defaultLimit caps the default page size at 500 (apiserver soft default).
// Higher caller-requested limits up to 2000 are honoured; anything else
// falls back to the default to keep large clusters responsive.
const defaultLimit = 500

func opts(q ListQuery) metav1.ListOptions {
	l := q.Limit
	if l <= 0 || l > 2000 {
		l = defaultLimit
	}
	return metav1.ListOptions{Limit: l, Continue: q.Continue}
}

// List returns a page of Secrets in the requested namespace (or all
// namespaces if Namespace is empty). The response carries only metadata and
// key names — values never appear here. The apiserver Continue token is
// forwarded verbatim so the FE can resume paging.
func (s *Service) List(ctx context.Context, clusterID uint64, q ListQuery) (*ListResponse[Summary], error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.secret.list")
	if err != nil {
		return nil, err
	}
	out, err := cs.CoreV1().Secrets(q.Namespace).List(ctx, opts(q))
	if err != nil {
		return nil, k8serrs.MapAPIError(err)
	}
	items := make([]Summary, 0, len(out.Items))
	for _, x := range out.Items {
		items = append(items, toSummary(x))
	}
	return &ListResponse[Summary]{
		Items:     items,
		Continue:  out.Continue,
		Truncated: out.Continue != "",
	}, nil
}

// Get returns a single Secret's metadata + key list. Values are deliberately
// omitted; callers wanting the payload must hit Data instead, which the
// wiring layer gates behind a separate permission.
func (s *Service) Get(ctx context.Context, clusterID uint64, ns, name string) (*Detail, error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.secret.get")
	if err != nil {
		return nil, err
	}
	x, err := cs.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, k8serrs.MapAPIError(err)
	}
	out := toSummary(*x)
	return &out, nil
}

// Data returns the base64-decoded Secret payload. The purpose string is
// k8s.secret.reveal so audit can flag this distinctly from a metadata read.
// Wiring (Task 15) must gate this behind k8s:secret:reveal.
func (s *Service) Data(ctx context.Context, clusterID uint64, ns, name string) (*DataResponse, error) {
	cs, err := s.cs.Clientset(ctx, clusterID, "k8s.secret.reveal")
	if err != nil {
		return nil, err
	}
	x, err := cs.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, k8serrs.MapAPIError(err)
	}
	out := toData(*x)
	return &out, nil
}
