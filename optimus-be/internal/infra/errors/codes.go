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
)
