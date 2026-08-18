package repo

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"gorm.io/gorm"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	helmrepo "helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"

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
		return defaultValuesHTTP(ctx, m, pwd, chart, version)
	case "oci":
		return defaultValuesOCI(m, pwd, chart, version)
	default:
		return "", apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type", "unsupported repo type: "+m.Type)
	}
}

// LoadChart fetches the chart .tgz from the upstream repo and parses it into
// a *chart.Chart via helm's loader.LoadArchive. Used by apps/release.Service
// (which holds a release.ChartLoader seam) to materialise charts at install
// and upgrade time. The .tgz is downloaded once into memory and discarded —
// there is no on-disk cache (matches GetDefaultValues semantics).
//
// Errors from the upstream fetch path are normalised via apps.MapError before
// returning, so handler-layer callers never see raw helm/registry text.
func (s *Service) LoadChart(ctx context.Context, repoID uint64, chartName, version string) (*chart.Chart, error) {
	m, err := s.repo.Get(ctx, repoID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	pwd, err := s.decryptPassword(ctx, m)
	if err != nil {
		return nil, err
	}
	var tgz []byte
	switch m.Type {
	case "http":
		tgz, err = chartTgzHTTP(ctx, nil, m, pwd, chartName, version)
	case "oci":
		tgz, err = chartTgzOCI(ctx, m, pwd, chartName, version)
	default:
		return nil, apperr.New(apperr.CodeAppsRepoOther, "apps.repo.unknown_type", "unsupported repo type: "+m.Type)
	}
	if tgz != nil {
		defer wipeChartBytes(tgz)
	}
	if err != nil {
		return nil, err
	}
	ch, lerr := loader.LoadArchive(bytes.NewReader(tgz))
	if lerr != nil {
		return nil, apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_chart", lerr.Error())
	}
	return ch, nil
}

// chartTgzHTTP downloads the raw .tgz for (chartName, version) from an HTTP
// chart repo. Mirrors defaultValuesHTTP's lookup path but returns bytes rather
// than reading values.yaml out.
func chartTgzHTTP(
	ctx context.Context,
	client *http.Client,
	m *models.AppsChartRepo,
	pwd, chartName, version string,
) ([]byte, error) {
	return chartTgzHTTPWithOCI(ctx, client, m, pwd, chartName, version, chartTgzOCIArtifact)
}

type ociArtifactDownloadFunc func(context.Context, string) ([]byte, error)

func chartTgzHTTPWithOCI(
	ctx context.Context,
	client *http.Client,
	m *models.AppsChartRepo,
	pwd, chartName, version string,
	downloadOCI ociArtifactDownloadFunc,
) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	client = secureArtifactHTTPClient(client, m.URL)
	indexBytes, err := downloadHTTPBytes(ctx, client, absoluteURL(m.URL, "index.yaml"), m.URL, m.Username, pwd)
	if indexBytes != nil {
		defer wipeChartBytes(indexBytes)
	}
	if err != nil {
		return nil, err
	}
	idx := helmrepo.NewIndexFile()
	if err := yaml.Unmarshal(indexBytes, idx); err != nil || idx.APIVersion == "" || idx.Entries == nil {
		return nil, apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_index", "invalid chart repository index")
	}
	entries, ok := idx.Entries[chartName]
	if !ok {
		return nil, apperr.New(apperr.CodeAppsRepoChartNotFound, "apps.repo.chart_not_found", chartName)
	}
	var picked *helmrepo.ChartVersion
	for _, e := range entries {
		if e.Version == version {
			picked = e
			break
		}
	}
	if picked == nil {
		return nil, apperr.New(apperr.CodeAppsRepoChartNotFound, "apps.repo.version_not_found", version)
	}
	if len(picked.URLs) == 0 {
		return nil, apperr.New(apperr.CodeAppsRepoInvalidIndex, "apps.repo.bad_index", chartName)
	}
	tgzURL := absoluteURL(m.URL, picked.URLs[0])
	parsed, parseErr := url.Parse(tgzURL)
	if parseErr == nil && strings.EqualFold(parsed.Scheme, "oci") {
		// An HTTP index may point at an artifact in a different OCI registry
		// (Bitnami does this). Pull it anonymously rather than passing the HTTP
		// repository's Basic Auth credentials to another origin.
		return downloadOCI(ctx, tgzURL)
	}
	return downloadHTTPBytes(ctx, client, tgzURL, m.URL, m.Username, pwd)
}

const maxArtifactRedirects = 3

func secureArtifactHTTPClient(base *http.Client, authBase string) *http.Client {
	client := *base
	prior := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !sameOrigin(authBase, req.URL.String()) {
			req.Header.Del("Authorization")
		}
		if len(via) > maxArtifactRedirects {
			return errors.New("chart repository redirect limit exceeded")
		}
		if prior != nil {
			if err := prior(req, via); err != nil {
				return err
			}
		}
		if !sameOrigin(authBase, req.URL.String()) {
			req.Header.Del("Authorization")
		}
		return nil
	}
	return &client
}

func downloadHTTPBytes(ctx context.Context, client *http.Client, rawURL, authBase, username, password string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, apps.MapError(err)
	}
	if (username != "" || password != "") && sameOrigin(authBase, rawURL) {
		req.SetBasicAuth(username, password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, apps.MapError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, apperr.New(apperr.CodeAppsRepoUnauthorized, "apps.repo.unauthorized", "chart repository authentication failed")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, apperr.New(apperr.CodeAppsRepoUnreachable, "apps.repo.unreachable", "chart repository request failed")
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return b, apps.MapError(err)
	}
	return b, nil
}

func sameOrigin(left, right string) bool {
	a, err := url.Parse(left)
	if err != nil {
		return false
	}
	b, err := url.Parse(right)
	if err != nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

// chartTgzOCI pulls the chart .tgz from an OCI registry. Mirrors
// defaultValuesOCI's auth + Pull flow but returns the raw chart bytes.
type ociPullFunc func(context.Context, *registry.Client, string, ...registry.PullOption) (*registry.PullResult, error)

func defaultOCIPull(ctx context.Context, client *registry.Client, ref string, opts ...registry.PullOption) (*registry.PullResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return client.Pull(ref, opts...)
}

func chartTgzOCI(ctx context.Context, m *models.AppsChartRepo, pwd, chartName, version string) ([]byte, error) {
	return chartTgzOCIWithPull(ctx, m, pwd, chartName, version, defaultOCIPull)
}

func chartTgzOCIWithPull(
	ctx context.Context,
	m *models.AppsChartRepo,
	pwd, chartName, version string,
	pullChart ociPullFunc,
) ([]byte, error) {
	ref := ociRef(m.URL, chartName) + ":" + version
	return pullOCIChart(ctx, m.URL, m.Username, pwd, ref, pullChart)
}

func chartTgzOCIArtifact(ctx context.Context, rawRef string) ([]byte, error) {
	ref, err := trimOCIScheme(rawRef)
	if err != nil {
		return nil, err
	}
	return pullOCIChart(ctx, rawRef, "", "", ref, defaultOCIPull)
}

func pullOCIChart(
	ctx context.Context,
	registryURL, username, password, ref string,
	pullChart ociPullFunc,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rc, err := registry.NewClient(registry.ClientOptHTTPClient(&http.Client{
		Transport: contextRoundTripper{ctx: ctx, base: http.DefaultTransport},
	}))
	if err != nil {
		return nil, apps.MapError(err)
	}
	if username != "" || password != "" {
		host := ociHost(registryURL)
		if err := rc.Login(host, registry.LoginOptBasicAuth(username, password)); err != nil {
			return nil, apps.MapError(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pull, err := pullChart(ctx, rc, ref, registry.PullOptWithChart(true))
	var data []byte
	if pull != nil && pull.Chart != nil {
		data = pull.Chart.Data
	}
	if err != nil {
		return data, apps.MapError(err)
	}
	if pull == nil || pull.Chart == nil {
		return nil, apperr.New(apperr.CodeAppsRepoOCIError, "apps.repo.oci_empty_chart", "empty chart pull result")
	}
	return data, nil
}

func trimOCIScheme(raw string) (string, error) {
	const prefix = "oci://"
	if len(raw) < len(prefix) || !strings.EqualFold(raw[:len(prefix)], prefix) {
		return "", apperr.New(apperr.CodeAppsRepoOCIError, "apps.repo.oci_invalid_ref", "invalid OCI chart reference")
	}
	return raw[len(prefix):], nil
}

type contextRoundTripper struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t contextRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.ctx.Err(); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req.Clone(t.ctx))
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

func defaultValuesHTTP(ctx context.Context, m *models.AppsChartRepo, pwd, chart, version string) (string, error) {
	tgz, err := chartTgzHTTP(ctx, nil, m, pwd, chart, version)
	if tgz != nil {
		defer wipeChartBytes(tgz)
	}
	if err != nil {
		return "", err
	}
	return readValuesFromTgz(tgz)
}

// fetchHTTPIndex downloads and parses the upstream index.yaml. The helm SDK
// writes the file to a temp path; we reload it via LoadIndexFile.
func fetchHTTPIndex(m *models.AppsChartRepo, pwd string) (*helmrepo.IndexFile, error) {
	cr, err := helmrepo.NewChartRepository(&helmrepo.Entry{
		Name:     m.Name,
		URL:      m.URL,
		Username: m.Username,
		Password: pwd,
	}, getter.All(cli.New()))
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
