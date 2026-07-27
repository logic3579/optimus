package repo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/registry"

	"optimus-be/internal/models"
)

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

func TestResolveArtifactHTTPHashesExactDownloadedBytes(t *testing.T) {
	archive := artifactChartArchive(t, "demo", "1.2.3")
	wantDigest := digestOf(archive)
	var downloads int
	m := &models.AppsChartRepo{Name: "demo", Type: "http", URL: "https://charts.example.test"}
	download := func(got *models.AppsChartRepo, pwd, chartName, version string) ([]byte, error) {
		downloads++
		require.Same(t, m, got)
		require.Empty(t, pwd)
		require.Equal(t, "demo", chartName)
		require.Equal(t, "1.2.3", version)
		return archive, nil
	}

	got, err := resolveArtifactHTTP(m, "", 42, "demo", "1.2.3", download)
	require.NoError(t, err)
	require.Equal(t, &Artifact{
		RepoID: 42, ChartName: "demo", Version: "1.2.3", Digest: wantDigest,
	}, got)
	require.Equal(t, 1, downloads, "chart archive must be downloaded once")
	require.Equal(t, make([]byte, len(archive)), archive, "downloaded chart bytes must be wiped")
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

func TestLoadVerifiedChartRejectsDigestMismatchBeforeParse(t *testing.T) {
	invalidArchive := []byte("not a helm chart")
	var downloads int
	download := func() ([]byte, error) {
		downloads++
		return invalidArchive, nil
	}

	_, err := loadVerifiedChart(Artifact{
		RepoID: 42, ChartName: "demo", Version: "1.2.3",
		Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}, download)
	require.ErrorIs(t, err, ErrArtifactDigestMismatch)
	require.Equal(t, 1, downloads, "chart archive must be downloaded once")
	require.Equal(t, make([]byte, len(invalidArchive)), invalidArchive, "downloaded chart bytes must be wiped")
}
