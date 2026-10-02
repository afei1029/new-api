package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// internalActivityMaxWindowSeconds bounds one activity scan. SaaS reads
// windows of a few minutes, so a week is far above normal use while still
// preventing an accidental full-table scan.
const internalActivityMaxWindowSeconds int64 = 7 * 24 * 60 * 60

// internalActivityLog is the minimal projection SaaS needs to attribute API
// activity. It never carries prompts, content, IPs, or the full log metadata;
// error logs only expose the error classification fields.
type internalActivityLog struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	TokenID   int    `json:"token_id"`
	Type      int    `json:"type"`
	Channel   int    `json:"channel"`
	CreatedAt int64  `json:"created_at"`
	RequestID string `json:"request_id"`
	ErrorType string `json:"error_type,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

type internalActivityLogRow struct {
	Id        int
	UserId    int
	TokenId   int
	Type      int
	ChannelId int
	CreatedAt int64
	RequestId string
	Other     string
}

// ListInternalSaaSActivityLogs returns consume and error logs of all users in
// a closed time window, oldest first, so SaaS can page through a window
// deterministically while building its activity statistics.
func ListInternalSaaSActivityLogs(c *gin.Context) {
	startTimestamp, startErr := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, endErr := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if startErr != nil || endErr != nil || startTimestamp <= 0 || endTimestamp < startTimestamp {
		internalSaaSError(c, http.StatusBadRequest, "INVALID_TIME_WINDOW", errors.New("start_timestamp and end_timestamp are required"))
		return
	}
	if endTimestamp-startTimestamp > internalActivityMaxWindowSeconds {
		internalSaaSError(c, http.StatusBadRequest, "TIME_WINDOW_TOO_LARGE", errors.New("time window must not exceed 7 days"))
		return
	}
	pageInfo := common.GetPageQuery(c)
	query := model.LOG_DB.Model(&model.Log{}).
		Where("logs.type IN ?", []int{model.LogTypeConsume, model.LogTypeError}).
		Where("logs.created_at >= ? AND logs.created_at <= ?", startTimestamp, endTimestamp)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "ACTIVITY_LIST_FAILED", err)
		return
	}
	var rows []internalActivityLogRow
	if err := query.
		Select("logs.id, logs.user_id, logs.token_id, logs.type, logs.channel_id, logs.created_at, logs.request_id, logs.other").
		Order("logs.created_at asc, logs.id asc").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).
		Scan(&rows).Error; err != nil {
		internalSaaSError(c, http.StatusInternalServerError, "ACTIVITY_LIST_FAILED", err)
		return
	}
	items := make([]internalActivityLog, 0, len(rows))
	for _, row := range rows {
		item := internalActivityLog{
			ID: row.Id, UserID: row.UserId, TokenID: row.TokenId, Type: row.Type,
			Channel: row.ChannelId, CreatedAt: row.CreatedAt, RequestID: row.RequestId,
		}
		if row.Type == model.LogTypeError && row.Other != "" {
			var other struct {
				ErrorType string `json:"error_type"`
				ErrorCode string `json:"error_code"`
			}
			if common.UnmarshalJsonStr(row.Other, &other) == nil {
				item.ErrorType, item.ErrorCode = other.ErrorType, other.ErrorCode
			}
		}
		items = append(items, item)
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}
