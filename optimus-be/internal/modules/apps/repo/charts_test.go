//go:build dbtest

package repo_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/models"
)

// tgzWithValues builds a minimal chart-style tarball that contains a single
// <root>/values.yaml entry. Mirror of the helper used by helpers_test.go;
// duplicated here because tests live in the _test external package.
func tgzWithValues(values string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte(values)
	_ = tw.WriteHeader(&tar.Header{
		Name: "demo/values.yaml", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestListCharts_HTTPRepo(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.yaml":
			fmt.Fprintf(w, `apiVersion: v1
entries:
  demo:
    - name: demo
      version: 1.0.0
      appVersion: "1"
      description: "demo chart"
      urls: ["%s/demo-1.0.0.tgz"]
`, serverURL)
		case "/demo-1.0.0.tgz":
			_, _ = w.Write(tgzWithValues("replicaCount: 1\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	s, r := setupSvc(t)
	m := &models.AppsChartRepo{Name: "demo", Type: "http", URL: server.URL}
	require.NoError(t, r.Create(context.Background(), m))

	charts, err := s.ListCharts(context.Background(), m.ID)
	require.NoError(t, err)
	require.Len(t, charts, 1)
	require.Equal(t, "demo", charts[0].Name)
	require.Equal(t, 1, charts[0].VersionCount)
	require.Equal(t, "demo chart", charts[0].Description)

	versions, err := s.ListVersions(context.Background(), m.ID, "demo")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.Equal(t, "1.0.0", versions[0].Version)
	require.Equal(t, "1", versions[0].AppVersion)

	values, err := s.GetDefaultValues(context.Background(), m.ID, "demo", "1.0.0")
	require.NoError(t, err)
	require.Contains(t, values, "replicaCount: 1")
}

func TestListVersions_ChartNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.yaml" {
			fmt.Fprint(w, `apiVersion: v1
entries:
  demo:
    - name: demo
      version: 1.0.0
      urls: ["/demo-1.0.0.tgz"]
`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	s, r := setupSvc(t)
	m := &models.AppsChartRepo{Name: "demo", Type: "http", URL: server.URL}
	require.NoError(t, r.Create(context.Background(), m))

	_, err := s.ListVersions(context.Background(), m.ID, "missing")
	require.Error(t, err)
}

func TestListCharts_RepoNotFound(t *testing.T) {
	s, _ := setupSvc(t)
	_, err := s.ListCharts(context.Background(), 9999)
	require.Error(t, err)
}

func TestListCharts_OCIInfersOne(t *testing.T) {
	s, r := setupSvc(t)
	m := &models.AppsChartRepo{Name: "oci-demo", Type: "oci", URL: "oci://ghcr.io/org/myapp"}
	require.NoError(t, r.Create(context.Background(), m))

	charts, err := s.ListCharts(context.Background(), m.ID)
	require.NoError(t, err)
	require.Len(t, charts, 1)
	require.Equal(t, "myapp", charts[0].Name)
}
