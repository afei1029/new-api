package relay

import (
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func captureOpenAIChatReasoningEffort(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) {
	if info == nil || request == nil {
		return
	}
	if effort := strings.TrimSpace(request.ReasoningEffort); effort != "" {
		info.ReasoningEffort = effort
	}
}

func captureOpenAIResponsesReasoningEffort(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) {
	if info == nil || request == nil || request.Reasoning == nil {
		return
	}
	if effort := strings.TrimSpace(request.Reasoning.Effort); effort != "" {
		info.ReasoningEffort = effort
	}
}
