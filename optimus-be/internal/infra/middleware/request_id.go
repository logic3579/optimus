package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const headerRequestID = "X-Request-ID"

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(headerRequestID)
		if rid == "" {
			rid = newRequestID()
		}
		c.Set(CtxKeyRequestID, rid)
		c.Writer.Header().Set(headerRequestID, rid)
		c.Next()
	}
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
