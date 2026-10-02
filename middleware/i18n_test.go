package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForceEnglishOverridesUserAndRequestLanguageForGatewayErrors(t *testing.T) {
	require.NoError(t, appI18n.Init())
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{Language: appI18n.LangZhCN})
		c.Next()
	})
	router.Use(ForceEnglish())
	router.GET("/v1/test", func(c *gin.Context) {
		c.String(http.StatusServiceUnavailable, appI18n.T(c, appI18n.MsgDistributorNoAvailableChannel, map[string]any{
			"Group": "renamed-group",
			"Model": "test-model",
		}))
	})

	request := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	request.Header.Set("Accept-Language", "zh-CN")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Equal(t, "No available channel for model test-model under group renamed-group (distributor)", response.Body.String())
}

func noChannelRouterForTest(tag string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{Language: appI18n.LangZhCN})
		c.Next()
	})
	router.Use(RouteTag(tag))
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusServiceUnavailable, appI18n.T(c, appI18n.MsgDistributorNoAvailableChannel, map[string]any{
			"Group": "Claude_B",
			"Model": "gpt-6.1-sol",
		}))
	})
	return router
}

// Every gateway route is tagged "relay"; tagging alone must force English so
// relay routes added by upstream cannot fall back to the user's language.
func TestRelayRouteTagForcesEnglishGatewayErrors(t *testing.T) {
	require.NoError(t, appI18n.Init())
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.Header.Set("Accept-Language", "zh-CN")
	response := httptest.NewRecorder()
	noChannelRouterForTest(RouteTagRelay).ServeHTTP(response, request)

	require.Equal(t, "No available channel for model gpt-6.1-sol under group Claude_B (distributor)", response.Body.String())
}

func TestDashboardRouteTagKeepsUserLanguage(t *testing.T) {
	require.NoError(t, appI18n.Init())
	response := httptest.NewRecorder()
	noChannelRouterForTest("api").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/test", nil))

	require.Equal(t, "分组 Claude_B 下模型 gpt-6.1-sol 无可用渠道（distributor）", response.Body.String())
}
