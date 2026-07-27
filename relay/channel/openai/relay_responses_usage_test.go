package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIResponsesUsagePreservesBillingTotal(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     1000,
		CompletionTokens: 100,
		TotalTokens:      1100,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 800,
		},
	}

	normalized := normalizeOpenAIResponsesUsage(usage)

	require.Same(t, usage, normalized)
	require.Equal(t, 1000, normalized.InputTokens)
	require.Equal(t, 100, normalized.OutputTokens)
	require.Equal(t, dto.BillingUsageSourceOAIResponses, normalized.UsageSource)
	require.Equal(t, dto.BillingUsageSemanticOpenAI, normalized.UsageSemantic)
	require.NotNil(t, normalized.BillingUsage)
	require.Equal(t, dto.BillingUsageSourceOAIResponses, normalized.BillingUsage.Source)
	require.Equal(t, 1000, normalized.BillingUsage.OpenAIUsage.PromptTokens)
	require.Equal(t, 800, normalized.BillingUsage.OpenAIUsage.PromptTokensDetails.CachedTokens)
}

func TestNormalizeOpenAIChatUsagePreservesBillingTotal(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     1000,
		CompletionTokens: 100,
		TotalTokens:      1100,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 800,
		},
	}

	normalized := normalizeOpenAIChatUsage(usage)

	require.Same(t, usage, normalized)
	require.Equal(t, 1000, normalized.InputTokens)
	require.Equal(t, 100, normalized.OutputTokens)
	require.Equal(t, dto.BillingUsageSourceOAIChat, normalized.UsageSource)
	require.Equal(t, dto.BillingUsageSemanticOpenAI, normalized.UsageSemantic)
	require.NotNil(t, normalized.BillingUsage)
	require.Equal(t, dto.BillingUsageSourceOAIChat, normalized.BillingUsage.Source)
	require.Equal(t, 1000, normalized.BillingUsage.OpenAIUsage.PromptTokens)
	require.Equal(t, 800, normalized.BillingUsage.OpenAIUsage.PromptTokensDetails.CachedTokens)
}
