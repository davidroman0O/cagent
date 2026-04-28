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

func TestResponsesToAgentIncludesClientToolBridge(t *testing.T) {
	req := ResponsesRequest{
		Model: "codex-test",
		Input: raw(`"resume the mission"`),
		Tools: raw(`[{
			"type":"function",
			"name":"ProposeMission",
			"description":"Propose a mission",
			"parameters":{"type":"object"}
		},{
			"type":"function",
			"name":"StartMissionRun",
			"description":"Start the mission runner",
			"parameters":{"type":"object","properties":{"resumeWorkerSessionId":{"type":"string"}}}
		}]`),
	}
	got := ResponsesToAgent(req, AgentDefaults{})
	prompt := got.Input[0].Text
	for _, want := range []string{"[CLIENT TOOLS]", "start_mission_run", "cagent_tool_call", "resumeWorkerSessionId", "features.json"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestResponsesToAgentIncludesDroidWorkerToolGuidance(t *testing.T) {
	req := ResponsesRequest{
		Model: "codex-test",
		Input: raw(`"finish the feature"`),
		Tools: raw(`[{
			"type":"function",
			"name":"EndFeatureRun",
			"description":"End the feature run",
			"parameters":{"type":"object"}
		}]`),
	}
	got := ResponsesToAgent(req, AgentDefaults{})
	prompt := got.Input[0].Text
	for _, want := range []string{"Droid worker workflow", "EndFeatureRun", "normal assistant summary"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestParseResponsesToolCall(t *testing.T) {
	text := "```json\n{\"cagent_tool_call\":{\"name\":\"start_mission_run\",\"arguments\":{\"resumeWorkerSessionId\":\"abc\"}}}\n```"
	call, ok := ParseResponsesToolCall(text, []string{"StartMissionRun"})
	if !ok {
		t.Fatalf("tool call not parsed")
	}
	if call.Name != "StartMissionRun" {
		t.Fatalf("name = %q", call.Name)
	}
	if call.ArgumentsString() != `{"resumeWorkerSessionId":"abc"}` {
		t.Fatalf("arguments = %s", call.ArgumentsString())
	}
}

func TestParseResponsesToolCallDroidMissionAliases(t *testing.T) {
	text := `{"cagent_tool_call":{"name":"dismiss_handoff_items","arguments":{"dismissals":[{"type":"incomplete_work","sourceFeatureId":"F1","summary":"defer","justification":"out of scope"}]}}}`
	call, ok := ParseResponsesToolCall(text, []string{"DismissHandoffItems"})
	if !ok {
		t.Fatalf("tool call not parsed")
	}
	if call.Name != "DismissHandoffItems" {
		t.Fatalf("name = %q", call.Name)
	}
	if !strings.Contains(call.ArgumentsString(), `"dismissals"`) {
		t.Fatalf("arguments = %s", call.ArgumentsString())
	}
}

func TestParseResponsesToolCallFunctionSyntax(t *testing.T) {
	text := `StartMissionRun({"resumeWorkerSessionId":"abc","restartFeature":true})`
	call, ok := ParseResponsesToolCall(text, []string{"StartMissionRun"})
	if !ok {
		t.Fatalf("tool call not parsed")
	}
	if call.Name != "StartMissionRun" {
		t.Fatalf("name = %q", call.Name)
	}
	if call.ArgumentsString() != `{"resumeWorkerSessionId":"abc","restartFeature":true}` {
		t.Fatalf("arguments = %s", call.ArgumentsString())
	}
}

func TestParseResponsesToolCallLabelSyntax(t *testing.T) {
	text := "Tool: EndFeatureRun\nArguments: {\"successState\":\"failure\",\"returnToOrchestrator\":true,\"handoff\":{\"salientSummary\":\"blocked\",\"whatWasImplemented\":\"nothing completed\",\"whatWasLeftUndone\":\"blocked\",\"verification\":{\"commandsRun\":[]},\"tests\":{\"added\":[],\"coverage\":\"none\"},\"discoveredIssues\":[]}}"
	call, ok := ParseResponsesToolCall(text, []string{"EndFeatureRun"})
	if !ok {
		t.Fatalf("tool call not parsed")
	}
	if call.Name != "EndFeatureRun" {
		t.Fatalf("name = %q", call.Name)
	}
	if !strings.Contains(call.ArgumentsString(), `"returnToOrchestrator":true`) {
		t.Fatalf("arguments = %s", call.ArgumentsString())
	}
}

func TestParseResponsesToolCallLabelSyntaxMultiline(t *testing.T) {
	text := "Tool: `ProposeMission`\nArguments:\n{\n  \"title\": \"Build cagent\",\n  \"proposal\": \"Implement Droid-compatible mission support.\"\n}"
	call, ok := ParseResponsesToolCall(text, []string{"ProposeMission"})
	if !ok {
		t.Fatalf("tool call not parsed")
	}
	if call.Name != "ProposeMission" {
		t.Fatalf("name = %q", call.Name)
	}
	if !strings.Contains(call.ArgumentsString(), `"title": "Build cagent"`) {
		t.Fatalf("arguments = %s", call.ArgumentsString())
	}
}

func TestAutoResponsesToolCallResumeMission(t *testing.T) {
	req := ResponsesRequest{
		Instructions: `When you receive the next user message, call start_mission_run with resumeWorkerSessionId="cbeed0a6-e5b1-4ea0-b3cb-e7446c69bb8c".`,
		Input:        raw(`"resume the mission"`),
	}
	call, ok := AutoResponsesToolCall(req, []string{"StartMissionRun"})
	if !ok {
		t.Fatalf("auto tool call not detected")
	}
	if call.Name != "StartMissionRun" {
		t.Fatalf("name = %q", call.Name)
	}
	if !strings.Contains(call.ArgumentsString(), "cbeed0a6-e5b1-4ea0-b3cb-e7446c69bb8c") {
		t.Fatalf("arguments = %s", call.ArgumentsString())
	}
}

func TestAutoResponsesToolCallDoesNotStartNewMissionEarly(t *testing.T) {
	req := ResponsesRequest{
		Input: raw(`"Run a tiny smoke-test mission. After the mission is initialized, call StartMissionRun."`),
	}
	if call, ok := AutoResponsesToolCall(req, []string{"ProposeMission", "StartMissionRun"}); ok {
		t.Fatalf("unexpected auto tool call: %#v", call)
	}
}

func TestAutoResponsesToolCallDoesNotLoopOnToolErrorTranscript(t *testing.T) {
	req := ResponsesRequest{
		Input: raw(`[
			{"type":"function_call_output","call_id":"call_1","output":"Start Mission Run resumeWorkerSessionId: \"9b1168ad-7d06-45f6-b8f8-7e8f234d46b0\"\nError: features.json must exist with at least one feature before starting the run."}
		]`),
	}
	if call, ok := AutoResponsesToolCall(req, []string{"StartMissionRun"}); ok {
		t.Fatalf("unexpected auto tool call: %#v", call)
	}
}

func TestAutoResponsesToolCallExplicitToolChoice(t *testing.T) {
	req := ResponsesRequest{
		Input:      raw(`"start now"`),
		ToolChoice: raw(`{"type":"function","name":"StartMissionRun"}`),
	}
	call, ok := AutoResponsesToolCall(req, []string{"StartMissionRun"})
	if !ok {
		t.Fatalf("auto tool call not detected")
	}
	if call.Name != "StartMissionRun" {
		t.Fatalf("name = %q", call.Name)
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
