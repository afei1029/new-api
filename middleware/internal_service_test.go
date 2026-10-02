package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func runInternalAuthTest(t *testing.T, configured, provided string) *httptest.ResponseRecorder {
	t.Helper()
	t.Setenv("SAAS_INTERNAL_TOKEN", configured)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SaaSInternalAuth())
	router.GET("/internal", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/internal", nil)
	if provided != "" {
		req.Header.Set(internalServiceTokenHeader, provided)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func TestSaaSInternalAuthFailsClosedWhenNotConfigured(t *testing.T) {
	resp := runInternalAuthTest(t, "", "secret")
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func TestSaaSInternalAuthRejectsMissingOrWrongToken(t *testing.T) {
	for _, provided := range []string{"", "wrong"} {
		resp := runInternalAuthTest(t, "secret", provided)
		if resp.Code != http.StatusUnauthorized {
			t.Errorf("provided %q: status = %d, want %d", provided, resp.Code, http.StatusUnauthorized)
		}
	}
}

func TestSaaSInternalAuthAcceptsExactToken(t *testing.T) {
	resp := runInternalAuthTest(t, "secret", "secret")
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
}
