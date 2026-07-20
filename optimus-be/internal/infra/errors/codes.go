package errors

// Code is the business-level numeric error code returned in response envelope.
type Code int

const (
	CodeOK Code = 0

	// 1xxxx system-level
	CodeInternal     Code = 10001
	CodeDBError      Code = 10002
	CodeTimeout      Code = 10003
	CodeUnauthorized Code = 10004 // generic auth failure (used internally)

	// 4xxxx client errors (mirror HTTP 4xx)
	CodeBadRequest           Code = 40001
	CodeValidation           Code = 40002
	CodeInvalidCredentials   Code = 40101
	CodeTokenInvalid         Code = 40102
	CodeTokenExpired         Code = 40103
	CodeRefreshTokenReplay   Code = 40104
	CodeForbidden            Code = 40301
	CodePermissionDenied     Code = 40302
	CodeNotFound             Code = 40401
	CodeConflict             Code = 40901
	CodeUserAlreadyExists    Code = 40902
	CodeRoleAlreadyExists    Code = 40903
	CodeMenuAlreadyExists    Code = 40904
	CodeBuiltinRoleImmutable Code = 40905
	CodeCannotDeleteSelf     Code = 40906
	CodeCannotDeleteAdmin    Code = 40907
	CodeRateLimited          Code = 42901

	// 5xxxx server business errors
	CodeSeedFailed      Code = 50001
	CodePermRegistryErr Code = 50002

	// 41xxx k8s runtime — runtime failures reaching or talking to apiserver.
	// Distinct from 40xxx client errors because they encode upstream-dependency
	// state, not malformed/unauthorized client requests. See P2 spec §9.
	CodeClusterUnreachable    Code = 41101 // network/timeout reaching apiserver
	CodeAPIServerForbidden    Code = 41103 // kubeconfig user's RBAC denies the call
	CodeAPIServerUnauthorized Code = 41104 // kubeconfig credentials expired/invalid
	CodeAPIServerOther        Code = 41105 // generic apiserver StatusError
	CodeLogUnavailable        Code = 41202 // pod log unavailable (pending/init/no previous)

	// 42xxx P3 apps domain — chart repo upstream + helm release runtime.
	// Distinct from 40xxx mirror because these encode upstream-helm/registry
	// dependency state, not malformed client requests. See P3 spec §5.
	//
	// 42001-42099 apps generic
	CodeAppsApplicationInUse            Code = 42001 // delete blocked: helm release still present
	CodeAppsChartRepoInUse              Code = 42002 // delete blocked: still referenced by application(s)
	CodeAppsReleaseNameDuplicate        Code = 42003 // (cluster_id,namespace,release_name) collision in DB
	CodeAppsApplicationOnDeletedCluster Code = 42004 // referenced cluster is soft-deleted

	// 42101-42199 chart repo upstream
	CodeAppsRepoUnreachable   Code = 42101 // network/DNS/TLS failure
	CodeAppsRepoUnauthorized  Code = 42102 // 401/403 from OCI or HTTP repo
	CodeAppsRepoChartNotFound Code = 42103 // chart name or version missing
	CodeAppsRepoInvalidIndex  Code = 42104 // HTTP repo index.yaml parse failure
	CodeAppsRepoOCIError      Code = 42105 // OCI manifest/blob fetch error
	CodeAppsRepoOther         Code = 42199 // other upstream error

	// 42201-42299 helm release runtime
	CodeAppsReleaseAlreadyExists   Code = 42201 // install: release already exists
	CodeAppsReleaseNotFound        Code = 42202 // upgrade/rollback/uninstall/status: helm secret missing
	CodeAppsReleaseHistoryTooShort Code = 42203 // rollback target revision missing
	CodeAppsReleaseStillPresent    Code = 42204 // application delete blocked: helm secret still exists
	CodeAppsReleaseInvalidValues   Code = 42205 // values yaml parse error / not a map
	CodeAppsReleaseOther           Code = 42299 // other helm SDK error

	// 43xxx P4 assets domain — cloud asset discovery + AWS SDK runtime.
	// See P4 spec §9. 43001-43099 cloud-account domain; 43100-43199 sync/AWS.
	// 43200+ reserved for future P4.x GCP/Azure provider expansion.

	// 43001-43099 cloud account / domain
	CodeAssetsCloudAccountInUse        Code = 43001 // cloudkey delete blocked: referenced by CloudAccount
	CodeAssetsCloudAccountNotFound     Code = 43002
	CodeAssetsCloudAccountNameConflict Code = 43003
	CodeAssetsRegionInvalid            Code = 43004 // region string fails AWS regex
	CodeAssetsProviderUnsupported      Code = 43005 // provider != "aws" at MVP
	CodeAssetsCloudAccountDisabled     Code = 43006 // manual sync on enabled=false account
	CodeAssetsCloudKeyNotFound         Code = 43008 // Create/Update references missing cloudkey_id
	CodeAssetsVPCNotFound              Code = 43009 // /vpcs/{id}/subnets row lookup miss
	// (43007 reserved)

	// 43100-43199 sync / AWS
	CodeAssetsSyncBusy        Code = 43101 // manual sync invoked while account lock held
	CodeAssetsAWSUnauthorized Code = 43102 // AuthFailure / InvalidClientTokenId / SignatureDoesNotMatch / ExpiredToken
	CodeAssetsAWSForbidden    Code = 43103 // UnauthorizedOperation / AccessDenied
	CodeAssetsAWSUnreachable  Code = 43104 // ctx deadline / DNS / net error / RequestCanceled
	CodeAssetsAWSThrottled    Code = 43105 // Throttling / RequestLimitExceeded after SDK retries
	CodeAssetsAWSOther        Code = 43106 // any other API error
	CodeAssetsAWSConfig       Code = 43107 // config.LoadDefaultConfig failed

	// 44xxx P5 observability domain — data sources, bounded Prometheus queries,
	// and dashboards. Message keys are observability.<area>.<reason>.
	// 44001-44099 data sources
	CodeObservabilityDatasourceNotFound     Code = 44001 // observability.datasource.not_found
	CodeObservabilityDatasourceNameTaken    Code = 44002 // observability.datasource.name_taken
	CodeObservabilityDatasourceInUse        Code = 44003 // observability.datasource.in_use
	CodeObservabilityDatasourceInvalidURL   Code = 44004 // observability.datasource.invalid_url
	CodeObservabilityDatasourceAuthMismatch Code = 44005 // observability.datasource.auth_mismatch
	CodeObservabilityDatasourceInvalidTLS   Code = 44006 // observability.datasource.invalid_tls

	// 44101-44199 Prometheus queries
	CodeObservabilityQueryDestinationDenied   Code = 44101 // observability.query.destination_denied
	CodeObservabilityQueryUpstreamUnreachable Code = 44102 // observability.query.upstream_unreachable
	CodeObservabilityQueryUpstreamTimeout     Code = 44103 // observability.query.upstream_timeout
	CodeObservabilityQueryUpstreamRejected    Code = 44104 // observability.query.upstream_rejected
	CodeObservabilityQueryInvalidResponse     Code = 44105 // observability.query.invalid_response
	CodeObservabilityQueryLimitExceeded       Code = 44106 // observability.query.limit_exceeded
	CodeObservabilityQueryInvalidRequest      Code = 44107 // observability.query.invalid_request

	// 44201-44299 dashboards
	CodeObservabilityDashboardNotFound        Code = 44201 // observability.dashboard.not_found
	CodeObservabilityDashboardNameTaken       Code = 44202 // observability.dashboard.name_taken
	CodeObservabilityDashboardInvalidPanel    Code = 44203 // observability.dashboard.invalid_panel
	CodeObservabilityDashboardBuiltinNotFound Code = 44204 // observability.dashboard.builtin_not_found
)
