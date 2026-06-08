package middleware

// Context key strings used across middleware to set/read request-scoped values.
const (
	CtxKeyRequestID = "x-request-id"
	CtxKeyUserID    = "x-user-id"
	CtxKeyLang      = "x-lang"
)
