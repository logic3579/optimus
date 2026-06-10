package workload_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
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

// allWorkloadFixtures returns a fake clientset preloaded with one object of
// each of the 7 supported workload kinds in namespace "n". Shared by both
// TestList_AllKinds and TestGet_AllKinds to keep the table-driven coverage
// pass short.
func allWorkloadFixtures() *fake.Clientset {
	rep := int32(1)
	return fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "n"},
			Spec:       appsv1.DeploymentSpec{Replicas: &rep},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: "n"},
			Spec:       appsv1.StatefulSetSpec{Replicas: &rep, ServiceName: "svc"},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "n"},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "j", Namespace: "n"},
			Spec:       batchv1.JobSpec{Completions: &rep},
			Status: batchv1.JobStatus{
				StartTime:      &metav1.Time{Time: metav1.Now().Time},
				CompletionTime: &metav1.Time{Time: metav1.Now().Time},
			},
		},
		&batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: "cj", Namespace: "n"},
			Spec:       batchv1.CronJobSpec{Schedule: "* * * * *"},
			Status:     batchv1.CronJobStatus{LastScheduleTime: &metav1.Time{Time: metav1.Now().Time}},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "rs",
				Namespace:       "n",
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "d"}},
			},
			Spec: appsv1.ReplicaSetSpec{Replicas: &rep},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "n"},
		},
	)
}

// TestList_AllKinds walks the List dispatcher for every supported kind and
// confirms each branch returns a non-nil envelope without error. Existence
// of one item per kind is enough to cover the projection loop too.
func TestList_AllKinds(t *testing.T) {
	svc := workload.NewService(&fakeCS{cs: allWorkloadFixtures()})
	for _, kind := range []string{
		"deployments", "statefulsets", "daemonsets",
		"jobs", "cronjobs", "replicasets", "pods",
	} {
		t.Run(kind, func(t *testing.T) {
			out, err := svc.List(context.Background(), 1, kind, workload.ListQuery{Namespace: "n"})
			require.NoError(t, err)
			require.NotNil(t, out)
		})
	}
}

// TestGet_AllKinds walks the Get dispatcher for every supported kind.
func TestGet_AllKinds(t *testing.T) {
	svc := workload.NewService(&fakeCS{cs: allWorkloadFixtures()})
	cases := []struct{ kind, name string }{
		{"deployments", "d"}, {"statefulsets", "sts"}, {"daemonsets", "ds"},
		{"jobs", "j"}, {"cronjobs", "cj"}, {"replicasets", "rs"}, {"pods", "p"},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			out, err := svc.Get(context.Background(), 1, c.kind, "n", c.name)
			require.NoError(t, err)
			require.NotNil(t, out)
		})
	}
}

// TestGet_UnsupportedKind hits the default branch of Get's switch.
func TestGet_UnsupportedKind(t *testing.T) {
	svc := workload.NewService(&fakeCS{cs: fake.NewSimpleClientset()})
	_, err := svc.Get(context.Background(), 1, "bogus", "n", "x")
	require.Error(t, err)
	be, ok := err.(*apperr.BizError)
	require.True(t, ok, "expected *apperr.BizError, got %T", err)
	require.Equal(t, apperr.CodeBadRequest, be.Code)
	require.Equal(t, "k8s.workload.unsupported_kind", be.MessageKey)
}

// TestGet_NotFound exercises the apierr.MapAPIError NotFound branch on the
// Get path so the error-translation chain is covered for at least one kind.
func TestGet_NotFound(t *testing.T) {
	svc := workload.NewService(&fakeCS{cs: fake.NewSimpleClientset()})
	_, err := svc.Get(context.Background(), 1, "deployments", "n", "missing")
	require.Error(t, err)
	be, ok := err.(*apperr.BizError)
	require.True(t, ok, "expected *apperr.BizError, got %T", err)
	require.Equal(t, apperr.CodeNotFound, be.Code)
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
