package repo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
)

// ErrArtifactDigestMismatch is intentionally stable and contains no upstream
// detail. The delivery adapter maps it into the P6 453xx execution taxonomy.
var ErrArtifactDigestMismatch = errors.New("chart artifact digest mismatch")

// Artifact is an immutable chart repository selection resolved to content.
type Artifact struct {
	RepoID    uint64
	ChartName string
	Version   string
	Digest    string
}

// ResolveArtifact downloads one chart archive and freezes its exact bytes to a
// lowercase SHA-256 digest. The archive is not cached or persisted.
func (s *Service) ResolveArtifact(ctx context.Context, repoID uint64, chartName, version string) (*Artifact, error) {
	source := s.artifactSource()
	m, pwd, err := source.lookupRepo(ctx, repoID)
	if err != nil {
		return nil, err
	}

	switch m.Type {
	case "http":
		return resolveArtifactHTTP(ctx, m, pwd, repoID, chartName, version, source.downloadHTTP)
	case "oci":
		return resolveArtifactDownload(ctx, m, pwd, repoID, chartName, version, source.downloadOCI)
	default:
		return nil, apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type", "unsupported chart repository type")
	}
}

// LoadVerifiedChart downloads an artifact once, verifies its digest before
// invoking Helm's parser, and wipes the downloaded archive after parsing.
func (s *Service) LoadVerifiedChart(ctx context.Context, artifact Artifact) (*chart.Chart, error) {
	source := s.artifactSource()
	m, pwd, err := source.lookupRepo(ctx, artifact.RepoID)
	if err != nil {
		return nil, err
	}

	var download func() ([]byte, error)
	switch m.Type {
	case "http":
		download = func() ([]byte, error) {
			return source.downloadHTTP(ctx, m, pwd, artifact.ChartName, artifact.Version)
		}
	case "oci":
		download = func() ([]byte, error) {
			return source.downloadOCI(ctx, m, pwd, artifact.ChartName, artifact.Version)
		}
	default:
		return nil, apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type", "unsupported chart repository type")
	}
	return loadVerifiedChart(artifact, download)
}

func loadVerifiedChart(artifact Artifact, download func() ([]byte, error)) (*chart.Chart, error) {
	tgz, err := download()
	if tgz != nil {
		defer wipeChartBytes(tgz)
	}
	if err != nil {
		return nil, err
	}

	actual := digestChartBytes(tgz)
	if subtle.ConstantTimeCompare([]byte(actual), []byte(artifact.Digest)) != 1 {
		return nil, ErrArtifactDigestMismatch
	}

	ch, err := loader.LoadArchive(bytes.NewReader(tgz))
	if err != nil {
		return nil, apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_chart", "invalid chart archive")
	}
	return ch, nil
}

func (s *Service) storedArtifactRepo(ctx context.Context, repoID uint64) (*models.AppsChartRepo, string, error) {
	m, err := s.repo.Get(ctx, repoID)
	if err != nil {
		return nil, "", mapNotFound(err)
	}
	pwd, err := s.decryptPassword(ctx, m)
	if err != nil {
		return nil, "", err
	}
	return m, pwd, nil
}

type artifactLookupFunc func(context.Context, uint64) (*models.AppsChartRepo, string, error)

type httpChartDownloadFunc func(context.Context, *models.AppsChartRepo, string, string, string) ([]byte, error)

type ociChartDownloadFunc func(context.Context, *models.AppsChartRepo, string, string, string) ([]byte, error)

// artifactSource is the narrow repository/transport collaborator used by the
// public artifact methods. Service keeps one optional collaborator so
// tests and alternate transports do not require mutable package globals.
type artifactSource struct {
	service    *Service
	lookupFn   artifactLookupFunc
	httpClient *http.Client
	httpFn     httpChartDownloadFunc
	ociFn      ociChartDownloadFunc
}

func (s *Service) artifactSource() *artifactSource {
	if s.artifacts != nil {
		return s.artifacts
	}
	return &artifactSource{service: s}
}

func (s *artifactSource) lookupRepo(ctx context.Context, repoID uint64) (*models.AppsChartRepo, string, error) {
	if s.lookupFn != nil {
		return s.lookupFn(ctx, repoID)
	}
	return s.service.storedArtifactRepo(ctx, repoID)
}

func (s *artifactSource) downloadHTTP(
	ctx context.Context,
	m *models.AppsChartRepo,
	pwd, chartName, version string,
) ([]byte, error) {
	if s.httpFn != nil {
		return s.httpFn(ctx, m, pwd, chartName, version)
	}
	return chartTgzHTTP(ctx, s.httpClient, m, pwd, chartName, version)
}

func (s *artifactSource) downloadOCI(
	ctx context.Context,
	m *models.AppsChartRepo,
	pwd, chartName, version string,
) ([]byte, error) {
	if s.ociFn != nil {
		return s.ociFn(ctx, m, pwd, chartName, version)
	}
	return chartTgzOCI(ctx, m, pwd, chartName, version)
}

func resolveArtifactHTTP(
	ctx context.Context,
	m *models.AppsChartRepo,
	pwd string,
	repoID uint64,
	chartName, version string,
	download httpChartDownloadFunc,
) (*Artifact, error) {
	tgz, err := download(ctx, m, pwd, chartName, version)
	if tgz != nil {
		defer wipeChartBytes(tgz)
	}
	if err != nil {
		return nil, err
	}
	return artifactForBytes(repoID, chartName, version, tgz), nil
}

func resolveArtifactDownload(
	ctx context.Context,
	m *models.AppsChartRepo,
	pwd string,
	repoID uint64,
	chartName, version string,
	download ociChartDownloadFunc,
) (*Artifact, error) {
	tgz, err := download(ctx, m, pwd, chartName, version)
	if tgz != nil {
		defer wipeChartBytes(tgz)
	}
	if err != nil {
		return nil, err
	}
	return artifactForBytes(repoID, chartName, version, tgz), nil
}

func artifactForBytes(repoID uint64, chartName, version string, tgz []byte) *Artifact {
	return &Artifact{
		RepoID:    repoID,
		ChartName: chartName,
		Version:   version,
		Digest:    digestChartBytes(tgz),
	}
}

func digestChartBytes(tgz []byte) string {
	sum := sha256.Sum256(tgz)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func wipeChartBytes(tgz []byte) {
	for i := range tgz {
		tgz[i] = 0
	}
}
