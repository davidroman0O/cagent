package config

import "testing"

func TestLoadReadsCodexContextOverrides(t *testing.T) {
	t.Setenv("CAGENT_CODEX_MODEL_CONTEXT_WINDOW", "1000000")
	t.Setenv("CAGENT_CODEX_MODEL_AUTO_COMPACT_TOKEN_LIMIT", "900000")
	t.Setenv("CAGENT_DEFAULT_REASONING_EFFORT", "xhigh")

	got := Load()
	if got.CodexModelContextWindow != 1000000 {
		t.Fatalf("context window = %d", got.CodexModelContextWindow)
	}
	if got.CodexModelAutoCompactTokenLimit != 900000 {
		t.Fatalf("auto compact token limit = %d", got.CodexModelAutoCompactTokenLimit)
	}
	if got.DefaultReasoningEffort != "xhigh" {
		t.Fatalf("default reasoning effort = %q", got.DefaultReasoningEffort)
	}
}
