package network

import (
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
)

// toService projects a corev1.Service onto the JSON DTO. TargetPort is
// stringified via intstr.IntOrString.String() so numeric and named target
// ports both serialise as strings (FE doesn't need to switch on type).
func toService(s corev1.Service) ServiceSummary {
	ports := make([]ServicePort, 0, len(s.Spec.Ports))
	for _, p := range s.Spec.Ports {
		ports = append(ports, ServicePort{
			Name:       p.Name,
			Port:       p.Port,
			TargetPort: p.TargetPort.String(),
			Protocol:   string(p.Protocol),
			NodePort:   p.NodePort,
		})
	}
	return ServiceSummary{
		Name:        s.Name,
		Namespace:   s.Namespace,
		Type:        string(s.Spec.Type),
		ClusterIP:   s.Spec.ClusterIP,
		ExternalIPs: s.Spec.ExternalIPs,
		Ports:       ports,
		Selector:    s.Spec.Selector,
		Labels:      s.Labels,
		Age:         s.CreationTimestamp.Time,
	}
}

// toIngress projects a netv1.Ingress onto the JSON DTO. Rule hosts are
// flattened (empty hosts skipped) and status load-balancer entries are
// flattened to a single string slice — IP and Hostname are both surfaced
// because cloud LBs publish one or the other.
func toIngress(i netv1.Ingress) IngressSummary {
	hosts := []string{}
	for _, r := range i.Spec.Rules {
		if r.Host != "" {
			hosts = append(hosts, r.Host)
		}
	}
	lbIPs := []string{}
	for _, ing := range i.Status.LoadBalancer.Ingress {
		if ing.IP != "" {
			lbIPs = append(lbIPs, ing.IP)
		}
		if ing.Hostname != "" {
			lbIPs = append(lbIPs, ing.Hostname)
		}
	}
	cls := ""
	if i.Spec.IngressClassName != nil {
		cls = *i.Spec.IngressClassName
	}
	return IngressSummary{
		Name:            i.Name,
		Namespace:       i.Namespace,
		IngressClass:    cls,
		Hosts:           hosts,
		LoadBalancerIPs: lbIPs,
		Labels:          i.Labels,
		Age:             i.CreationTimestamp.Time,
	}
}
