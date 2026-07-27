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
