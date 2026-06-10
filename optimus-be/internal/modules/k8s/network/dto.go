// Package network exposes read-only handlers for the namespaced Kubernetes
// network kinds in the P2 catalogue: Service and Ingress. The two kinds are
// dispatched by the `kind` path parameter so the URL shape mirrors the
// workload vertical:
//
//	GET /k8s/clusters/:id/network/:kind
//	GET /k8s/clusters/:id/network/:kind/:ns/:name
//
// The generic ListResponse[T] envelope and ListQuery are aliased from the
// clusterscoped vertical so the FE sees the exact same shape regardless of
// which resource vertical produced the response.
package network

import (
	"time"

	"optimus-be/internal/modules/k8s/clusterscoped"
)

// ServiceSummary is the JSON projection of a corev1.Service.
type ServiceSummary struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Type        string            `json:"type"`
	ClusterIP   string            `json:"cluster_ip"`
	ExternalIPs []string          `json:"external_ips,omitempty"`
	Ports       []ServicePort     `json:"ports"`
	Selector    map[string]string `json:"selector,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Age         time.Time         `json:"age"`
}

// ServicePort is the JSON projection of a corev1.ServicePort. TargetPort is
// stringified (it can be either an int or a named port) so the FE can render
// it without re-implementing intstr.IntOrString.
type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"target_port"`
	Protocol   string `json:"protocol"`
	NodePort   int32  `json:"node_port,omitempty"`
}

// IngressSummary is the JSON projection of a netv1.Ingress. Hosts is the
// flattened rule.host list; LoadBalancerIPs flattens status.loadBalancer.
// ingress entries (both IPs and hostnames) so the FE only renders one column.
type IngressSummary struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	IngressClass    string            `json:"ingress_class,omitempty"`
	Hosts           []string          `json:"hosts"`
	LoadBalancerIPs []string          `json:"load_balancer_ips,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Age             time.Time         `json:"age"`
}

// ListResponse aliases the generic envelope defined in the clusterscoped
// vertical so every resource vertical returns the exact same JSON shape.
type ListResponse[T any] = clusterscoped.ListResponse[T]

// ListQuery aliases the shared query-string struct — same form tags, same
// pagination semantics as the rest of the k8s read API.
type ListQuery = clusterscoped.ListQuery
