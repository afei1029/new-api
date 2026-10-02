package middleware

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
)

const RouteTagKey = "route_tag"

// RouteTagRelay marks model gateway routes (/v1, /v1beta, /mj, /pg, task
// plugin endpoints, ...).
const RouteTagRelay = "relay"

func RouteTag(tag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RouteTagKey, tag)
		// SaaS customization: gateway errors are always English, regardless of
		// the NewAPI user's language setting or the Accept-Language header.
		// Every gateway route is tagged "relay", so forcing English here also
		// covers relay routes added by upstream. Dashboard routes keep i18n.
		if tag == RouteTagRelay {
			c.Set(string(constant.ContextKeyLanguageOverride), i18n.LangEn)
		}
		c.Next()
	}
}

func SetUpLogger(server *gin.Engine) {
	server.Use(redactTaskArtifactAccessQuery())
	server.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var requestID string
		if param.Keys != nil {
			requestID, _ = param.Keys[common.RequestIdKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		path := param.Path
		// OAuth callbacks carry one-time codes and state in the query string.
		// Redact the log value only; the handler still needs the original query.
		if strings.HasPrefix(path, "/api/oauth/") || strings.HasPrefix(path, "/oauth/") {
			path, _, _ = strings.Cut(path, "?")
		}
		return fmt.Sprintf("[GIN] %s | %s | %s | %3d | %13v | %15s | %7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			tag,
			requestID,
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			path,
		)
	}))
}
