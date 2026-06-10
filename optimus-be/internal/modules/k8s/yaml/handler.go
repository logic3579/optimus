// Package yaml implements the universal YAML endpoint
// GET /api/v1/k8s/clusters/:id/yaml?kind=<k>&namespace=<ns>&name=<n>.
//
// Unlike the typed read endpoints in the workload/network/config/secret/
// clusterscoped packages, this handler dispatches on the ?kind= query param
// and looks up the required permission code in the KindPerm map. The
// permission check therefore happens inside the handler — not in route
// middleware — because the per-route RequirePermission gate cannot know the
// permission until it has parsed the query string.
//
// For Secret: the boundary is k8s:secret:read (NOT :reveal). Returning the
// full object as YAML inevitably exposes base64-encoded data; spec §5.6 and
// §11.5 document that an operator with cluster YAML access can decode the
// payload offline anyway, so a separate :reveal permission would not change
// the effective exposure.
package yaml

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	sigsyaml "sigs.k8s.io/yaml"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/infra/response"
	k8serrs "optimus-be/internal/modules/k8s"
	"optimus-be/internal/modules/rbac"
)

// KindPerm maps supported ?kind= values to the permission code required.
// Keep in sync with the typed endpoint perm gates:
//   - workload kinds → k8s:workload:read
//   - service/ingress → k8s:network:read
//   - configmap      → k8s:config:read
//   - secret         → k8s:secret:read (see package doc — NOT :reveal)
//   - namespace/node/event → k8s:cluster_resource:read
var KindPerm = map[string]string{
	"deployment":  "k8s:workload:read",
	"statefulset": "k8s:workload:read",
	"daemonset":   "k8s:workload:read",
	"pod":         "k8s:workload:read",
	"job":         "k8s:workload:read",
	"cronjob":     "k8s:workload:read",
	"replicaset":  "k8s:workload:read",
	"service":     "k8s:network:read",
	"ingress":     "k8s:network:read",
	"configmap":   "k8s:config:read",
	"secret":      "k8s:secret:read",
	"namespace":   "k8s:cluster_resource:read",
	"node":        "k8s:cluster_resource:read",
	"event":       "k8s:cluster_resource:read",
}

// Clientsetter mirrors the seam used by the other k8s sub-packages so tests
// can substitute a fake clientset without dragging in k8s/client.Factory.
type Clientsetter interface {
	Clientset(ctx context.Context, clusterID uint64, purpose string) (kubernetes.Interface, error)
}

// Handler owns the Clientsetter (for upstream calls) and the rbac cache (for
// the in-handler permission check).
type Handler struct {
	cs    Clientsetter
	cache *rbac.PermissionCache
}

// NewHandler constructs a Handler. Both deps are required at runtime; tests
// for unhappy paths that reject before consulting the cache may pass nil.
func NewHandler(cs Clientsetter, cache *rbac.PermissionCache) *Handler {
	return &Handler{cs: cs, cache: cache}
}

// Get handles GET /k8s/clusters/:id/yaml. Validates inputs, performs the
// dynamic permission check, fetches the typed object via the kind dispatch
// table, and returns it as text/yaml.
func (h *Handler) Get() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid cluster id"))
			return
		}
		kind := c.Query("kind")
		ns := c.Query("namespace")
		name := c.Query("name")
		if kind == "" || name == "" {
			response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "kind and name required"))
			return
		}
		perm, ok := KindPerm[kind]
		if !ok {
			response.Error(c, apperr.New(apperr.CodeBadRequest, "k8s.yaml.unsupported_kind",
				"unsupported kind: "+kind))
			return
		}

		// In-handler permission check: route-level middleware can't see
		// the ?kind= value, so we resolve it here against the same cache
		// that RequirePermission uses on the typed endpoints.
		uid := c.GetUint64(middleware.CtxKeyUserID)
		codes, err := h.cache.Get(c.Request.Context(), uid)
		if err != nil {
			response.Error(c, err)
			return
		}
		has := false
		for _, p := range codes {
			if p == perm {
				has = true
				break
			}
		}
		if !has {
			response.Error(c, apperr.New(apperr.CodePermissionDenied, "auth.permission_denied", "permission denied"))
			return
		}

		cs, err := h.cs.Clientset(c.Request.Context(), id, "k8s.yaml.get")
		if err != nil {
			response.Error(c, err)
			return
		}
		obj, err := fetch(c.Request.Context(), cs, kind, ns, name)
		if err != nil {
			response.Error(c, k8serrs.MapAPIError(err))
			return
		}
		b, err := sigsyaml.Marshal(obj)
		if err != nil {
			response.Error(c, apperr.Wrap(err, apperr.CodeInternal, "common.internal", "yaml marshal failed"))
			return
		}
		c.Data(http.StatusOK, "text/yaml", b)
	}
}

// fetch dispatches to the typed client-go getter for the given kind. The
// returned `any` is then handed to sigs.k8s.io/yaml for marshalling — which
// reflects on the embedded ObjectMeta/TypeMeta to emit canonical YAML.
func fetch(ctx context.Context, cs kubernetes.Interface, kind, ns, name string) (any, error) {
	g := metav1.GetOptions{}
	switch kind {
	case "deployment":
		return cs.AppsV1().Deployments(ns).Get(ctx, name, g)
	case "statefulset":
		return cs.AppsV1().StatefulSets(ns).Get(ctx, name, g)
	case "daemonset":
		return cs.AppsV1().DaemonSets(ns).Get(ctx, name, g)
	case "replicaset":
		return cs.AppsV1().ReplicaSets(ns).Get(ctx, name, g)
	case "pod":
		return cs.CoreV1().Pods(ns).Get(ctx, name, g)
	case "job":
		return cs.BatchV1().Jobs(ns).Get(ctx, name, g)
	case "cronjob":
		return cs.BatchV1().CronJobs(ns).Get(ctx, name, g)
	case "service":
		return cs.CoreV1().Services(ns).Get(ctx, name, g)
	case "ingress":
		return cs.NetworkingV1().Ingresses(ns).Get(ctx, name, g)
	case "configmap":
		return cs.CoreV1().ConfigMaps(ns).Get(ctx, name, g)
	case "secret":
		return cs.CoreV1().Secrets(ns).Get(ctx, name, g)
	case "namespace":
		return cs.CoreV1().Namespaces().Get(ctx, name, g)
	case "node":
		return cs.CoreV1().Nodes().Get(ctx, name, g)
	case "event":
		return cs.CoreV1().Events(ns).Get(ctx, name, g)
	}
	return nil, apperr.New(apperr.CodeBadRequest, "k8s.yaml.unsupported_kind", "unsupported kind: "+kind)
}
