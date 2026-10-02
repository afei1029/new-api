package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// internalSaaSUserRequest creates the NewAPI user that backs one SaaS
// organization. The user never signs in to NewAPI, so SaaS sends no password
// or email; NewAPI stores an unusable random password instead.
type internalSaaSUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

const internalSaaSUsernameMaxLength = 20

// internalSaaSTokenRequest mirrors the editable fields of the dashboard token
// API. Business rules are applied by service.PrepareNewToken/ApplyTokenUpdate.
type internalSaaSTokenRequest struct {
	Name               string               `json:"name"`
	Group              string               `json:"group"`
	ExpiredTime        int64                `json:"expired_time"`
	RemainQuota        int                  `json:"remain_quota"`
	UnlimitedQuota     bool                 `json:"unlimited_quota"`
	ModelLimitsEnabled bool                 `json:"model_limits_enabled"`
	ModelLimits        string               `json:"model_limits"`
	AllowIPs           string               `json:"allow_ips"`
	CrossGroupRetry    bool                 `json:"cross_group_retry"`
	AutoGroups         tokenAutoGroupsInput `json:"auto_groups"`
}

type internalSaaSTokenUpdateRequest struct {
	Status *int `json:"status"`
	internalSaaSTokenRequest
}

func (request internalSaaSTokenRequest) mutationInput() service.TokenMutationInput {
	var allowIPs *string
	if request.AllowIPs != "" {
		value := request.AllowIPs
		allowIPs = &value
	}
	return service.TokenMutationInput{
		Name:               request.Name,
		ExpiredTime:        request.ExpiredTime,
		RemainQuota:        request.RemainQuota,
		UnlimitedQuota:     request.UnlimitedQuota,
		ModelLimitsEnabled: request.ModelLimitsEnabled,
		ModelLimits:        request.ModelLimits,
		AllowIps:           allowIPs,
		Group:              request.Group,
		CrossGroupRetry:    request.CrossGroupRetry,
		AutoGroupsSet:      request.AutoGroups.Set,
		AutoGroups:         request.AutoGroups.Groups,
		// NewAPI already lets a key's remaining quota drop below zero through
		// usage; the SaaS console must be able to set and keep such values.
		AllowNegativeQuota: true,
	}
}

func internalUserGroupResolver(user *model.User) service.TokenUserGroupFunc {
	return func() (string, error) { return user.Group, nil }
}

// internalTokenRuleError maps shared token rule failures to HTTP statuses so
// the SaaS client can distinguish bad input, quota limits and server faults.
func internalTokenRuleError(c *gin.Context, err error) {
	var ruleErr *service.TokenRuleError
	if !errors.As(err, &ruleErr) {
		internalSaaSError(c, http.StatusInternalServerError, "TOKEN_OPERATION_FAILED", err)
		return
	}
	message := ruleErr.Message
	if message == "" && ruleErr.Key != "" {
		message = common.TranslateMessage(c, ruleErr.Key, ruleErr.Args)
	}
	if message == "" {
		message = ruleErr.Error()
	}
	switch ruleErr.Kind {
	case service.TokenRuleInvalid:
		internalSaaSError(c, http.StatusBadRequest, "TOKEN_RULE_REJECTED", errors.New(message))
	case service.TokenRuleLimit:
		internalSaaSError(c, http.StatusConflict, "TOKEN_LIMIT_REACHED", errors.New(message))
	default:
		internalSaaSError(c, http.StatusInternalServerError, "TOKEN_OPERATION_FAILED", errors.New(message))
	}
}

func internalTokenView(token *model.Token) gin.H {
	autoGroups, err := token.GetAutoGroups()
	if err != nil {
		common.SysError(fmt.Sprintf("failed to parse auto groups for token %d: %v", token.Id, err))
		autoGroups = nil
	}
	if autoGroups == nil {
		autoGroups = []string{}
	}
	return gin.H{
		"id": token.Id, "status": token.Status, "name": token.Name,
		"created_time": token.CreatedTime, "accessed_time": token.AccessedTime,
		"group":        token.Group,
		"expired_time": token.ExpiredTime, "remain_quota": token.RemainQuota,
		"unlimited_quota":      token.UnlimitedQuota,
		"used_quota":           token.UsedQuota,
		"model_limits_enabled": token.ModelLimitsEnabled,
		"model_limits":         token.ModelLimits,
		"allow_ips":            token.GetIpLimits(),
		"cross_group_retry":    token.CrossGroupRetry,
		"auto_groups":          autoGroups,
		"key":                  token.GetMaskedKey(),
	}
}

func ListInternalSaaSGroups(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	// Reuse the same group visibility rules as the user's groups endpoint,
	// while the service credential bypasses the user's dashboard session.
	userGroup, _ := model.GetUserGroup(user.Id, false)
	usableGroups := service.GetUserUsableGroups(userGroup)
	groups := make([]gin.H, 0, len(usableGroups))
	for groupName, desc := range usableGroups {
		groups = append(groups, gin.H{"value": groupName, "label": desc})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": groups})
}

func ListInternalSaaSTokens(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	page := 1
	pageSize := 100
	if value, err := strconv.Atoi(c.Query("p")); err == nil && value > 0 {
		page = value
	}
	if value, err := strconv.Atoi(c.Query("page_size")); err == nil && value > 0 && value <= 100 {
		pageSize = value
	}
	tokens, err := model.GetAllUserTokens(user.Id, (page-1)*pageSize, pageSize)
	if err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "TOKEN_LIST_FAILED", err)
		return
	}
	total, err := model.CountUserTokens(user.Id)
	if err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "TOKEN_COUNT_FAILED", err)
		return
	}
	items := make([]gin.H, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, internalTokenView(token))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"page": page, "page_size": pageSize, "total": total, "items": items}})
}

func GetInternalSaaSToken(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	tokenID, err := strconv.Atoi(c.Param("token_id"))
	if err != nil || tokenID <= 0 {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_TOKEN_ID", errors.New("invalid token id"))
		return
	}
	token, err := model.GetTokenByIds(tokenID, user.Id)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "TOKEN_NOT_FOUND", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": internalTokenView(token)})
}

func GetInternalSaaSTokenKey(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	tokenID, err := strconv.Atoi(c.Param("token_id"))
	if err != nil || tokenID <= 0 {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_TOKEN_ID", errors.New("invalid token id"))
		return
	}
	token, err := model.GetTokenByIds(tokenID, user.Id)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "TOKEN_NOT_FOUND", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"key": token.GetFullKey()}})
}

type internalSaaSBalanceRequest struct {
	Mode  string `json:"mode"`  // add, subtract, override
	Value int    `json:"value"` // quota units
}

func internalSaaSError(c *gin.Context, status int, code string, err error) {
	message := "internal service operation failed"
	if err != nil {
		message = err.Error()
	}
	c.AbortWithStatusJSON(status, gin.H{"success": false, "code": code, "message": message})
}

// EnsureInternalSaaSUser creates the NewAPI user for one SaaS organization
// without a dashboard login. The username is organization-specific, so it is
// the idempotency key: retries return the existing user instead of creating a
// duplicate. Email is intentionally not used, because one SaaS user can own
// several organizations and each of them needs its own NewAPI user.
func EnsureInternalSaaSUser(c *gin.Context) {
	var request internalSaaSUserRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_REQUEST", err)
		return
	}
	request.Username = strings.TrimSpace(request.Username)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	if request.Username == "" || utf8.RuneCountInString(request.Username) > internalSaaSUsernameMaxLength {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_USERNAME", fmt.Errorf("username must be 1 to %d characters", internalSaaSUsernameMaxLength))
		return
	}
	if request.DisplayName == "" || utf8.RuneCountInString(request.DisplayName) > internalSaaSUsernameMaxLength {
		request.DisplayName = request.Username
	}

	var existing model.User
	if err := model.DB.Where("username = ?", request.Username).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": existing.Id, "created": false}})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		internalSaaSError(c, http.StatusInternalServerError, "USER_LOOKUP_FAILED", err)
		return
	}

	// Nobody signs in as this user; the password only satisfies NewAPI's
	// account model and is discarded after hashing.
	password, err := common.GenerateRandomCharsKey(48)
	if err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "USER_CREATE_FAILED", err)
		return
	}
	user := model.User{
		Username:    request.Username,
		Password:    password,
		DisplayName: request.DisplayName,
		Role:        common.RoleCommonUser,
	}
	if err := user.Insert(0); err != nil {
		internalSaaSError(c, http.StatusConflict, "USER_CREATE_FAILED", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": user.Id, "created": true}})
}

func internalTargetUser(c *gin.Context) (*model.User, error) {
	id, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || id <= 0 {
		return nil, errors.New("invalid user id")
	}
	return model.GetUserById(id, true)
}

func CreateInternalSaaSToken(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	var request internalSaaSTokenRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_REQUEST", err)
		return
	}
	token, err := service.PrepareNewToken(user.Id, request.mutationInput(), internalUserGroupResolver(user))
	if err != nil {
		internalTokenRuleError(c, err)
		return
	}
	if err := token.Insert(); err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "TOKEN_CREATE_FAILED", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": token.Id, "key": token.Key}})
}

func UpdateInternalSaaSToken(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	tokenID, err := strconv.Atoi(c.Param("token_id"))
	if err != nil || tokenID <= 0 {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_TOKEN_ID", errors.New("invalid token id"))
		return
	}
	var request internalSaaSTokenUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_REQUEST", err)
		return
	}
	input := request.mutationInput()
	if request.Status == nil {
		if err := service.ValidateTokenFieldsUpdate(input); err != nil {
			internalTokenRuleError(c, err)
			return
		}
	} else if *request.Status != common.TokenStatusEnabled && *request.Status != common.TokenStatusDisabled {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_TOKEN_STATUS", errors.New("status must be 1 or 2"))
		return
	}
	token, err := model.GetTokenByIds(tokenID, user.Id)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "TOKEN_NOT_FOUND", err)
		return
	}
	if request.Status != nil {
		// Status-only update, equivalent to PUT /api/token/?status_only=true.
		if err := service.ValidateTokenStatusChange(token, *request.Status); err != nil {
			internalTokenRuleError(c, err)
			return
		}
		token.Status = *request.Status
	} else if err := service.ApplyTokenUpdate(token, input, internalUserGroupResolver(user)); err != nil {
		internalTokenRuleError(c, err)
		return
	}
	if err := token.Update(); err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "TOKEN_UPDATE_FAILED", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": internalTokenView(token)})
}

func DeleteInternalSaaSToken(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	tokenID, err := strconv.Atoi(c.Param("token_id"))
	if err != nil || tokenID <= 0 {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_TOKEN_ID", errors.New("invalid token id"))
		return
	}
	token, err := model.GetTokenByIds(tokenID, user.Id)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "TOKEN_NOT_FOUND", err)
		return
	}
	if err := token.Delete(); err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "TOKEN_DELETE_FAILED", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type internalSaaSTokenBatchDeleteRequest struct {
	IDs []int `json:"ids"`
}

const internalSaaSTokenBatchLimit = 100

// DeleteInternalSaaSTokens removes several keys owned by the target user in one
// transaction. IDs that do not belong to the user are ignored by the model
// query, so one organization can never delete another user's keys.
func DeleteInternalSaaSTokens(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	var request internalSaaSTokenBatchDeleteRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_REQUEST", err)
		return
	}
	if len(request.IDs) == 0 || len(request.IDs) > internalSaaSTokenBatchLimit {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_TOKEN_IDS", fmt.Errorf("ids must contain 1 to %d token ids", internalSaaSTokenBatchLimit))
		return
	}
	for _, id := range request.IDs {
		if id <= 0 {
			internalSaaSError(c, http.StatusBadRequest, "INVALID_TOKEN_IDS", errors.New("token ids must be positive"))
			return
		}
	}
	count, err := model.BatchDeleteTokens(request.IDs, user.Id)
	if err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "TOKEN_DELETE_FAILED", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deleted": count}})
}

type internalSaaSUserStatusRequest struct {
	Status string `json:"status"` // active, disabled
}

// SetInternalSaaSUserStatus enables or disables the NewAPI user that backs a
// SaaS organization. It follows the same update path as the admin
// enable/disable action: the auth version is bumped, browser sessions are
// revoked, and token caches are invalidated so a disabled organization's keys
// stop working immediately. Setting the current status again is a no-op.
func SetInternalSaaSUserStatus(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	var request internalSaaSUserStatusRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_REQUEST", err)
		return
	}
	var status int
	switch strings.ToLower(strings.TrimSpace(request.Status)) {
	case "active":
		status = common.UserStatusEnabled
	case "disabled":
		status = common.UserStatusDisabled
	default:
		internalSaaSError(c, http.StatusBadRequest, "INVALID_USER_STATUS", errors.New("status must be active or disabled"))
		return
	}
	// SaaS organization users are always common users; never touch admins.
	if user.Role != common.RoleCommonUser {
		internalSaaSError(c, http.StatusForbidden, "USER_NOT_MANAGED", errors.New("only SaaS organization users can be managed"))
		return
	}
	if user.Status != status {
		user.Status = status
		if err := user.Update(false); err != nil {
			internalSaaSError(c, http.StatusInternalServerError, "USER_STATUS_UPDATE_FAILED", err)
			return
		}
		if err := model.InvalidateUserTokensCache(user.Id); err != nil {
			common.SysLog(fmt.Sprintf("failed to invalidate tokens cache for user %d: %s", user.Id, err.Error()))
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user_id": user.Id, "status": request.Status}})
}

func GetInternalSaaSBalance(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	quota, err := model.GetUserQuota(user.Id, true)
	if err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "BALANCE_READ_FAILED", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user_id": user.Id, "quota": quota, "used_quota": user.UsedQuota, "request_count": user.RequestCount}})
}

// AdjustInternalSaaSBalance changes a user's wallet through NewAPI's own
// manual quota adjustment, the same model function behind the admin
// "add_quota" action. It locks the user row, rejects results outside the
// wallet bounds, syncs the quota cache, and is recorded as a top-up log.
// Like NewAPI itself, it allows the resulting balance to be negative.
func AdjustInternalSaaSBalance(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	var request internalSaaSBalanceRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_BALANCE_REQUEST", err)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(request.Mode))
	if mode != "add" && mode != "subtract" && mode != "override" {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_BALANCE_REQUEST", errors.New("mode must be add, subtract, or override"))
		return
	}
	// add/subtract amounts must be positive. An override may set any value,
	// including a negative balance: NewAPI allows wallets to go below zero.
	if mode != "override" && request.Value <= 0 {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_BALANCE_REQUEST", errors.New("value must be positive for add and subtract"))
		return
	}
	// The SaaS backend acts as the platform root operator.
	adjustment, err := model.AdjustUserQuota(user.Id, common.RoleRootUser, mode, request.Value)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidUserQuotaAdjustment):
			internalSaaSError(c, http.StatusBadRequest, "INVALID_BALANCE_REQUEST", err)
		case errors.Is(err, model.ErrWalletQuotaLimitExceeded):
			internalSaaSError(c, http.StatusBadRequest, "BALANCE_LIMIT_EXCEEDED", err)
		case errors.Is(err, gorm.ErrRecordNotFound):
			internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		default:
			internalSaaSError(c, http.StatusInternalServerError, "BALANCE_UPDATE_FAILED", err)
		}
		return
	}
	model.RecordLog(adjustment.UserID, model.LogTypeTopup, fmt.Sprintf(
		"SaaS 调整余额（%s）：%s → %s", mode, logger.LogQuota(adjustment.Before), logger.LogQuota(adjustment.After),
	))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"user_id": adjustment.UserID, "quota": adjustment.After, "before_quota": adjustment.Before,
	}})
}

// ListInternalSaaSUsage exposes the same consumption records used by the
// user's dashboard, but authenticates through the private SaaS service token.
func ListInternalSaaSUsage(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	pageInfo := common.GetPageQuery(c)
	typeValue := 2
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenIDs, err := parseInternalTokenIds(c.Query("token_ids"))
	if err != nil {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_TOKEN_IDS", err)
		return
	}
	logs, total, err := model.GetUserLogs(user.Id, typeValue, startTimestamp, endTimestamp,
		c.Query("model_name"), c.Query("token_name"), pageInfo.GetStartIdx(), pageInfo.GetPageSize(),
		c.Query("group"), c.Query("request_id"), c.Query("upstream_request_id"), tokenIDs)
	if err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "USAGE_LIST_FAILED", err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
}

func GetInternalSaaSUsageSummary(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenIDs, err := parseInternalTokenIds(c.Query("token_ids"))
	if err != nil {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_TOKEN_IDS", err)
		return
	}
	requestIDs, err := parseInternalRequestIds(c.Query("request_ids"))
	if err != nil {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_REQUEST_IDS", err)
		return
	}
	result, err := model.GetUserUsageSummary(user.Id, startTimestamp, endTimestamp, c.Query("model_name"), c.Query("group"), tokenIDs, requestIDs)
	if err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "USAGE_SUMMARY_FAILED", err)
		return
	}
	models, err := model.GetUserModelUsage(user.Id, startTimestamp, endTimestamp, c.Query("model_name"), c.Query("group"), tokenIDs, requestIDs)
	if err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "USAGE_MODEL_FAILED", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"request_count": result.RequestCount, "prompt_tokens": result.PromptTokens,
		"completion_tokens": result.CompletionTokens, "cache_read": result.CacheRead,
		"cache_write": result.CacheWrite, "quota": result.Quota, "models": models,
	}})
}

func GetInternalSaaSUsageTrend(c *gin.Context) {
	user, err := internalTargetUser(c)
	if err != nil {
		internalSaaSError(c, http.StatusNotFound, "USER_NOT_FOUND", err)
		return
	}
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenIDs, err := parseInternalTokenIds(c.Query("token_ids"))
	if err != nil {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_TOKEN_IDS", err)
		return
	}
	requestIDs, err := parseInternalRequestIds(c.Query("request_ids"))
	if err != nil {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_REQUEST_IDS", err)
		return
	}
	items, err := model.GetUserTokenTrend(user.Id, startTimestamp, endTimestamp, c.Query("model_name"), c.Query("group"), tokenIDs, requestIDs, c.Query("timezone"))
	if err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "USAGE_TREND_FAILED", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// parseInternalRequestIds parses a comma-separated request ID list (max 100).
func parseInternalRequestIds(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	if len(parts) > 100 {
		return nil, errors.New("too many request ids")
	}
	ids := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" || len(id) > 64 {
			return nil, errors.New("invalid request id")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func parseInternalTokenIds(value string) ([]int, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	if len(parts) > 100 {
		return nil, errors.New("too many token ids")
	}
	ids := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			return nil, errors.New("invalid token id")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}
