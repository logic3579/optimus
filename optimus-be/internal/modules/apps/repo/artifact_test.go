package repo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/registry"

	"optimus-be/internal/models"
)

type artifactRoundTripFunc func(*http.Request) (*http.Response, error)

func (f artifactRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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

func artifactServiceForTest(m *models.AppsChartRepo) *Service {
	return &Service{artifactLookup: func(context.Context, uint64) (*models.AppsChartRepo, string, error) {
		return m, "secret", nil
	}}
}

func TestResolveArtifactHTTPUsesContextAndHashesExactDownloadedBytes(t *testing.T) {
	archive := artifactChartArchive(t, "demo", "1.2.3")
	wantDigest := digestOf(archive)
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "artifact-request")
	m := &models.AppsChartRepo{
		ID: 42, Name: "demo", Type: "http", URL: "https://charts.example.test/repository", Username: "robot",
	}
	var indexRequests, archiveRequests int
	client := &http.Client{Transport: artifactRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "artifact-request", req.Context().Value(contextKey{}))
		username, password, ok := req.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "robot", username)
		require.Equal(t, "secret", password)

		var body []byte
		switch req.URL.String() {
		case "https://charts.example.test/repository/index.yaml":
			indexRequests++
			body = []byte(`apiVersion: v1
entries:
  demo:
    - name: demo
      version: 9.9.9
      urls: ["wrong.tgz"]
    - name: demo
      version: 1.2.3
      urls: ["archives/demo-1.2.3.tgz"]
`)
		case "https://charts.example.test/repository/archives/demo-1.2.3.tgz":
			archiveRequests++
			body = archive
		default:
			return nil, fmt.Errorf("unexpected request URL: %s", req.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    req,
		}, nil
	})}
	svc := artifactServiceForTest(m)
	svc.artifactHTTPClient = client

	got, err := svc.ResolveArtifact(ctx, m.ID, "demo", "1.2.3")
	require.NoError(t, err)
	require.Equal(t, &Artifact{
		RepoID: m.ID, ChartName: "demo", Version: "1.2.3", Digest: wantDigest,
	}, got)
	require.Equal(t, 1, indexRequests)
	require.Equal(t, 1, archiveRequests, "chart archive must be downloaded once")
}

func TestResolveArtifactHTTPCanceledContextStopsRequest(t *testing.T) {
	m := &models.AppsChartRepo{ID: 42, Type: "http", URL: "https://charts.example.test/repository"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := artifactServiceForTest(m)
	svc.artifactHTTPClient = &http.Client{Transport: artifactRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	})}

	_, err := svc.ResolveArtifact(ctx, m.ID, "demo", "1.2.3")
	require.ErrorIs(t, err, context.Canceled)
}

func TestResolveArtifactHTTPDoesNotForwardAuthCrossOrigin(t *testing.T) {
	archive := artifactChartArchive(t, "demo", "1.2.3")
	m := &models.AppsChartRepo{
		ID: 42, Type: "http", URL: "https://charts.example.test/repository", Username: "robot",
	}
	client := &http.Client{Transport: artifactRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body []byte
		switch req.URL.String() {
		case "https://charts.example.test/repository/index.yaml":
			_, _, ok := req.BasicAuth()
			require.True(t, ok)
			body = []byte(`apiVersion: v1
entries:
  demo:
    - name: demo
      version: 1.2.3
      urls: ["https://cdn.example.test/demo-1.2.3.tgz"]
`)
		case "https://cdn.example.test/demo-1.2.3.tgz":
			_, _, ok := req.BasicAuth()
			require.False(t, ok)
			body = archive
		default:
			return nil, fmt.Errorf("unexpected request URL: %s", req.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    req,
		}, nil
	})}
	svc := artifactServiceForTest(m)
	svc.artifactHTTPClient = client

	got, err := svc.ResolveArtifact(context.Background(), m.ID, "demo", "1.2.3")
	require.NoError(t, err)
	require.Equal(t, digestOf(archive), got.Digest)
}

func TestResolveArtifactOCIHashesInjectedPullBytes(t *testing.T) {
	archive := artifactChartArchive(t, "demo", "2.0.0")
	wantDigest := digestOf(archive)
	var pulls int
	m := &models.AppsChartRepo{Name: "demo", Type: "oci", URL: "oci://registry.example/org/demo"}
	pull := func(ctx context.Context, _ *registry.Client, ref string, _ ...registry.PullOption) (*registry.PullResult, error) {
		pulls++
		require.NoError(t, ctx.Err())
		require.Equal(t, "registry.example/org/demo:2.0.0", ref)
		return &registry.PullResult{Chart: &registry.DescriptorPullSummaryWithMeta{
			DescriptorPullSummary: registry.DescriptorPullSummary{Data: archive},
		}}, nil
	}

	got, err := resolveArtifactOCI(context.Background(), m, "", 42, "demo", "2.0.0", pull)
	require.NoError(t, err)
	require.Equal(t, &Artifact{
		RepoID: 42, ChartName: "demo", Version: "2.0.0", Digest: wantDigest,
	}, got)
	require.Equal(t, 1, pulls)
	require.Equal(t, make([]byte, len(archive)), archive, "downloaded chart bytes must be wiped")
}

func TestResolveArtifactOCICanceledContextStopsPull(t *testing.T) {
	m := &models.AppsChartRepo{ID: 42, Type: "oci", URL: "oci://registry.example/org/demo"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var pulls int
	svc := artifactServiceForTest(m)
	svc.artifactOCIDownload = func(got context.Context, _ *models.AppsChartRepo, _, _, _ string) ([]byte, error) {
		pulls++
		require.ErrorIs(t, got.Err(), context.Canceled)
		return nil, got.Err()
	}

	_, err := svc.ResolveArtifact(ctx, m.ID, "demo", "1.2.3")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, pulls)
}

func TestResolveArtifactWipesPartialBytesOnDownloadError(t *testing.T) {
	wantErr := errors.New("safe test failure")
	for _, repoType := range []string{"http", "oci"} {
		t.Run(repoType, func(t *testing.T) {
			partial := []byte("partial chart bytes")
			m := &models.AppsChartRepo{ID: 42, Type: repoType, URL: repoType + "://example.test/demo"}
			var err error
			if repoType == "http" {
				svc := artifactServiceForTest(m)
				svc.artifactHTTPDownload = func(context.Context, *models.AppsChartRepo, string, string, string) ([]byte, error) {
					return partial, wantErr
				}
				_, err = svc.ResolveArtifact(context.Background(), m.ID, "demo", "1.2.3")
			} else {
				pull := func(context.Context, *registry.Client, string, ...registry.PullOption) (*registry.PullResult, error) {
					return &registry.PullResult{Chart: &registry.DescriptorPullSummaryWithMeta{
						DescriptorPullSummary: registry.DescriptorPullSummary{Data: partial},
					}}, wantErr
				}
				_, err = resolveArtifactOCI(context.Background(), m, "", m.ID, "demo", "1.2.3", pull)
			}

			require.ErrorIs(t, err, wantErr)
			require.Equal(t, make([]byte, len(partial)), partial)
		})
	}
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

func TestLoadVerifiedChartParsesAndWipesVerifiedBytes(t *testing.T) {
	archive := artifactChartArchive(t, "demo", "1.2.3")
	digest := digestOf(archive)
	m := &models.AppsChartRepo{ID: 42, Type: "http", URL: "https://charts.example.test/repository"}
	svc := artifactServiceForTest(m)
	svc.artifactHTTPDownload = func(context.Context, *models.AppsChartRepo, string, string, string) ([]byte, error) {
		return archive, nil
	}

	ch, err := svc.LoadVerifiedChart(context.Background(), Artifact{
		RepoID: m.ID, ChartName: "demo", Version: "1.2.3", Digest: digest,
	})
	require.NoError(t, err)
	require.Equal(t, "demo", ch.Metadata.Name)
	require.Equal(t, "1.2.3", ch.Metadata.Version)
	require.Equal(t, make([]byte, len(archive)), archive)
}

func TestLoadVerifiedChartWipesPartialBytesOnDownloadError(t *testing.T) {
	partial := []byte("partial chart bytes")
	wantErr := errors.New("safe test failure")

	_, err := loadVerifiedChart(Artifact{}, func() ([]byte, error) {
		return partial, wantErr
	})
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, make([]byte, len(partial)), partial)
}
