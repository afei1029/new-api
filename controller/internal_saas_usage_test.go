package controller

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInternalSaaSUsageFindsBillingRecordByRequestIDForTargetUserOnly(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	now := common.GetTimestamp()
	for _, log := range []model.Log{
		{UserId: 11, Type: model.LogTypeConsume, TokenId: 5, ModelName: "gpt-image-2", RequestId: "req-target", Quota: 1500, CreatedAt: now},
		{UserId: 11, Type: model.LogTypeConsume, TokenId: 5, ModelName: "gpt-image-2", RequestId: "req-other", Quota: 999, CreatedAt: now},
		// Same request ID under a different NewAPI user must never be returned.
		{UserId: 12, Type: model.LogTypeConsume, TokenId: 5, ModelName: "gpt-image-2", RequestId: "req-foreign", Quota: 7, CreatedAt: now},
	} {
		log := log
		require.NoError(t, db.Create(&log).Error)
	}

	query := func(userID int, requestID string) []map[string]any {
		target := "/api/internal/saas?p=1&page_size=10&token_ids=5&model_name=gpt-image-2&request_id=" + requestID
		ctx, recorder := newAuthenticatedContext(t, http.MethodGet, target, nil, 0)
		ctx.Params = gin.Params{{Key: "user_id", Value: strconv.Itoa(userID)}}
		require.NoError(t, db.AutoMigrate(&model.User{}))
		db.Exec("INSERT OR IGNORE INTO users (id, username, password, aff_code) VALUES (?, ?, 'password', ?)", userID, "usage-"+strconv.Itoa(userID), "aff-"+strconv.Itoa(userID))
		ListInternalSaaSUsage(ctx)
		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Data struct {
				Items []map[string]any `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Data.Items
	}

	items := query(11, "req-target")
	require.Len(t, items, 1)
	assert.Equal(t, "req-target", items[0]["request_id"])
	assert.EqualValues(t, 1500, items[0]["quota"])

	assert.Empty(t, query(11, "req-foreign"), "another user's request ID must not be visible")
}
