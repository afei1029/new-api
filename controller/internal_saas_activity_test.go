package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type internalActivityResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Total int              `json:"total"`
		Items []map[string]any `json:"items"`
	} `json:"data"`
}

func queryInternalActivity(t *testing.T, target string) (int, internalActivityResponse) {
	t.Helper()
	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, target, nil, 0)
	ListInternalSaaSActivityLogs(ctx)
	var response internalActivityResponse
	if recorder.Code == http.StatusOK {
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	}
	return recorder.Code, response
}

func TestInternalSaaSActivityLogsReturnsConsumeAndErrorLogsAcrossUsers(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	for _, log := range []model.Log{
		{UserId: 11, Type: model.LogTypeConsume, TokenId: 5, ChannelId: 3, CreatedAt: 1000, RequestId: "req-a", Content: "secret prompt", Ip: "1.2.3.4", Other: `{"model_ratio":2}`},
		{UserId: 12, Type: model.LogTypeError, TokenId: 6, ChannelId: 4, CreatedAt: 1001, RequestId: "req-b", Other: `{"error_type":"openai_error","error_code":"insufficient_quota","admin_info":{"x":1}}`},
		{UserId: 13, Type: model.LogTypeTopup, CreatedAt: 1002},
		{UserId: 14, Type: model.LogTypeConsume, TokenId: 7, CreatedAt: 999},
		{UserId: 15, Type: model.LogTypeConsume, TokenId: 8, CreatedAt: 2001},
	} {
		log := log
		require.NoError(t, db.Create(&log).Error)
	}

	code, response := queryInternalActivity(t, "/api/internal/saas/activity/logs?start_timestamp=1000&end_timestamp=2000&p=1&page_size=100")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, response.Data.Total, "only consume and error logs inside the closed window")
	require.Len(t, response.Data.Items, 2)

	first, second := response.Data.Items[0], response.Data.Items[1]
	assert.EqualValues(t, 11, first["user_id"])
	assert.EqualValues(t, 5, first["token_id"])
	assert.EqualValues(t, 3, first["channel"])
	assert.Equal(t, "req-a", first["request_id"])
	assert.NotContains(t, first, "error_code")
	for _, field := range []string{"content", "ip", "other", "username", "quota"} {
		assert.NotContains(t, first, field, "activity logs must not expose %s", field)
	}
	assert.EqualValues(t, model.LogTypeError, second["type"])
	assert.Equal(t, "openai_error", second["error_type"])
	assert.Equal(t, "insufficient_quota", second["error_code"])
}

func TestInternalSaaSActivityLogsPagesOldestFirst(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	for index := 0; index < 3; index++ {
		log := model.Log{UserId: 20 + index, Type: model.LogTypeConsume, TokenId: 1, CreatedAt: int64(1100 - index)}
		require.NoError(t, db.Create(&log).Error)
	}

	_, page1 := queryInternalActivity(t, "/api/internal/saas/activity/logs?start_timestamp=1000&end_timestamp=2000&p=1&page_size=2")
	_, page2 := queryInternalActivity(t, "/api/internal/saas/activity/logs?start_timestamp=1000&end_timestamp=2000&p=2&page_size=2")
	require.Equal(t, 3, page1.Data.Total)
	require.Len(t, page1.Data.Items, 2)
	require.Len(t, page2.Data.Items, 1)
	assert.EqualValues(t, 1098, page1.Data.Items[0]["created_at"])
	assert.EqualValues(t, 1099, page1.Data.Items[1]["created_at"])
	assert.EqualValues(t, 1100, page2.Data.Items[0]["created_at"])
}

func TestInternalSaaSActivityLogsRejectsInvalidWindows(t *testing.T) {
	setupTokenControllerTestDB(t)
	for _, target := range []string{
		"/api/internal/saas/activity/logs",
		"/api/internal/saas/activity/logs?start_timestamp=2000&end_timestamp=1000",
		"/api/internal/saas/activity/logs?start_timestamp=1&end_timestamp=" + "700000",
	} {
		code, _ := queryInternalActivity(t, target)
		assert.Equal(t, http.StatusBadRequest, code, target)
	}
}
