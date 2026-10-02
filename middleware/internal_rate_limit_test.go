package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func globalAPIRateLimitRouter(t *testing.T, limit int) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	useRateLimitMiniRedis(t)
	previous := []any{common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration}
	common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = true, limit, 60
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = previous[0].(bool)
		common.GlobalApiRateLimitNum = previous[1].(int)
		common.GlobalApiRateLimitDuration = previous[2].(int64)
	})
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.Use(GlobalAPIRateLimit())
	router.GET("/api/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return router
}

func globalAPIRequest(router http.Handler, secret string) int {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	request.RemoteAddr = "192.0.2.20:1234"
	if secret != "" {
		request.Header.Set(internalServiceTokenHeader, secret)
	}
	router.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestGlobalAPIRateLimitSkipsValidSaaSServiceRequests(t *testing.T) {
	t.Setenv("SAAS_INTERNAL_TOKEN", "service-secret")
	router := globalAPIRateLimitRouter(t, 2)

	for i := 0; i < 5; i++ {
		assert.Equal(t, http.StatusNoContent, globalAPIRequest(router, "service-secret"), "service request %d", i)
	}
	// Service requests must not consume the shared per-IP budget.
	assert.Equal(t, http.StatusNoContent, globalAPIRequest(router, ""))
	assert.Equal(t, http.StatusNoContent, globalAPIRequest(router, ""))
	assert.Equal(t, http.StatusTooManyRequests, globalAPIRequest(router, ""))
}

func TestGlobalAPIRateLimitStillLimitsWrongSecret(t *testing.T) {
	t.Setenv("SAAS_INTERNAL_TOKEN", "service-secret")
	router := globalAPIRateLimitRouter(t, 2)

	assert.Equal(t, http.StatusNoContent, globalAPIRequest(router, "wrong"))
	assert.Equal(t, http.StatusNoContent, globalAPIRequest(router, "wrong"))
	assert.Equal(t, http.StatusTooManyRequests, globalAPIRequest(router, "wrong"))
}

func TestGlobalAPIRateLimitDoesNotTrustHeaderWithoutConfiguredSecret(t *testing.T) {
	t.Setenv("SAAS_INTERNAL_TOKEN", "")
	router := globalAPIRateLimitRouter(t, 1)

	assert.Equal(t, http.StatusNoContent, globalAPIRequest(router, "anything"))
	assert.Equal(t, http.StatusTooManyRequests, globalAPIRequest(router, "anything"))
}
