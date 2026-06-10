package workload_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/modules/k8s/workload"
)

// fakeCS satisfies workload.Clientsetter by returning a preloaded in-memory
// fake clientset. The clusterID / purpose args are ignored — the tests
// don't exercise the routing/audit seam, only the projection logic.
type fakeCS struct{ cs kubernetes.Interface }

func (f *fakeCS) Clientset(context.Context, uint64, string) (kubernetes.Interface, error) {
	return f.cs, nil
}

func TestList_Deployments(t *testing.T) {
	rep := int32(3)
	cs := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &rep,
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 2, UpdatedReplicas: 3, AvailableReplicas: 2},
	})
	svc := workload.NewService(&fakeCS{cs: cs})
	out, err := svc.List(context.Background(), 1, "deployments", workload.ListQuery{Namespace: "default"})
	require.NoError(t, err)
	lr := out.(*workload.ListResponse[workload.DeploymentSummary])
	require.Len(t, lr.Items, 1)
	require.Equal(t, int32(3), lr.Items[0].ReplicasDesired)
	require.Equal(t, int32(2), lr.Items[0].ReplicasReady)
	require.Equal(t, int32(3), lr.Items[0].ReplicasUpdated)
	require.Equal(t, int32(2), lr.Items[0].ReplicasAvailable)
	require.Equal(t, "RollingUpdate", lr.Items[0].Strategy)
}

func TestList_Pods_ReadyCount(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "n"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Ready: true, RestartCount: 1},
				{Ready: false, RestartCount: 2},
			},
		},
	})
	svc := workload.NewService(&fakeCS{cs: cs})
	out, err := svc.List(context.Background(), 1, "pods", workload.ListQuery{Namespace: "n"})
	require.NoError(t, err)
	lr := out.(*workload.ListResponse[workload.PodSummary])
	require.Len(t, lr.Items, 1)
	require.Equal(t, 1, lr.Items[0].ReadyContainers)
	require.Equal(t, 2, lr.Items[0].TotalContainers)
	require.Equal(t, int32(3), lr.Items[0].RestartCount)
	require.Equal(t, "Running", lr.Items[0].Phase)
}

func TestList_UnsupportedKind(t *testing.T) {
	svc := workload.NewService(&fakeCS{cs: fake.NewSimpleClientset()})
	_, err := svc.List(context.Background(), 1, "garbage", workload.ListQuery{})
	require.Error(t, err)
	be, ok := err.(*apperr.BizError)
	require.True(t, ok, "expected *apperr.BizError, got %T", err)
	require.Equal(t, apperr.CodeBadRequest, be.Code)
	require.Equal(t, "k8s.workload.unsupported_kind", be.MessageKey)
}

// TestGet_Pod exercises the Get dispatcher and confirms the same toPod
// projection runs whether the entry point was List or Get.
func TestGet_Pod(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "n"},
		Spec:       corev1.PodSpec{NodeName: "node-a"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.5",
			ContainerStatuses: []corev1.ContainerStatus{
				{Ready: true, RestartCount: 0},
			},
		},
	})
	svc := workload.NewService(&fakeCS{cs: cs})
	out, err := svc.Get(context.Background(), 1, "pods", "n", "p")
	require.NoError(t, err)
	pod := out.(*workload.PodSummary)
	require.Equal(t, "p", pod.Name)
	require.Equal(t, "node-a", pod.NodeName)
	require.Equal(t, "10.0.0.5", pod.PodIP)
	require.Equal(t, 1, pod.ReadyContainers)
}
