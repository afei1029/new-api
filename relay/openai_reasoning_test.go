package relay

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func TestCaptureOpenAIChatReasoningEffort(t *testing.T) {
	info := &relaycommon.RelayInfo{}

	captureOpenAIChatReasoningEffort(info, &dto.GeneralOpenAIRequest{ReasoningEffort: "high"})

	if info.ReasoningEffort != "high" {
		t.Fatalf("got %q, want high", info.ReasoningEffort)
	}
}

func TestCaptureOpenAIResponsesReasoningEffort(t *testing.T) {
	info := &relaycommon.RelayInfo{}

	captureOpenAIResponsesReasoningEffort(info, &dto.OpenAIResponsesRequest{
		Reasoning: &dto.Reasoning{Effort: "medium"},
	})

	if info.ReasoningEffort != "medium" {
		t.Fatalf("got %q, want medium", info.ReasoningEffort)
	}
}

func TestCaptureOpenAIReasoningEffortDoesNotOverwriteWithEmptyValue(t *testing.T) {
	tests := []struct {
		name    string
		capture func(*relaycommon.RelayInfo)
	}{
		{
			name: "chat",
			capture: func(info *relaycommon.RelayInfo) {
				captureOpenAIChatReasoningEffort(info, &dto.GeneralOpenAIRequest{})
			},
		},
		{
			name: "responses",
			capture: func(info *relaycommon.RelayInfo) {
				captureOpenAIResponsesReasoningEffort(info, &dto.OpenAIResponsesRequest{})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ReasoningEffort: "high"}
			tt.capture(info)
			if info.ReasoningEffort != "high" {
				t.Fatalf("got %q, want high", info.ReasoningEffort)
			}
		})
	}
}

func TestCaptureOpenAIReasoningEffortHandlesNil(t *testing.T) {
	captureOpenAIChatReasoningEffort(nil, nil)
	captureOpenAIResponsesReasoningEffort(nil, nil)
}
