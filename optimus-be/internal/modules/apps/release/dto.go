// Package release wraps the helm SDK action.* surface (install, upgrade,
// rollback, uninstall, status, history) behind a Gin handler that always
// roots writes at an Optimus-registered application. Operations on
// unregistered helm releases are not possible by API design (see P3 spec §6).
package release

// InstallRequest is the body of POST /apps/applications/:id/release/install.
type InstallRequest struct {
	ChartVersion string `json:"chart_version" binding:"required,max=64"`
	ValuesYAML   string `json:"values_yaml"   binding:"max=1048576"` // 1 MiB cap
}

// UpgradeRequest is the body of POST /apps/applications/:id/release/upgrade.
// ChartRepoID is optional — when present and different from the current
// chart_repo_id, the application row is repointed atomically with the helm
// upgrade (see Service.Upgrade).
type UpgradeRequest struct {
	ChartRepoID  *uint64 `json:"chart_repo_id,omitempty"`
	ChartVersion string  `json:"chart_version" binding:"required,max=64"`
	ValuesYAML   string  `json:"values_yaml"   binding:"max=1048576"`
}

// RollbackRequest selects the revision to roll back to. Revision must be a
// positive integer present in helm history; missing revisions return
// CodeAppsReleaseHistoryTooShort (42203).
type RollbackRequest struct {
	Revision int `json:"revision" binding:"required,min=1"`
}

// UninstallRequest controls whether helm history is retained after uninstall.
// KeepHistory=true keeps the helm secrets so a subsequent rollback is
// possible; false removes them.
type UninstallRequest struct {
	KeepHistory bool `json:"keep_history"`
}

// ReleaseStatus is the live state of a helm release.
type ReleaseStatus struct {
	Status         string `json:"status"` // deployed|failed|pending|unknown
	Revision       int    `json:"revision"`
	ChartVersion   string `json:"chart_version"`
	AppVersion     string `json:"app_version"`
	LastDeployedAt string `json:"last_deployed_at"`
	Notes          string `json:"notes,omitempty"`
}

// RevisionRow is one entry of helm history.
type RevisionRow struct {
	Revision     int    `json:"revision"`
	Status       string `json:"status"`
	ChartVersion string `json:"chart_version"`
	AppVersion   string `json:"app_version"`
	UpdatedAt    string `json:"updated_at"`
	Description  string `json:"description"`
}

// InstallResult / UpgradeResult / RollbackResult are returned by writes.
type InstallResult struct {
	Revision       int    `json:"revision"`
	Status         string `json:"status"`
	ChartVersion   string `json:"chart_version"`
	LastDeployedAt string `json:"last_deployed_at"`
}

// UpgradeResult mirrors InstallResult — both writes share a result shape.
type UpgradeResult = InstallResult

// RollbackResult mirrors InstallResult — read back via Status after the helm
// action returns (helm's Rollback.Run is void on success).
type RollbackResult = InstallResult
