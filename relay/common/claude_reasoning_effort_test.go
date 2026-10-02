package common

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// SaaS customization (2026-07-18): usage logs must record the reasoning
// effort of native Claude requests. Upstream now derives it for every
// protocol in reasoningEffortFromRequest, replacing the former local
// captureClaudeReasoningEffort patch; this test guards that behavior.
func TestReasoningEffortFromClaudeOutputConfig(t *testing.T) {
	request := &dto.ClaudeRequest{OutputConfig: json.RawMessage(`{"effort":"medium"}`)}
	if got := reasoningEffortFromRequest(request); got != "medium" {
		t.Fatalf("got %q, want medium", got)
	}
}

func TestReasoningEffortFromClaudeWithoutEffortIsEmpty(t *testing.T) {
	if got := reasoningEffortFromRequest(&dto.ClaudeRequest{}); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
