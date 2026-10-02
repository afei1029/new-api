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

func setupInternalBalanceTest(t *testing.T, quota int) *model.User {
	t.Helper()
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	user := &model.User{Id: 301, Username: "wallet-user", Password: "password", Group: "default", Status: common.UserStatusEnabled, Quota: quota, AffCode: "wallet-aff"}
	require.NoError(t, db.Create(user).Error)
	return user
}

func adjustInternalBalanceForTest(t *testing.T, userID int, mode string, value int) (int, map[string]any) {
	t.Helper()
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/internal/saas", map[string]any{"mode": mode, "value": value}, 0)
	ctx.Params = gin.Params{{Key: "user_id", Value: strconv.Itoa(userID)}}
	AdjustInternalSaaSBalance(ctx)
	var response struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	if response.Data == nil {
		response.Data = map[string]any{"code": response.Code}
	}
	return recorder.Code, response.Data
}

func storedQuota(t *testing.T, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	return user.Quota
}

func TestInternalSaaSBalanceAddSubtractOverride(t *testing.T) {
	user := setupInternalBalanceTest(t, 1000)

	status, data := adjustInternalBalanceForTest(t, user.Id, "add", 500)
	require.Equal(t, http.StatusOK, status)
	assert.EqualValues(t, 1000, data["before_quota"])
	assert.EqualValues(t, 1500, data["quota"])

	status, data = adjustInternalBalanceForTest(t, user.Id, "subtract", 200)
	require.Equal(t, http.StatusOK, status)
	assert.EqualValues(t, 1300, data["quota"])

	status, data = adjustInternalBalanceForTest(t, user.Id, "override", 42)
	require.Equal(t, http.StatusOK, status)
	assert.EqualValues(t, 1300, data["before_quota"])
	assert.EqualValues(t, 42, data["quota"])
	assert.Equal(t, 42, storedQuota(t, user.Id))

	var topups int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeTopup).Count(&topups).Error)
	assert.EqualValues(t, 3, topups, "each adjustment is visible in the NewAPI top-up log")
}

func TestInternalSaaSBalanceAllowsNegativeBalances(t *testing.T) {
	user := setupInternalBalanceTest(t, 100)

	status, data := adjustInternalBalanceForTest(t, user.Id, "subtract", 250)
	require.Equal(t, http.StatusOK, status)
	assert.EqualValues(t, -150, data["quota"], "NewAPI allows wallets to go below zero")

	status, data = adjustInternalBalanceForTest(t, user.Id, "override", -500)
	require.Equal(t, http.StatusOK, status)
	assert.EqualValues(t, -500, data["quota"])

	status, data = adjustInternalBalanceForTest(t, user.Id, "add", 600)
	require.Equal(t, http.StatusOK, status)
	assert.EqualValues(t, 100, data["quota"])
	assert.Equal(t, 100, storedQuota(t, user.Id))
}

func TestInternalSaaSBalanceRejectsInvalidInput(t *testing.T) {
	user := setupInternalBalanceTest(t, 100)

	for _, request := range []struct {
		mode  string
		value int
	}{{"add", 0}, {"add", -1}, {"subtract", 0}, {"subtract", -5}, {"set", 10}, {"", 10}} {
		status, _ := adjustInternalBalanceForTest(t, user.Id, request.mode, request.value)
		assert.Equal(t, http.StatusBadRequest, status, request.mode)
	}
	assert.Equal(t, 100, storedQuota(t, user.Id), "rejected requests must not change the balance")
}

func TestInternalSaaSBalanceUnknownUser(t *testing.T) {
	setupInternalBalanceTest(t, 0)

	status, _ := adjustInternalBalanceForTest(t, 99999, "add", 10)
	assert.Equal(t, http.StatusNotFound, status)
}
