package repo

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"gorm.io/gorm"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	helmrepo "helm.sh/helm/v3/pkg/repo"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/apps"
)

// ListCharts returns chart names available in the upstream repo. For HTTP
// repos it walks index.yaml; for OCI repos it surfaces a single inferred
// chart name because the OCI distribution spec does not expose a portable
// list-artifacts API and helm SDK does not paper over that.
func (s *Service) ListCharts(ctx context.Context, repoID uint64) ([]ChartSummary, error) {
	m, err := s.repo.Get(ctx, repoID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	pwd, err := s.decryptPassword(ctx, m)
	if err != nil {
		return nil, err
	}
	switch m.Type {
	case "http":
		return listHTTP(m, pwd)
	case "oci":
		return listOCI(m, pwd)
	default:
		return nil, apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type", "unsupported repo type: "+m.Type)
	}
}

// ListVersions returns versions for one chart in a repo. HTTP path filters
// index.yaml by chart name; OCI path calls registry.Client.Tags.
func (s *Service) ListVersions(ctx context.Context, repoID uint64, chart string) ([]VersionSummary, error) {
	m, err := s.repo.Get(ctx, repoID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	pwd, err := s.decryptPassword(ctx, m)
	if err != nil {
		return nil, err
	}
	switch m.Type {
	case "http":
		return versionsHTTP(m, pwd, chart)
	case "oci":
		return versionsOCI(m, pwd, chart)
	default:
		return nil, apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type", "unsupported repo type: "+m.Type)
	}
}

// GetDefaultValues fetches the chart's bundled values.yaml as plain text. The
// .tgz is downloaded once, read into memory, and discarded — there is no
// on-disk cache.
func (s *Service) GetDefaultValues(ctx context.Context, repoID uint64, chart, version string) (string, error) {
	m, err := s.repo.Get(ctx, repoID)
	if err != nil {
		return "", mapNotFound(err)
	}
	pwd, err := s.decryptPassword(ctx, m)
	if err != nil {
		return "", err
	}
	switch m.Type {
	case "http":
		return defaultValuesHTTP(m, pwd, chart, version)
	case "oci":
		return defaultValuesOCI(m, pwd, chart, version)
	default:
		return "", apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type", "unsupported repo type: "+m.Type)
	}
}

// mapNotFound translates gorm.ErrRecordNotFound into the apps domain's 40401.
// Any other error is returned unchanged (the caller will pass it through
// apps.MapError once T9 lands).
func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.New(apperr.CodeNotFound, "apps.repo.not_found", "chart repo not found")
	}
	return err
}

// --- HTTP repo path --------------------------------------------------------

func listHTTP(m *models.AppsChartRepo, pwd string) ([]ChartSummary, error) {
	idx, err := fetchHTTPIndex(m, pwd)
	if err != nil {
		return nil, err
	}
	out := make([]ChartSummary, 0, len(idx.Entries))
	for name, versions := range idx.Entries {
		desc := ""
		if len(versions) > 0 && versions[0].Metadata != nil {
			desc = versions[0].Description
		}
		out = append(out, ChartSummary{
			Name:         name,
			VersionCount: len(versions),
			Description:  desc,
		})
	}
	return out, nil
}

func versionsHTTP(m *models.AppsChartRepo, pwd, chart string) ([]VersionSummary, error) {
	idx, err := fetchHTTPIndex(m, pwd)
	if err != nil {
		return nil, err
	}
	entries, ok := idx.Entries[chart]
	if !ok {
		return nil, apperr.New(apperr.CodeAppsRepoChartNotFound, "apps.repo.chart_not_found", chart)
	}
	out := make([]VersionSummary, 0, len(entries))
	for _, e := range entries {
		var appV string
		if e.Metadata != nil {
			appV = e.AppVersion
		}
		out = append(out, VersionSummary{
			Version:    e.Version,
			AppVersion: appV,
			Created:    e.Created.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return out, nil
}

func defaultValuesHTTP(m *models.AppsChartRepo, pwd, chart, version string) (string, error) {
	idx, err := fetchHTTPIndex(m, pwd)
	if err != nil {
		return "", err
	}
	entries, ok := idx.Entries[chart]
	if !ok {
		return "", apperr.New(apperr.CodeAppsRepoChartNotFound, "apps.repo.chart_not_found", chart)
	}
	var picked *helmrepo.ChartVersion
	for _, e := range entries {
		if e.Version == version {
			picked = e
			break
		}
	}
	if picked == nil {
		return "", apperr.New(apperr.CodeAppsRepoChartNotFound, "apps.repo.version_not_found", version)
	}
	if len(picked.URLs) == 0 {
		return "", apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_index", chart)
	}
	tgzURL := absoluteURL(m.URL, picked.URLs[0])
	values, err := downloadValuesYAML(tgzURL, m.Username, pwd)
	if err != nil {
		return "", err
	}
	return values, nil
}

// fetchHTTPIndex downloads and parses the upstream index.yaml. The helm SDK
// writes the file to a temp path; we reload it via LoadIndexFile.
func fetchHTTPIndex(m *models.AppsChartRepo, pwd string) (*helmrepo.IndexFile, error) {
	cr, err := helmrepo.NewChartRepository(&helmrepo.Entry{
		Name:     m.Name,
		URL:      m.URL,
		Username: m.Username,
		Password: pwd,
	}, getter.All(nil))
	if err != nil {
		return nil, apps.MapError(err)
	}
	idxPath, err := cr.DownloadIndexFile()
	if err != nil {
		return nil, apps.MapError(err)
	}
	idx, err := helmrepo.LoadIndexFile(idxPath)
	if err != nil {
		return nil, apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_index", err.Error())
	}
	return idx, nil
}

// absoluteURL resolves a possibly-relative chart URL against the repo base.
func absoluteURL(repoBase, chartURL string) string {
	u, err := url.Parse(chartURL)
	if err == nil && u.IsAbs() {
		return chartURL
	}
	base := strings.TrimRight(repoBase, "/")
	return base + "/" + strings.TrimLeft(chartURL, "/")
}

// downloadValuesYAML fetches the .tgz over HTTP and extracts values.yaml.
func downloadValuesYAML(tgzURL, username, password string) (string, error) {
	opts := []getter.Option{}
	if username != "" || password != "" {
		opts = append(opts, getter.WithBasicAuth(username, password))
	}
	g, err := getter.NewHTTPGetter(opts...)
	if err != nil {
		return "", apps.MapError(err)
	}
	buf, err := g.Get(tgzURL)
	if err != nil {
		return "", apps.MapError(err)
	}
	return readValuesFromTgz(buf.Bytes())
}

// --- OCI repo path ---------------------------------------------------------

// listOCI returns a best-effort single-chart result. OCI does not expose a
// "list charts in registry" API in the helm SDK; this surfaces the URL's
// trailing path segment so the UI has at least one entry to click on. The
// real-world expectation is users will type/select the chart name explicitly.
func listOCI(m *models.AppsChartRepo, _ string) ([]ChartSummary, error) {
	parsed := strings.TrimPrefix(m.URL, "oci://")
	parts := strings.Split(strings.TrimRight(parsed, "/"), "/")
	if len(parts) < 2 {
		return []ChartSummary{}, nil // registry root — cannot enumerate
	}
	name := parts[len(parts)-1]
	return []ChartSummary{{Name: name, VersionCount: 0, Description: ""}}, nil
}

func versionsOCI(m *models.AppsChartRepo, pwd, chart string) ([]VersionSummary, error) {
	rc, err := registry.NewClient()
	if err != nil {
		return nil, apps.MapError(err)
	}
	if m.Username != "" || pwd != "" {
		host := ociHost(m.URL)
		if err := rc.Login(host, registry.LoginOptBasicAuth(m.Username, pwd)); err != nil {
			return nil, apps.MapError(err)
		}
	}
	ref := ociRef(m.URL, chart)
	tags, err := rc.Tags(ref)
	if err != nil {
		return nil, apps.MapError(err)
	}
	out := make([]VersionSummary, 0, len(tags))
	for _, t := range tags {
		out = append(out, VersionSummary{Version: t})
	}
	return out, nil
}

func defaultValuesOCI(m *models.AppsChartRepo, pwd, chart, version string) (string, error) {
	rc, err := registry.NewClient()
	if err != nil {
		return "", apps.MapError(err)
	}
	if m.Username != "" || pwd != "" {
		host := ociHost(m.URL)
		if err := rc.Login(host, registry.LoginOptBasicAuth(m.Username, pwd)); err != nil {
			return "", apps.MapError(err)
		}
	}
	ref := ociRef(m.URL, chart) + ":" + version
	pull, err := rc.Pull(ref, registry.PullOptWithChart(true))
	if err != nil {
		return "", apps.MapError(err)
	}
	if pull == nil || pull.Chart == nil {
		return "", apperr.New(apperr.CodeAppsRepoOCIError, "apps.repo.oci_empty_chart", "empty chart pull result")
	}
	return readValuesFromTgz(pull.Chart.Data)
}

// ociHost returns the registry host portion of an oci:// URL (everything
// between the scheme and the first '/').
func ociHost(u string) string {
	host := strings.TrimPrefix(u, "oci://")
	if i := strings.Index(host, "/"); i > 0 {
		host = host[:i]
	}
	return host
}

// ociRef builds the chart reference (no tag) by appending /<chart> to the
// repo URL's path if it's not already the trailing segment.
func ociRef(u, chart string) string {
	ref := strings.TrimPrefix(u, "oci://")
	ref = strings.TrimRight(ref, "/")
	if !strings.HasSuffix(ref, "/"+chart) {
		ref = ref + "/" + chart
	}
	return ref
}
