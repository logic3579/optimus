package repo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"

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
	m, pwd, err := s.artifactRepo(ctx, repoID)
	if err != nil {
		return nil, err
	}

	switch m.Type {
	case "http":
		tgz, err := chartTgzHTTP(m, pwd, chartName, version)
		if err != nil {
			return nil, err
		}
		defer wipeChartBytes(tgz)
		return artifactForBytes(repoID, chartName, version, tgz), nil
	case "oci":
		return resolveArtifactOCI(m, pwd, repoID, chartName, version, defaultOCIPull)
	default:
		return nil, apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type", "unsupported chart repository type")
	}
}

// LoadVerifiedChart downloads an artifact once, verifies its digest before
// invoking Helm's parser, and wipes the downloaded archive after parsing.
func (s *Service) LoadVerifiedChart(ctx context.Context, artifact Artifact) (*chart.Chart, error) {
	m, pwd, err := s.artifactRepo(ctx, artifact.RepoID)
	if err != nil {
		return nil, err
	}

	var tgz []byte
	switch m.Type {
	case "http":
		tgz, err = chartTgzHTTP(m, pwd, artifact.ChartName, artifact.Version)
	case "oci":
		tgz, err = chartTgzOCI(m, pwd, artifact.ChartName, artifact.Version)
	default:
		return nil, apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type", "unsupported chart repository type")
	}
	if err != nil {
		return nil, err
	}
	defer wipeChartBytes(tgz)

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

func (s *Service) artifactRepo(ctx context.Context, repoID uint64) (*models.AppsChartRepo, string, error) {
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

func resolveArtifactOCI(
	m *models.AppsChartRepo,
	pwd string,
	repoID uint64,
	chartName, version string,
	pull ociPullFunc,
) (*Artifact, error) {
	tgz, err := chartTgzOCIWithPull(m, pwd, chartName, version, pull)
	if err != nil {
		return nil, err
	}
	defer wipeChartBytes(tgz)
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
