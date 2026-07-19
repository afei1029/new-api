package middleware

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

func TestTokenAuthenticationError(t *testing.T) {
	tests := []struct {
		name    string
		token   *model.Token
		message string
		code    types.ErrorCode
	}{
		{
			name:    "missing or unknown key",
			message: "Invalid API key",
			code:    types.ErrorCodeInvalidAPIKey,
		},
		{
			name:    "disabled key",
			token:   &model.Token{Status: common.TokenStatusDisabled},
			message: "API key is disabled",
			code:    types.ErrorCodeAPIKeyDisabled,
		},
		{
			name:    "expired key status",
			token:   &model.Token{Status: common.TokenStatusExpired},
			message: "API key has expired",
			code:    types.ErrorCodeAPIKeyExpired,
		},
		{
			name: "expired key timestamp",
			token: &model.Token{
				Status:      common.TokenStatusEnabled,
				ExpiredTime: time.Now().Add(-time.Minute).Unix(),
			},
			message: "API key has expired",
			code:    types.ErrorCodeAPIKeyExpired,
		},
		{
			name:    "exhausted key status",
			token:   &model.Token{Status: common.TokenStatusExhausted},
			message: "API key quota has been exhausted",
			code:    types.ErrorCodeInsufficientTokenQuota,
		},
		{
			name: "exhausted limited key",
			token: &model.Token{
				Status:         common.TokenStatusEnabled,
				ExpiredTime:    -1,
				UnlimitedQuota: false,
				RemainQuota:    0,
			},
			message: "API key quota has been exhausted",
			code:    types.ErrorCodeInsufficientTokenQuota,
		},
		{
			name: "other invalid key state",
			token: &model.Token{
				Status:         99,
				ExpiredTime:    -1,
				UnlimitedQuota: true,
			},
			message: "Invalid API key",
			code:    types.ErrorCodeInvalidAPIKey,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, code := tokenAuthenticationError(test.token)
			if message != test.message {
				t.Fatalf("message = %q, want %q", message, test.message)
			}
			if code != test.code {
				t.Fatalf("code = %q, want %q", code, test.code)
			}
		})
	}
}
