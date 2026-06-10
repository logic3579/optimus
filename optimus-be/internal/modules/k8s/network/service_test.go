package network_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/k8s/network"
)

// fakeCS satisfies network.Clientsetter by returning a preloaded in-memory
// fake clientset. The clusterID / purpose args are ignored — the tests
// don't exercise the routing/audit seam, only the projection logic.
type fakeCS struct{ cs kubernetes.Interface }

func (f *fakeCS) Clientset(context.Context, uint64, string) (kubernetes.Interface, error) {
	return f.cs, nil
}

func TestList_Services(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.1",
			Ports:     []corev1.ServicePort{{Port: 80, Protocol: "TCP"}},
		},
	})
	svc := network.NewService(&fakeCS{cs: cs})
	out, err := svc.List(context.Background(), 1, "services", network.ListQuery{})
	require.NoError(t, err)
	lr := out.(*network.ListResponse[network.ServiceSummary])
	require.Len(t, lr.Items, 1)
	require.Equal(t, "10.0.0.1", lr.Items[0].ClusterIP)
	require.Equal(t, "ClusterIP", lr.Items[0].Type)
	require.Equal(t, int32(80), lr.Items[0].Ports[0].Port)
	require.Equal(t, "TCP", lr.Items[0].Ports[0].Protocol)
}

func TestList_Ingresses(t *testing.T) {
	cls := "nginx"
	cs := fake.NewSimpleClientset(&netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "i", Namespace: "n"},
		Spec: netv1.IngressSpec{
			IngressClassName: &cls,
			Rules:            []netv1.IngressRule{{Host: "example.com"}},
		},
		Status: netv1.IngressStatus{
			LoadBalancer: netv1.IngressLoadBalancerStatus{
				Ingress: []netv1.IngressLoadBalancerIngress{{IP: "192.0.2.1"}},
			},
		},
	})
	svc := network.NewService(&fakeCS{cs: cs})
	out, err := svc.List(context.Background(), 1, "ingresses", network.ListQuery{})
	require.NoError(t, err)
	lr := out.(*network.ListResponse[network.IngressSummary])
	require.Len(t, lr.Items, 1)
	require.Equal(t, "nginx", lr.Items[0].IngressClass)
	require.Equal(t, "example.com", lr.Items[0].Hosts[0])
	require.Equal(t, "192.0.2.1", lr.Items[0].LoadBalancerIPs[0])
}

// TestGet_Service hits the Service-kind branch of the Get dispatcher.
func TestGet_Service(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.1"},
	})
	svc := network.NewService(&fakeCS{cs: cs})
	out, err := svc.Get(context.Background(), 1, "services", "n", "s")
	require.NoError(t, err)
	got := out.(*network.ServiceSummary)
	require.Equal(t, "10.0.0.1", got.ClusterIP)
}

// TestGet_Ingress hits the Ingress-kind branch of the Get dispatcher.
func TestGet_Ingress(t *testing.T) {
	cs := fake.NewSimpleClientset(&netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "i", Namespace: "n"},
	})
	svc := network.NewService(&fakeCS{cs: cs})
	out, err := svc.Get(context.Background(), 1, "ingresses", "n", "i")
	require.NoError(t, err)
	got := out.(*network.IngressSummary)
	require.Equal(t, "i", got.Name)
}

// TestGet_UnsupportedKind exercises the default branch of Get's switch.
func TestGet_UnsupportedKind(t *testing.T) {
	svc := network.NewService(&fakeCS{cs: fake.NewSimpleClientset()})
	_, err := svc.Get(context.Background(), 1, "bogus", "n", "x")
	require.Error(t, err)
	be, ok := err.(*apperr.BizError)
	require.True(t, ok, "expected *apperr.BizError, got %T", err)
	require.Equal(t, apperr.CodeBadRequest, be.Code)
	require.Equal(t, "k8s.network.unsupported_kind", be.MessageKey)
}

// TestGet_NotFound exercises the MapAPIError NotFound branch on Get.
func TestGet_NotFound(t *testing.T) {
	svc := network.NewService(&fakeCS{cs: fake.NewSimpleClientset()})
	_, err := svc.Get(context.Background(), 1, "services", "n", "missing")
	require.Error(t, err)
	be, ok := err.(*apperr.BizError)
	require.True(t, ok, "expected *apperr.BizError, got %T", err)
	require.Equal(t, apperr.CodeNotFound, be.Code)
}

func TestList_UnsupportedKind(t *testing.T) {
	svc := network.NewService(&fakeCS{cs: fake.NewSimpleClientset()})
	_, err := svc.List(context.Background(), 1, "bogus", network.ListQuery{})
	require.Error(t, err)
	be, ok := err.(*apperr.BizError)
	require.True(t, ok, "expected *apperr.BizError, got %T", err)
	require.Equal(t, apperr.CodeBadRequest, be.Code)
	require.Equal(t, "k8s.network.unsupported_kind", be.MessageKey)
}
