package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ensureInternalSaaSUserForTest(t *testing.T, body map[string]any) (int, map[string]any) {
	t.Helper()
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/internal/saas/users", body, 0)
	EnsureInternalSaaSUser(ctx)
	var response struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return recorder.Code, response.Data
}

func TestEnsureInternalSaaSUserCreatesUnusableAccountAndIsIdempotent(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	status, first := ensureInternalSaaSUserForTest(t, map[string]any{"username": "team-1234abcd", "display_name": "team-1234abcd"})
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, true, first["created"])

	status, second := ensureInternalSaaSUserForTest(t, map[string]any{"username": "team-1234abcd"})
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, false, second["created"])
	assert.Equal(t, first["id"], second["id"])

	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "team-1234abcd").First(&user).Error)
	assert.Empty(t, user.Email, "organization users are not bound to an owner email")
	assert.NotEmpty(t, user.Password, "NewAPI stores a random password hash")
	assert.Equal(t, common.RoleCommonUser, user.Role)
}

func TestEnsureInternalSaaSUserCreatesSeparateUsersPerOrganization(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	_, personal := ensureInternalSaaSUserForTest(t, map[string]any{"username": "owner-aaaaaaaa"})
	_, team := ensureInternalSaaSUserForTest(t, map[string]any{"username": "team-bbbbbbbb"})

	assert.Equal(t, true, personal["created"])
	assert.Equal(t, true, team["created"])
	assert.NotEqual(t, personal["id"], team["id"])
}

func TestEnsureInternalSaaSUserRejectsInvalidUsername(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	for _, username := range []string{"", "   ", "this-username-is-far-too-long"} {
		status, _ := ensureInternalSaaSUserForTest(t, map[string]any{"username": username})
		assert.Equal(t, http.StatusBadRequest, status, username)
	}
}
