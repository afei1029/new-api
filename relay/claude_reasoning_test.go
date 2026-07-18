package relay

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestCaptureClaudeReasoningEffort(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	request := &dto.ClaudeRequest{OutputConfig: json.RawMessage(`{"effort":"medium"}`)}

	captureClaudeReasoningEffort(info, request)

	if info.ReasoningEffort != "medium" {
		t.Fatalf("got %q, want medium", info.ReasoningEffort)
	}
}

func TestCaptureClaudeReasoningEffortDoesNotOverwriteWithEmptyValue(t *testing.T) {
	info := &relaycommon.RelayInfo{ReasoningEffort: "high"}

	captureClaudeReasoningEffort(info, &dto.ClaudeRequest{})

	if info.ReasoningEffort != "high" {
		t.Fatalf("got %q, want high", info.ReasoningEffort)
	}
}
