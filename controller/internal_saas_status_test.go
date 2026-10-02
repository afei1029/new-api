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

func setInternalUserStatusForTest(t *testing.T, userID int, status string) int {
	t.Helper()
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/internal/saas", map[string]any{"status": status}, 0)
	ctx.Params = gin.Params{{Key: "user_id", Value: strconv.Itoa(userID)}}
	SetInternalSaaSUserStatus(ctx)
	return recorder.Code
}

func setupInternalStatusTest(t *testing.T, role int) *model.User {
	t.Helper()
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	user := &model.User{Id: 401, Username: "status-user", Password: "password", Group: "default", Status: common.UserStatusEnabled, Role: role, AffCode: "status-aff"}
	require.NoError(t, db.Create(user).Error)
	return user
}

func storedUserStatus(t *testing.T, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	return user.Status
}

func TestInternalSaaSUserStatusDisablesAndEnables(t *testing.T) {
	user := setupInternalStatusTest(t, common.RoleCommonUser)

	require.Equal(t, http.StatusOK, setInternalUserStatusForTest(t, user.Id, "disabled"))
	assert.Equal(t, common.UserStatusDisabled, storedUserStatus(t, user.Id))

	// Repeating the current status is a successful no-op.
	require.Equal(t, http.StatusOK, setInternalUserStatusForTest(t, user.Id, "disabled"))
	assert.Equal(t, common.UserStatusDisabled, storedUserStatus(t, user.Id))

	require.Equal(t, http.StatusOK, setInternalUserStatusForTest(t, user.Id, "active"))
	assert.Equal(t, common.UserStatusEnabled, storedUserStatus(t, user.Id))
}

func TestInternalSaaSUserStatusRejectsInvalidRequests(t *testing.T) {
	user := setupInternalStatusTest(t, common.RoleCommonUser)

	assert.Equal(t, http.StatusBadRequest, setInternalUserStatusForTest(t, user.Id, "deleted"))
	assert.Equal(t, http.StatusNotFound, setInternalUserStatusForTest(t, 99999, "disabled"))
	assert.Equal(t, common.UserStatusEnabled, storedUserStatus(t, user.Id))
}

func TestInternalSaaSUserStatusNeverTouchesAdministrators(t *testing.T) {
	admin := setupInternalStatusTest(t, common.RoleRootUser)

	assert.Equal(t, http.StatusForbidden, setInternalUserStatusForTest(t, admin.Id, "disabled"))
	assert.Equal(t, common.UserStatusEnabled, storedUserStatus(t, admin.Id))
}
