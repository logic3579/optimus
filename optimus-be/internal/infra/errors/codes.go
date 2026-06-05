package errors

// Code is the business-level numeric error code returned in response envelope.
type Code int

const (
	CodeOK Code = 0

	// 1xxxx system-level
	CodeInternal       Code = 10001
	CodeDBError        Code = 10002
	CodeTimeout        Code = 10003
	CodeUnauthorized   Code = 10004 // generic auth failure (used internally)

	// 4xxxx client errors (mirror HTTP 4xx)
	CodeBadRequest             Code = 40001
	CodeValidation             Code = 40002
	CodeInvalidCredentials     Code = 40101
	CodeTokenInvalid           Code = 40102
	CodeTokenExpired           Code = 40103
	CodeRefreshTokenReplay     Code = 40104
	CodeForbidden              Code = 40301
	CodePermissionDenied       Code = 40302
	CodeNotFound               Code = 40401
	CodeConflict               Code = 40901
	CodeUserAlreadyExists      Code = 40902
	CodeRoleAlreadyExists      Code = 40903
	CodeMenuAlreadyExists      Code = 40904
	CodeBuiltinRoleImmutable   Code = 40905
	CodeCannotDeleteSelf       Code = 40906
	CodeCannotDeleteAdmin      Code = 40907
	CodeRateLimited            Code = 42901

	// 5xxxx server business errors
	CodeSeedFailed       Code = 50001
	CodePermRegistryErr  Code = 50002
)
