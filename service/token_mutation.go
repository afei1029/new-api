package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

// Token mutation rules are shared by the dashboard token API and the private
// SaaS integration API so both entry points keep the same business behavior.

const (
	TokenRuleInvalid  = "invalid"
	TokenRuleLimit    = "limit"
	TokenRuleInternal = "internal"
)

// TokenRuleError describes why a token mutation was rejected. Callers decide
// how to render it: dashboard handlers keep their original response shape,
// while the internal API maps Kind to an HTTP status.
type TokenRuleError struct {
	Kind    string
	Key     string
	Args    map[string]any
	Message string
	Err     error
}

func (e *TokenRuleError) Error() string {
	switch {
	case e.Message != "":
		return e.Message
	case e.Err != nil:
		return e.Err.Error()
	default:
		return e.Key
	}
}

func (e *TokenRuleError) Unwrap() error { return e.Err }

// TokenMutationInput carries the user-editable token fields.
type TokenMutationInput struct {
	Name               string
	ExpiredTime        int64
	RemainQuota        int
	UnlimitedQuota     bool
	ModelLimitsEnabled bool
	ModelLimits        string
	AllowIps           *string
	Group              string
	CrossGroupRetry    bool
	// AutoGroupsSet distinguishes an omitted auto_groups field from an
	// explicit null/empty value during updates.
	AutoGroupsSet bool
	AutoGroups    []string
	// AllowNegativeQuota is set only by the SaaS service API. The dashboard
	// API keeps NewAPI's original non-negative rule.
	AllowNegativeQuota bool
}

// TokenUserGroupFunc lazily resolves the owner's user group. It is only
// called when explicit auto groups need validation.
type TokenUserGroupFunc func() (string, error)

func MaxTokenQuota() int {
	quota, err := common.WalletQuotaFromDecimalStrict(
		decimal.NewFromInt(1_000_000_000).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
	if err != nil {
		return common.MaxWalletQuota
	}
	return quota
}

func validateTokenFields(input TokenMutationInput) error {
	if len(input.Name) > 50 {
		return &TokenRuleError{Kind: TokenRuleInvalid, Key: i18n.MsgTokenNameTooLong}
	}
	if !input.UnlimitedQuota {
		if input.RemainQuota < 0 && !input.AllowNegativeQuota {
			return &TokenRuleError{Kind: TokenRuleInvalid, Key: i18n.MsgTokenQuotaNegative}
		}
		maxQuotaValue := MaxTokenQuota()
		if input.RemainQuota > maxQuotaValue || input.RemainQuota < -maxQuotaValue {
			return &TokenRuleError{Kind: TokenRuleInvalid, Key: i18n.MsgTokenQuotaExceedMax, Args: map[string]any{"Max": maxQuotaValue}}
		}
	}
	return nil
}

// ApplyTokenAutoGroups validates and stores an explicit auto group list.
func ApplyTokenAutoGroups(token *model.Token, groups []string, userGroup TokenUserGroupFunc) error {
	if len(groups) == 0 {
		if err := token.SetAutoGroups(nil); err != nil {
			return &TokenRuleError{Kind: TokenRuleInternal, Err: err}
		}
		return nil
	}

	maxCount := setting.GetMaxTokenAutoGroups()
	if len(groups) > maxCount {
		return &TokenRuleError{Kind: TokenRuleInvalid, Key: i18n.MsgTokenAutoGroupsTooMany, Args: map[string]any{"Max": maxCount}}
	}

	ownerGroup, err := userGroup()
	if err != nil {
		return &TokenRuleError{Kind: TokenRuleInternal, Err: err}
	}
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if _, ok := seen[group]; ok {
			return &TokenRuleError{Kind: TokenRuleInvalid, Key: i18n.MsgTokenAutoGroupsDuplicate, Args: map[string]any{"Group": group}}
		}
		seen[group] = struct{}{}
		if !IsUserSelectableGroup(ownerGroup, group) {
			return &TokenRuleError{Kind: TokenRuleInvalid, Key: i18n.MsgTokenAutoGroupsInvalid, Args: map[string]any{"Group": group}}
		}
	}

	if err := token.SetAutoGroups(groups); err != nil {
		return &TokenRuleError{Kind: TokenRuleInternal, Err: err}
	}
	return nil
}

// PrepareNewToken validates a create request and returns an unsaved token with
// a freshly generated key. The caller persists it with token.Insert().
func PrepareNewToken(userID int, input TokenMutationInput, userGroup TokenUserGroupFunc) (*model.Token, error) {
	if err := validateTokenFields(input); err != nil {
		return nil, err
	}

	maxTokens := operation_setting.GetMaxUserTokens()
	count, err := model.CountUserTokens(userID)
	if err != nil {
		return nil, &TokenRuleError{Kind: TokenRuleInternal, Err: err}
	}
	if int(count) >= maxTokens {
		return nil, &TokenRuleError{Kind: TokenRuleLimit, Message: fmt.Sprintf("已达到最大令牌数量限制 (%d)", maxTokens)}
	}

	token := &model.Token{
		UserId:             userID,
		Name:               input.Name,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        input.ExpiredTime,
		RemainQuota:        input.RemainQuota,
		UnlimitedQuota:     input.UnlimitedQuota,
		ModelLimitsEnabled: input.ModelLimitsEnabled,
		ModelLimits:        input.ModelLimits,
		AllowIps:           input.AllowIps,
		Group:              input.Group,
		CrossGroupRetry:    input.CrossGroupRetry,
	}
	if input.Group == "auto" {
		if err := ApplyTokenAutoGroups(token, input.AutoGroups, userGroup); err != nil {
			return nil, err
		}
	} else {
		token.CrossGroupRetry = false
		_ = token.SetAutoGroups(nil)
	}

	key, err := common.GenerateKey()
	if err != nil {
		common.SysLog("failed to generate token key: " + err.Error())
		return nil, &TokenRuleError{Kind: TokenRuleInternal, Key: i18n.MsgTokenGenerateFailed, Err: err}
	}
	token.Key = key
	return token, nil
}

// ValidateTokenFieldsUpdate runs the input checks that do not depend on the
// stored token. It runs before loading the token to keep the original order.
func ValidateTokenFieldsUpdate(input TokenMutationInput) error {
	return validateTokenFields(input)
}

// ValidateTokenStatusChange rejects enabling a token that is already expired
// or exhausted.
func ValidateTokenStatusChange(token *model.Token, status int) error {
	if status != common.TokenStatusEnabled {
		return nil
	}
	if token.Status == common.TokenStatusExpired && token.ExpiredTime <= common.GetTimestamp() && token.ExpiredTime != -1 {
		return &TokenRuleError{Kind: TokenRuleInvalid, Key: i18n.MsgTokenExpiredCannotEnable}
	}
	if token.Status == common.TokenStatusExhausted && token.RemainQuota <= 0 && !token.UnlimitedQuota {
		return &TokenRuleError{Kind: TokenRuleInvalid, Key: i18n.MsgTokenExhaustedCannotEable}
	}
	return nil
}

// ApplyTokenUpdate copies editable fields onto a stored token. Validation of
// the plain fields must already have passed through ValidateTokenFieldsUpdate.
func ApplyTokenUpdate(token *model.Token, input TokenMutationInput, userGroup TokenUserGroupFunc) error {
	// If you add more fields, please also update token.Update()
	token.Name = input.Name
	token.ExpiredTime = input.ExpiredTime
	token.RemainQuota = input.RemainQuota
	token.UnlimitedQuota = input.UnlimitedQuota
	token.ModelLimitsEnabled = input.ModelLimitsEnabled
	token.ModelLimits = input.ModelLimits
	token.AllowIps = input.AllowIps
	token.Group = input.Group
	token.CrossGroupRetry = input.CrossGroupRetry
	if input.Group != "auto" {
		token.CrossGroupRetry = false
		_ = token.SetAutoGroups(nil)
	} else if input.AutoGroupsSet {
		return ApplyTokenAutoGroups(token, input.AutoGroups, userGroup)
	}
	return nil
}
