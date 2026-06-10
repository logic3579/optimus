package clusterscoped_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"optimus-be/internal/modules/k8s/clusterscoped"
)

// fakeCS satisfies clusterscoped.Clientsetter by returning a preloaded
// in-memory fake clientset. The clusterID / purpose args are ignored — the
// tests don't exercise the routing/audit seam, only the projection logic.
type fakeCS struct{ cs kubernetes.Interface }

func (f *fakeCS) Clientset(context.Context, uint64, string) (kubernetes.Interface, error) {
	return f.cs, nil
}

func TestListNamespaces(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default", CreationTimestamp: metav1.NewTime(time.Now())},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
	)
	svc := clusterscoped.NewService(&fakeCS{cs: cs})
	out, err := svc.ListNamespaces(context.Background(), 1, clusterscoped.ListQuery{})
	require.NoError(t, err)
	require.Len(t, out.Items, 2)
	names := []string{out.Items[0].Name, out.Items[1].Name}
	require.Contains(t, names, "default")
	require.Contains(t, names, "kube-system")
}

func TestListNodes_RolesExtraction(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "n1",
			Labels: map[string]string{"node-role.kubernetes.io/control-plane": ""},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: "v1.30.5"},
		},
	})
	svc := clusterscoped.NewService(&fakeCS{cs: cs})
	out, err := svc.ListNodes(context.Background(), 1, clusterscoped.ListQuery{})
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	require.True(t, out.Items[0].Ready)
	require.Contains(t, out.Items[0].Roles, "control-plane")
}

// TestGetNode covers the Get-by-name path that List tests don't reach.
func TestGetNode(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "n1",
			Labels: map[string]string{"node-role.kubernetes.io/worker": ""},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: "v1.30.5"},
		},
	})
	svc := clusterscoped.NewService(&fakeCS{cs: cs})
	out, err := svc.GetNode(context.Background(), 1, "n1")
	require.NoError(t, err)
	require.Equal(t, "n1", out.Name)
	require.True(t, out.Ready)
	require.Contains(t, out.Roles, "worker")
}

// TestGetNode_NotFound exercises the MapAPIError NotFound branch on Get.
func TestGetNode_NotFound(t *testing.T) {
	cs := fake.NewSimpleClientset()
	svc := clusterscoped.NewService(&fakeCS{cs: cs})
	_, err := svc.GetNode(context.Background(), 1, "missing")
	require.Error(t, err)
}

func TestListEvents(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Namespace: "default", Name: "e1"},
		Type:           "Warning",
		Reason:         "FailedScheduling",
		Message:        "no nodes",
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "p1", Namespace: "default"},
		Count:          3,
	})
	svc := clusterscoped.NewService(&fakeCS{cs: cs})
	out, err := svc.ListEvents(context.Background(), 1, clusterscoped.ListQuery{Namespace: "default"})
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	require.Equal(t, int32(3), out.Items[0].Count)
}
