package provider

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/davidroman0O/cagent/internal/agent"
)

func TestBuildCodexExecArgsInitial(t *testing.T) {
	req := agent.AgentRequest{
		Model:                      "gpt-5.1-codex",
		ReasoningEffort:            "high",
		MaxOutputTokens:            4096,
		ModelContextWindow:         1000000,
		ModelAutoCompactTokenLimit: 900000,
		CWD:                        "/tmp/work",
		Input:                      []agent.ContentPart{{Type: "text", Text: "hello"}},
		Images:                     []string{"/tmp/a.png"},
		Sandbox:                    agent.SandboxConfig{Mode: "workspace-write"},
		ApprovalPolicy:             "on-request",
		SkipGitRepoCheck:           true,
	}

	args, prompt := BuildCodexExecArgs(req)
	got := strings.Join(args, " ")
	for _, want := range []string{
		"exec",
		"--sandbox workspace-write",
		"-c approval_policy=\"on-request\"",
		"-c model_reasoning_effort=\"high\"",
		"-c model_context_window=1000000",
		"-c model_auto_compact_token_limit=900000",
		"--json",
		"--model gpt-5.1-codex",
		"--cd /tmp/work",
		"--skip-git-repo-check",
		"--image /tmp/a.png",
		"-",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("args missing %q in %q", want, got)
		}
	}
	if prompt != "hello" {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestBuildCodexExecArgsUsesProviderContextDefaults(t *testing.T) {
	p := CodexProvider{
		Bin:                        "codex",
		ModelContextWindow:         1000000,
		ModelAutoCompactTokenLimit: 900000,
	}
	args, _ := p.BuildExecArgs(agent.AgentRequest{
		Input: []agent.ContentPart{{Type: "text", Text: "hello"}},
	})
	got := strings.Join(args, " ")
	for _, want := range []string{
		"-c model_context_window=1000000",
		"-c model_auto_compact_token_limit=900000",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("args missing %q in %q", want, got)
		}
	}
}

func TestBuildCodexExecArgsRequestOverridesProviderContextDefaults(t *testing.T) {
	args, _ := BuildCodexExecArgsWithConfig(agent.AgentRequest{
		ModelContextWindow:         400000,
		ModelAutoCompactTokenLimit: 350000,
		Input:                      []agent.ContentPart{{Type: "text", Text: "hello"}},
	}, CodexExecConfig{
		ModelContextWindow:         1000000,
		ModelAutoCompactTokenLimit: 900000,
	})
	got := strings.Join(args, " ")
	for _, want := range []string{
		"-c model_context_window=400000",
		"-c model_auto_compact_token_limit=350000",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("args missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "1000000") || strings.Contains(got, "900000") {
		t.Fatalf("args kept provider defaults despite request override: %q", got)
	}
}

func TestBuildCodexExecArgsResume(t *testing.T) {
	args, _ := BuildCodexExecArgs(agent.AgentRequest{
		ProviderSessionID: "abc123",
		Input:             []agent.ContentPart{{Type: "text", Text: "continue"}},
		SkipGitRepoCheck:  true,
	})
	got := strings.Join(args, " ")
	if !strings.HasPrefix(got, "exec resume ") {
		t.Fatalf("resume args = %q", got)
	}
	if !strings.Contains(got, "abc123 -") {
		t.Fatalf("resume session id/prompt marker missing: %q", got)
	}
}

func TestParseCodexEventThreadStarted(t *testing.T) {
	ev := ParseCodexEvent(`{"type":"thread.started","session_id":"sess_123"}`)
	if ev.Type != agent.EventStarted {
		t.Fatalf("type = %s", ev.Type)
	}
	if ev.ProviderSessionID != "sess_123" {
		t.Fatalf("provider session = %q", ev.ProviderSessionID)
	}
}

func TestParseCodexEventAgentMessageAndUsage(t *testing.T) {
	msg := ParseCodexEvent(`{"type":"item.completed","item":{"type":"agent_message","text":"final text"}}`)
	if msg.Type != agent.EventMessage || msg.Message != "final text" {
		t.Fatalf("message event = %#v", msg)
	}

	done := ParseCodexEvent(`{"type":"turn.completed","output":"final text","usage":{"input_tokens":10,"output_tokens":4}}`)
	if done.Type != agent.EventDone || done.Final != "final text" {
		t.Fatalf("done event = %#v", done)
	}
	if done.Usage == nil || done.Usage.TotalTokens != 14 {
		t.Fatalf("usage = %#v", done.Usage)
	}
}

func TestLocalCodexCLISmoke(t *testing.T) {
	bin, err := ResolveCodexBinary("")
	if err != nil {
		t.Skipf("codex CLI not installed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := NewCodexSmokeCommand(ctx, bin).CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("codex --version failed: %v\n%s", exitErr, out)
		}
		t.Fatalf("codex --version failed: %v", err)
	}
	if !strings.Contains(string(out), "codex") {
		t.Fatalf("unexpected codex --version output: %q", out)
	}
}

func TestCodexExecIntegration(t *testing.T) {
	if os.Getenv("CAGENT_CODEX_EXEC_TEST") != "1" {
		t.Skip("set CAGENT_CODEX_EXEC_TEST=1 to run a real codex exec turn")
	}
	p, err := NewCodexProviderWithOptions(CodexOptions{
		ModelContextWindow:         1000000,
		ModelAutoCompactTokenLimit: 900000,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	events, err := p.Run(ctx, agent.AgentRequest{
		Input:            []agent.ContentPart{{Type: "text", Text: "Reply with exactly: cagent-ok"}},
		Sandbox:          agent.SandboxConfig{Mode: "read-only"},
		ApprovalPolicy:   "never",
		SkipGitRepoCheck: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	final := ""
	for event := range events {
		if event.Type == agent.EventError {
			t.Fatalf("codex exec error: %s", event.Err)
		}
		if event.Type == agent.EventDone && event.Final != "" {
			final = event.Final
		}
		if event.Type == agent.EventMessage && event.Message != "" {
			final = event.Message
		}
	}
	if !strings.Contains(strings.ToLower(final), "cagent-ok") {
		t.Fatalf("unexpected final output: %q", final)
	}
}
