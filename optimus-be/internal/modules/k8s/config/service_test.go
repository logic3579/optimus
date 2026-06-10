package config_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"optimus-be/internal/modules/k8s/config"
)

// fakeCS satisfies config.Clientsetter by returning a preloaded in-memory
// fake clientset. The clusterID / purpose args are ignored — the tests
// only exercise the projection logic, not the routing/audit seam.
type fakeCS struct{ cs kubernetes.Interface }

func (f *fakeCS) Clientset(context.Context, uint64, string) (kubernetes.Interface, error) {
	return f.cs, nil
}

func TestList_ConfigMaps(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "n"},
		Data:       map[string]string{"key": "value"},
	})
	svc := config.NewService(&fakeCS{cs: cs})
	out, err := svc.List(context.Background(), 1, config.ListQuery{Namespace: "n"})
	require.NoError(t, err)
	require.Equal(t, 1, len(out.Items))
	require.Equal(t, "demo", out.Items[0].Name)
	require.Equal(t, []string{"key"}, out.Items[0].DataKeys)
	require.Equal(t, 1, out.Items[0].DataCount)
}

// TestGet_NotFound exercises the MapAPIError NotFound branch on Get.
func TestGet_NotFound(t *testing.T) {
	svc := config.NewService(&fakeCS{cs: fake.NewSimpleClientset()})
	_, err := svc.Get(context.Background(), 1, "n", "missing")
	require.Error(t, err)
}

// TestList_Empty covers the no-data, no-binary-data path in toSummary.
func TestList_Empty(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "n"},
	})
	svc := config.NewService(&fakeCS{cs: cs})
	out, err := svc.List(context.Background(), 1, config.ListQuery{Namespace: "n"})
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	require.Equal(t, 0, out.Items[0].DataCount)
}

func TestGet_ConfigMap_IncludesData(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "n"},
		Data:       map[string]string{"key": "value"},
		BinaryData: map[string][]byte{"bin": {0xff}},
	})
	svc := config.NewService(&fakeCS{cs: cs})
	out, err := svc.Get(context.Background(), 1, "n", "demo")
	require.NoError(t, err)
	require.Equal(t, "value", out.Data["key"])
	require.Equal(t, []string{"bin"}, out.BinaryKeys)
	require.Equal(t, 2, out.DataCount)
	require.Equal(t, []string{"bin", "key"}, out.DataKeys)
}
