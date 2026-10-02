package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserUsageQueryFiltersByGatewayOrUpstreamRequestID(t *testing.T) {
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
	LOG_DB.Where("user_id = ?", 901).Delete(&Log{})
	for _, log := range []Log{
		{UserId: 901, Type: LogTypeConsume, TokenId: 7, RequestId: "gw-mine", Quota: 10, CreatedAt: 100},
		{UserId: 901, Type: LogTypeConsume, TokenId: 7, RequestId: "gw-x", UpstreamRequestId: "up-mine", Quota: 20, CreatedAt: 100},
		{UserId: 901, Type: LogTypeConsume, TokenId: 7, RequestId: "gw-other", Quota: 400, CreatedAt: 100},
	} {
		log := log
		require.NoError(t, LOG_DB.Create(&log).Error)
	}

	sum := func(requestIDs []string) int64 {
		tx, err := userUsageQuery(901, 0, 0, "", "", []int{7}, requestIDs)
		require.NoError(t, err)
		var quota int64
		require.NoError(t, tx.Select("COALESCE(SUM(quota), 0)").Scan(&quota).Error)
		return quota
	}

	require.EqualValues(t, 30, sum([]string{"gw-mine", "up-mine"}), "only the member's requests are summed")
	require.EqualValues(t, 430, sum(nil), "no request filter keeps the existing token-scoped behavior")
}
