package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
)

// TestFormatUserLogsStripsQuotaSaturation verifies the admin-only quota
// saturation marker (nested under other.admin_info) is removed for non-admin
// log views, since formatUserLogs strips the whole admin_info object.
func TestFormatUserLogsStripsQuotaSaturation(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"model_price": 0.004,
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromDecimal",
				"kind":    "overflow",
				"clamped": common.MaxQuota,
			},
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := parsed["admin_info"]
	require.False(t, hasAdminInfo, "admin_info (and nested quota_saturation) must be stripped for non-admin views")
	// Non-admin billing fields remain visible.
	require.Contains(t, parsed, "model_price")
}

func TestUserTokenExpressionsNormalizeCachedInput(t *testing.T) {
	input, cacheRead, cacheWrite := userTokenExpressions()

	require.Contains(t, input, "usage_semantic")
	require.Contains(t, input, "prompt_tokens_excludes_cache")
	require.Contains(t, input, "GREATEST(logs.prompt_tokens - "+cacheRead+", 0)")
	require.Contains(t, cacheRead, "cache_tokens")
	require.True(t, strings.Index(cacheWrite, "cache_write_tokens") < strings.Index(cacheWrite, "cache_creation_tokens_5m"))
	require.Contains(t, cacheWrite, "cache_creation_tokens_1h")
	require.Contains(t, cacheWrite, "cache_creation_tokens")
}
