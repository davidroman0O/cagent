package compat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatToAgentFlattensMessages(t *testing.T) {
	req := ChatCompletionRequest{
		Model:                      "codex-test",
		MaxTokens:                  1234,
		ModelContextWindow:         1000000,
		ModelAutoCompactTokenLimit: 900000,
		Messages: []ChatMessage{
			{Role: "system", Content: raw(`"be terse"`)},
			{Role: "user", Content: raw(`"hello"`)},
			{Role: "assistant", Content: raw(`"hi"`), ToolCalls: raw(`[{"id":"call_1","type":"function"}]`)},
			{Role: "tool", ToolCallID: "call_1", Content: raw(`"42"`)},
		},
	}
	got := ChatToAgent(req, AgentDefaults{})
	if got.Model != "codex-test" {
		t.Fatalf("model = %q", got.Model)
	}
	if got.MaxOutputTokens != 1234 {
		t.Fatalf("max output tokens = %d", got.MaxOutputTokens)
	}
	if got.ModelContextWindow != 1000000 {
		t.Fatalf("model context window = %d", got.ModelContextWindow)
	}
	if got.ModelAutoCompactTokenLimit != 900000 {
		t.Fatalf("model auto compact token limit = %d", got.ModelAutoCompactTokenLimit)
	}
	prompt := got.Input[0].Text
	for _, want := range []string{"[SYSTEM]\nbe terse", "[USER]\nhello", "[ASSISTANT]\nhi", "Tool calls:", "[TOOL tool_call_id=call_1]\n42"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestResponsesToAgentReadsContextMetadataAliases(t *testing.T) {
	req := ResponsesRequest{
		Model: "codex-test",
		Input: raw(`"hello"`),
		Metadata: map[string]any{
			"context_window":           float64(1000000),
			"auto_compact_token_limit": float64(900000),
		},
	}
	got := ResponsesToAgent(req, AgentDefaults{})
	if got.ModelContextWindow != 1000000 {
		t.Fatalf("model context window = %d", got.ModelContextWindow)
	}
	if got.ModelAutoCompactTokenLimit != 900000 {
		t.Fatalf("model auto compact token limit = %d", got.ModelAutoCompactTokenLimit)
	}
}

func TestResponsesToAgentReadsReasoningObject(t *testing.T) {
	req := ResponsesRequest{
		Model:     "gpt-5.5",
		Input:     raw(`"hello"`),
		Reasoning: ReasoningConfig{Effort: "xhigh"},
	}
	got := ResponsesToAgent(req, AgentDefaults{})
	if got.ReasoningEffort != "xhigh" {
		t.Fatalf("reasoning effort = %q", got.ReasoningEffort)
	}
}

func TestChatToAgentReadsReasoningMetadataAlias(t *testing.T) {
	req := ChatCompletionRequest{
		Model:    "gpt-5.5",
		Messages: []ChatMessage{{Role: "user", Content: raw(`"hello"`)}},
		Metadata: map[string]any{"model_reasoning_effort": "high"},
	}
	got := ChatToAgent(req, AgentDefaults{})
	if got.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q", got.ReasoningEffort)
	}
}

func TestResponsesToAgentReadsReasoningFromDefaultModelSuffix(t *testing.T) {
	req := ResponsesRequest{
		Model: "codex-default",
		Input: raw(`"hello"`),
	}
	got := ResponsesToAgent(req, AgentDefaults{Model: "codex:gpt-5.5:xhigh"})
	if got.Provider != "codex" {
		t.Fatalf("provider = %q", got.Provider)
	}
	if got.Model != "gpt-5.5" {
		t.Fatalf("model = %q", got.Model)
	}
	if got.ReasoningEffort != "xhigh" {
		t.Fatalf("reasoning effort = %q", got.ReasoningEffort)
	}
}

func TestChatToAgentReadsDefaultReasoningEffort(t *testing.T) {
	req := ChatCompletionRequest{
		Model:    "codex-default",
		Messages: []ChatMessage{{Role: "user", Content: raw(`"hello"`)}},
	}
	got := ChatToAgent(req, AgentDefaults{Model: "codex:gpt-5.5", ReasoningEffort: "high"})
	if got.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q", got.ReasoningEffort)
	}
}

func TestChatToAgentModelReasoningOverridesDefaultReasoningEffort(t *testing.T) {
	req := ChatCompletionRequest{
		Model:    "codex:gpt-5.5:high",
		Messages: []ChatMessage{{Role: "user", Content: raw(`"hello"`)}},
	}
	got := ChatToAgent(req, AgentDefaults{ReasoningEffort: "xhigh"})
	if got.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q", got.ReasoningEffort)
	}
}

func TestChatToAgentSessionUsesLatestUserAndSystem(t *testing.T) {
	req := ChatCompletionRequest{
		SessionID: "sess_1",
		Messages: []ChatMessage{
			{Role: "system", Content: raw(`"policy"`)},
			{Role: "user", Content: raw(`"old"`)},
			{Role: "assistant", Content: raw(`"old answer"`)},
			{Role: "user", Content: raw(`"new"`)},
		},
	}
	got := ChatToAgent(req, AgentDefaults{})
	prompt := got.Input[0].Text
	if strings.Contains(prompt, "old") {
		t.Fatalf("session prompt included old transcript:\n%s", prompt)
	}
	if !strings.Contains(prompt, "policy") || !strings.Contains(prompt, "new") {
		t.Fatalf("session prompt missing system/latest user:\n%s", prompt)
	}
}

func TestResponsesToAgent(t *testing.T) {
	req := ResponsesRequest{
		Model:        "codex-test",
		Instructions: "follow rules",
		Input: raw(`[
			{"role":"user","content":[{"type":"input_text","text":"implement this"}]}
		]`),
	}
	got := ResponsesToAgent(req, AgentDefaults{CWD: "/repo"})
	if got.CWD != "/repo" {
		t.Fatalf("cwd = %q", got.CWD)
	}
	if !strings.Contains(got.Input[0].Text, "follow rules") || !strings.Contains(got.Input[0].Text, "implement this") {
		t.Fatalf("prompt = %q", got.Input[0].Text)
	}
}

func TestParseModel(t *testing.T) {
	model, effort, provider := ParseModel("codex:gpt-5-codex:high")
	if provider != "codex" || model != "gpt-5-codex" || effort != "high" {
		t.Fatalf("parsed = provider:%q model:%q effort:%q", provider, model, effort)
	}
	model, effort, provider = ParseModel("codex-default")
	if provider != "" || model != "" || effort != "" {
		t.Fatalf("codex-default should map to local default, got provider:%q model:%q effort:%q", provider, model, effort)
	}
}

func raw(s string) json.RawMessage {
	return json.RawMessage(s)
}
