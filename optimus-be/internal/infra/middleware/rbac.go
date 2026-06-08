package middleware

import (
	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/infra/response"
	"optimus-be/internal/modules/rbac"
)

// RequirePermission rejects requests whose authenticated user lacks the given permission code.
// Must come AFTER JWTAuth in the middleware chain.
func RequirePermission(cache *rbac.PermissionCache, code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetUint64(CtxKeyUserID)
		if uid == 0 {
			response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.unauthenticated", "authentication required"))
			c.Abort()
			return
		}
		codes, err := cache.Get(c.Request.Context(), uid)
		if err != nil {
			response.Error(c, apperr.Wrap(err, apperr.CodeInternal, "common.internal", "permission lookup failed"))
			c.Abort()
			return
		}
		for _, p := range codes {
			if p == code {
				c.Next()
				return
			}
		}
		response.Error(c, apperr.New(apperr.CodePermissionDenied, "auth.permission_denied", "missing permission: "+code))
		c.Abort()
	}
}
