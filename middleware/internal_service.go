package middleware

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const internalServiceTokenHeader = "X-SaaS-Internal-Token"

// isSaaSInternalRequest reports whether the request carries the configured
// SaaS service-to-service secret. It fails closed when no secret is set.
func isSaaSInternalRequest(c *gin.Context) bool {
	configured := strings.TrimSpace(os.Getenv("SAAS_INTERNAL_TOKEN"))
	provided := strings.TrimSpace(c.GetHeader(internalServiceTokenHeader))
	return configured != "" && provided != "" && subtle.ConstantTimeCompare([]byte(configured), []byte(provided)) == 1
}

// SaaSInternalAuth protects the fixed service-to-service API used by the SaaS
// backend. It intentionally fails closed when the shared secret is missing.
func SaaSInternalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isSaaSInternalRequest(c) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"code":    "INTERNAL_SERVICE_UNAUTHORIZED",
				"message": "internal service authentication required",
			})
			return
		}
		c.Next()
	}
}
