//go:build dbtest

package repo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/registry"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/models"
)

const artifactMigrationsPath = "../../../../migrations"

type artifactCipher struct{}

func (artifactCipher) Seal(b []byte) ([]byte, error) { return b, nil }
func (artifactCipher) Open(b []byte) ([]byte, error) { return b, nil }

func newArtifactService(t *testing.T) (*Service, *Repo) {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join(artifactMigrationsPath))
	t.Cleanup(teardown)
	r := NewRepo(gdb)
	return NewService(r, artifactCipher{}, nil), r
}

func artifactChartArchive(t *testing.T, name, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	chartYAML := []byte(fmt.Sprintf("apiVersion: v2\nname: %s\nversion: %s\n", name, version))
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: name + "/Chart.yaml", Mode: 0o644, Size: int64(len(chartYAML)), Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write(chartYAML)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("sha256:%x", sum)
}

func TestServiceResolveArtifactHTTPHashesExactDownloadedBytes(t *testing.T) {
	archive := artifactChartArchive(t, "demo", "1.2.3")
	var archiveRequests atomic.Int32
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.yaml":
			_, _ = fmt.Fprintf(w, `apiVersion: v1
entries:
  demo:
    - name: demo
      version: 1.2.3
      urls: ["%s/demo-1.2.3.tgz"]
`, serverURL)
		case "/demo-1.2.3.tgz":
			archiveRequests.Add(1)
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	svc, r := newArtifactService(t)
	m := &models.AppsChartRepo{Name: "demo", Type: "http", URL: server.URL}
	require.NoError(t, r.Create(context.Background(), m))

	got, err := svc.ResolveArtifact(context.Background(), m.ID, "demo", "1.2.3")
	require.NoError(t, err)
	require.Equal(t, &Artifact{
		RepoID: m.ID, ChartName: "demo", Version: "1.2.3", Digest: digestOf(archive),
	}, got)
	require.EqualValues(t, 1, archiveRequests.Load(), "chart archive must be downloaded once")
}

func TestResolveArtifactOCIHashesInjectedPullBytes(t *testing.T) {
	archive := artifactChartArchive(t, "demo", "2.0.0")
	wantDigest := digestOf(archive)
	var pulls int
	m := &models.AppsChartRepo{Name: "demo", Type: "oci", URL: "oci://registry.example/org/demo"}
	pull := func(_ *registry.Client, ref string, _ ...registry.PullOption) (*registry.PullResult, error) {
		pulls++
		require.Equal(t, "registry.example/org/demo:2.0.0", ref)
		return &registry.PullResult{Chart: &registry.DescriptorPullSummaryWithMeta{
			DescriptorPullSummary: registry.DescriptorPullSummary{Data: archive},
		}}, nil
	}

	got, err := resolveArtifactOCI(m, "", 42, "demo", "2.0.0", pull)
	require.NoError(t, err)
	require.Equal(t, &Artifact{
		RepoID: 42, ChartName: "demo", Version: "2.0.0", Digest: wantDigest,
	}, got)
	require.Equal(t, 1, pulls)
	require.Equal(t, make([]byte, len(archive)), archive, "downloaded chart bytes must be wiped")
}

func TestServiceLoadVerifiedChartRejectsDigestMismatchBeforeParsing(t *testing.T) {
	invalidArchive := []byte("not a helm chart")
	var archiveRequests atomic.Int32
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.yaml":
			_, _ = fmt.Fprintf(w, `apiVersion: v1
entries:
  demo:
    - name: demo
      version: 1.2.3
      urls: ["%s/demo-1.2.3.tgz"]
`, serverURL)
		case "/demo-1.2.3.tgz":
			archiveRequests.Add(1)
			_, _ = w.Write(invalidArchive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	svc, r := newArtifactService(t)
	m := &models.AppsChartRepo{Name: "demo", Type: "http", URL: server.URL}
	require.NoError(t, r.Create(context.Background(), m))

	_, err := svc.LoadVerifiedChart(context.Background(), Artifact{
		RepoID: m.ID, ChartName: "demo", Version: "1.2.3",
		Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	require.ErrorIs(t, err, ErrArtifactDigestMismatch)
	require.EqualValues(t, 1, archiveRequests.Load(), "chart archive must be downloaded once")
}
