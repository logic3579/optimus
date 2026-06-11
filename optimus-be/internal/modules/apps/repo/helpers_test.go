package repo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/models"
)

// tgzWithValues builds a minimal chart-style tarball that contains a
// single <root>/values.yaml entry. Used by tests in this package.
func tgzWithValues(t *testing.T, root, values string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte(values)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     root + "/values.yaml",
		Mode:     0o644,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write(body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestReadValuesFromTgz_HappyPath(t *testing.T) {
	body := tgzWithValues(t, "demo", "replicaCount: 1\n")
	out, err := readValuesFromTgz(body)
	require.NoError(t, err)
	require.Equal(t, "replicaCount: 1\n", out)
}

func TestReadValuesFromTgz_NoValuesYAML(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "demo/Chart.yaml", Mode: 0o644, Size: 4, Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write([]byte("name"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	out, err := readValuesFromTgz(buf.Bytes())
	require.NoError(t, err)
	require.Equal(t, "", out)
}

func TestReadValuesFromTgz_NotGzip(t *testing.T) {
	_, err := readValuesFromTgz([]byte("not a tarball"))
	require.Error(t, err)
}

func TestReadValuesFromTgz_SkipsNestedSubchartValues(t *testing.T) {
	// Nested subchart values.yaml at <root>/charts/<sub>/values.yaml must NOT
	// be returned — only the chart's own values.yaml counts.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("subchart: true\n")
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "demo/charts/sub/values.yaml", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write(body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	out, err := readValuesFromTgz(buf.Bytes())
	require.NoError(t, err)
	require.Equal(t, "", out, "nested subchart values must be skipped")
}

func TestAbsoluteURL(t *testing.T) {
	cases := []struct {
		base, in, want string
	}{
		{"https://x.example.com", "demo-1.0.0.tgz", "https://x.example.com/demo-1.0.0.tgz"},
		{"https://x.example.com/", "/demo-1.0.0.tgz", "https://x.example.com/demo-1.0.0.tgz"},
		{"https://x.example.com", "https://other.example.com/chart.tgz", "https://other.example.com/chart.tgz"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, absoluteURL(tc.base, tc.in))
	}
}

func TestOCIHost(t *testing.T) {
	require.Equal(t, "ghcr.io", ociHost("oci://ghcr.io/org/myapp"))
	require.Equal(t, "registry.example.com", ociHost("oci://registry.example.com"))
}

func TestOCIRef(t *testing.T) {
	// chart already in path -> no extra segment.
	require.Equal(t, "ghcr.io/org/myapp", ociRef("oci://ghcr.io/org/myapp", "myapp"))
	// chart not in path -> appended.
	require.Equal(t, "ghcr.io/org/myapp", ociRef("oci://ghcr.io/org", "myapp"))
	// trailing slash in URL.
	require.Equal(t, "ghcr.io/org/myapp", ociRef("oci://ghcr.io/org/", "myapp"))
}

func TestListOCI_InfersChartName(t *testing.T) {
	out, err := listOCI(&models.AppsChartRepo{URL: "oci://ghcr.io/org/myapp"}, "")
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "myapp", out[0].Name)

	out, err = listOCI(&models.AppsChartRepo{URL: "oci://ghcr.io"}, "")
	require.NoError(t, err)
	require.Empty(t, out, "registry root cannot be enumerated")
}
