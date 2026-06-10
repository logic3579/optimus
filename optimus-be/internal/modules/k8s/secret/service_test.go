package secret_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"optimus-be/internal/modules/k8s/secret"
)

// fakeCS satisfies secret.Clientsetter by returning a preloaded in-memory
// fake clientset. The clusterID / purpose args are ignored — these tests
// only exercise the projection logic, not the routing/audit seam.
type fakeCS struct{ cs kubernetes.Interface }

func (f *fakeCS) Clientset(context.Context, uint64, string) (kubernetes.Interface, error) {
	return f.cs, nil
}

func TestData_DecodeUTF8(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"token": []byte("hunter2")},
	})
	svc := secret.NewService(&fakeCS{cs: cs})
	d, err := svc.Data(context.Background(), 1, "n", "s")
	require.NoError(t, err)
	require.Equal(t, "hunter2", d.Data["token"])
}

func TestData_BinaryWrappedAsBase64(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
		Data:       map[string][]byte{"key": {0xff, 0xfe, 0xfd}},
	})
	svc := secret.NewService(&fakeCS{cs: cs})
	d, err := svc.Data(context.Background(), 1, "n", "s")
	require.NoError(t, err)
	m, ok := d.Data["key"].(map[string]any)
	require.True(t, ok, "binary value must be wrapped in a map")
	require.True(t, m["base64"].(bool))
	require.Equal(t, "//79", m["value"])
}

// TestList_NoValuesLeaked is a critical guardrail — it serialises the List
// response and asserts the raw secret value never appears in it. Any future
// refactor that accidentally surfaces values will fail this test.
func TestList_NoValuesLeaked(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
		Data:       map[string][]byte{"k": []byte("super-secret-value")},
	})
	svc := secret.NewService(&fakeCS{cs: cs})
	out, err := svc.List(context.Background(), 1, secret.ListQuery{Namespace: "n"})
	require.NoError(t, err)
	require.Equal(t, 1, len(out.Items))
	require.Equal(t, []string{"k"}, out.Items[0].DataKeys)
	j, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(j), "super-secret-value", "value must NOT appear in List response")
}

// TestGet_NoValuesLeaked mirrors the List guardrail for the single-resource
// path. Get returns the same Summary shape as List, so this also protects
// against any future divergence where Get grows a Data field by accident.
func TestGet_NoValuesLeaked(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
		Data:       map[string][]byte{"k": []byte("super-secret-value")},
	})
	svc := secret.NewService(&fakeCS{cs: cs})
	out, err := svc.Get(context.Background(), 1, "n", "s")
	require.NoError(t, err)
	require.Equal(t, "s", out.Name)
	require.Equal(t, []string{"k"}, out.DataKeys)
	j, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(j), "super-secret-value", "value must NOT appear in Get response")
}
