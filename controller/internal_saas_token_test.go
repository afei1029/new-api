package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newInternalSaaSTokenContext builds a request without dashboard login
// context; the target user comes only from the route parameter.
func newInternalSaaSTokenContext(t *testing.T, method string, body any, userID int, tokenID int) *gin.Context {
	t.Helper()
	ctx, _ := newInternalSaaSTokenContextWithRecorder(t, method, body, userID, tokenID)
	return ctx
}

func newInternalSaaSTokenContextWithRecorder(t *testing.T, method string, body any, userID int, tokenID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	ctx, recorder := newAuthenticatedContext(t, method, "/api/internal/saas", body, 0)
	ctx.Params = gin.Params{{Key: "user_id", Value: strconv.Itoa(userID)}}
	if tokenID > 0 {
		ctx.Params = append(ctx.Params, gin.Param{Key: "token_id", Value: strconv.Itoa(tokenID)})
	}
	return ctx, recorder
}

func TestInternalSaaSCreateTokenAppliesSharedAutoGroupRules(t *testing.T) {
	configureTokenAutoGroupsTest(t, "5", `["default","vip"]`)
	user := setupTokenAutoGroupsControllerTest(t)

	request := baseAutoTokenRequest("internal-auto")
	request["auto_groups"] = []string{"vip", "default"}
	ctx := newInternalSaaSTokenContext(t, http.MethodPost, request, user.Id, 0)
	CreateInternalSaaSToken(ctx)
	require.Equal(t, http.StatusOK, ctx.Writer.Status())

	var token model.Token
	require.NoError(t, model.DB.Where("name = ?", "internal-auto").First(&token).Error)
	assert.True(t, token.CrossGroupRetry, "auto group retry must be preserved like the dashboard API")
	assert.JSONEq(t, `["vip","default"]`, token.AutoGroups)
}

func TestInternalSaaSCreateTokenRejectsInvalidAutoGroupsAndQuota(t *testing.T) {
	configureTokenAutoGroupsTest(t, "5", `["default","vip"]`)
	user := setupTokenAutoGroupsControllerTest(t)

	invalidGroups := baseAutoTokenRequest("internal-invalid")
	invalidGroups["auto_groups"] = []string{"missing"}
	ctx := newInternalSaaSTokenContext(t, http.MethodPost, invalidGroups, user.Id, 0)
	CreateInternalSaaSToken(ctx)
	assert.Equal(t, http.StatusBadRequest, ctx.Writer.Status())

	overQuota := map[string]any{
		"name":            "internal-over-quota",
		"group":           "default",
		"expired_time":    -1,
		"unlimited_quota": false,
		"remain_quota":    -(service.MaxTokenQuota() + 1),
	}
	ctx = newInternalSaaSTokenContext(t, http.MethodPost, overQuota, user.Id, 0)
	CreateInternalSaaSToken(ctx)
	assert.Equal(t, http.StatusBadRequest, ctx.Writer.Status())

	var count int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestInternalSaaSUpdateTokenStatusRejectsExpiredToken(t *testing.T) {
	configureTokenAutoGroupsTest(t, "5", `["default","vip"]`)
	user := setupTokenAutoGroupsControllerTest(t)
	token := seedToken(t, model.DB, user.Id, "expired", "internal-expired-key")
	require.NoError(t, model.DB.Model(token).Updates(map[string]any{
		"status":       common.TokenStatusExpired,
		"expired_time": common.GetTimestamp() - 60,
	}).Error)

	ctx := newInternalSaaSTokenContext(t, http.MethodPatch, map[string]any{"status": common.TokenStatusEnabled}, user.Id, token.Id)
	UpdateInternalSaaSToken(ctx)
	assert.Equal(t, http.StatusBadRequest, ctx.Writer.Status())

	var stored model.Token
	require.NoError(t, model.DB.First(&stored, token.Id).Error)
	assert.Equal(t, common.TokenStatusExpired, stored.Status)
}

func TestInternalSaaSUpdateTokenNonAutoClearsAutoGroups(t *testing.T) {
	configureTokenAutoGroupsTest(t, "5", `["default","vip"]`)
	user := setupTokenAutoGroupsControllerTest(t)
	token := seedToken(t, model.DB, user.Id, "auto", "internal-auto-key")
	token.Group = "auto"
	token.CrossGroupRetry = true
	require.NoError(t, token.SetAutoGroups([]string{"vip"}))
	require.NoError(t, model.DB.Save(token).Error)

	request := map[string]any{
		"name":            "renamed",
		"group":           "default",
		"expired_time":    -1,
		"unlimited_quota": true,
	}
	ctx := newInternalSaaSTokenContext(t, http.MethodPatch, request, user.Id, token.Id)
	UpdateInternalSaaSToken(ctx)
	require.Equal(t, http.StatusOK, ctx.Writer.Status())

	var stored model.Token
	require.NoError(t, model.DB.First(&stored, token.Id).Error)
	assert.Equal(t, "renamed", stored.Name)
	assert.Equal(t, "default", stored.Group)
	assert.False(t, stored.CrossGroupRetry)
	assert.Empty(t, stored.AutoGroups)
}

func TestInternalSaaSGetTokenReturnsFullNonSecretFields(t *testing.T) {
	configureTokenAutoGroupsTest(t, "5", `["default","vip"]`)
	user := setupTokenAutoGroupsControllerTest(t)
	token := seedToken(t, model.DB, user.Id, "full-view", "internal-full-view-key")
	token.Group = "auto"
	token.CrossGroupRetry = true
	token.UsedQuota = 321
	token.AccessedTime = 1700000000
	require.NoError(t, token.SetAutoGroups([]string{"vip", "default"}))
	require.NoError(t, model.DB.Save(token).Error)

	ctx, recorder := newInternalSaaSTokenContextWithRecorder(t, http.MethodGet, nil, user.Id, token.Id)
	GetInternalSaaSToken(ctx)
	require.Equal(t, http.StatusOK, ctx.Writer.Status())

	var response struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, true, response.Data["cross_group_retry"])
	assert.Equal(t, []any{"vip", "default"}, response.Data["auto_groups"])
	assert.EqualValues(t, 321, response.Data["used_quota"])
	assert.EqualValues(t, 1700000000, response.Data["accessed_time"])
	assert.NotEqual(t, "internal-full-view-key", response.Data["key"], "read APIs must return a masked key")
}

func TestInternalSaaSBatchDeleteOnlyDeletesTargetUserTokens(t *testing.T) {
	configureTokenAutoGroupsTest(t, "5", `["default","vip"]`)
	user := setupTokenAutoGroupsControllerTest(t)
	first := seedToken(t, model.DB, user.Id, "first", "internal-batch-first")
	second := seedToken(t, model.DB, user.Id, "second", "internal-batch-second")
	foreign := seedToken(t, model.DB, user.Id+1, "foreign", "internal-batch-foreign")

	ctx := newInternalSaaSTokenContext(t, http.MethodPost, map[string]any{"ids": []int{first.Id, second.Id, foreign.Id}}, user.Id, 0)
	DeleteInternalSaaSTokens(ctx)
	require.Equal(t, http.StatusOK, ctx.Writer.Status())

	var remaining []model.Token
	require.NoError(t, model.DB.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	assert.Equal(t, foreign.Id, remaining[0].Id)
}

func TestInternalSaaSBatchDeleteRejectsInvalidIDs(t *testing.T) {
	user := setupTokenAutoGroupsControllerTest(t)
	tooMany := make([]int, internalSaaSTokenBatchLimit+1)
	for index := range tooMany {
		tooMany[index] = index + 1
	}
	for _, ids := range [][]int{{}, {0}, tooMany} {
		ctx := newInternalSaaSTokenContext(t, http.MethodPost, map[string]any{"ids": ids}, user.Id, 0)
		DeleteInternalSaaSTokens(ctx)
		assert.Equal(t, http.StatusBadRequest, ctx.Writer.Status())
	}
}

func TestInternalSaaSTokenAllowsNegativeRemainingQuota(t *testing.T) {
	configureTokenAutoGroupsTest(t, "5", `["default","vip"]`)
	user := setupTokenAutoGroupsControllerTest(t)

	request := map[string]any{
		"name":            "negative-quota",
		"group":           "default",
		"expired_time":    -1,
		"unlimited_quota": false,
		"remain_quota":    -1000,
	}
	ctx := newInternalSaaSTokenContext(t, http.MethodPost, request, user.Id, 0)
	CreateInternalSaaSToken(ctx)
	require.Equal(t, http.StatusOK, ctx.Writer.Status())

	var token model.Token
	require.NoError(t, model.DB.Where("name = ?", "negative-quota").First(&token).Error)
	assert.Equal(t, -1000, token.RemainQuota)

	request["remain_quota"] = -2000
	ctx = newInternalSaaSTokenContext(t, http.MethodPatch, request, user.Id, token.Id)
	UpdateInternalSaaSToken(ctx)
	require.Equal(t, http.StatusOK, ctx.Writer.Status())
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, -2000, token.RemainQuota)
}
