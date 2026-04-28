package bench

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/davidroman0O/cagent/internal/agent"
	"github.com/davidroman0O/cagent/internal/provider"
)

type Mode string

const (
	ModeCodexCLI  Mode = "codex-cli"
	ModeResponses Mode = "responses"
)

type Config struct {
	Mode                       Mode
	BaseURL                    string
	APIToken                   string
	CodexBin                   string
	Model                      string
	ReasoningEffort            string
	ModelContextWindow         int
	ModelAutoCompactTokenLimit int
	CWD                        string
	Targets                    []int
	Timeout                    time.Duration
}

type Result struct {
	Mode               Mode   `json:"mode"`
	Model              string `json:"model,omitempty"`
	RequestedTokens    int    `json:"requested_tokens"`
	MaxOutputTokens    int    `json:"max_output_tokens,omitempty"`
	ActualOutputTokens int    `json:"actual_output_tokens,omitempty"`
	InputTokens        int    `json:"input_tokens,omitempty"`
	TotalTokens        int    `json:"total_tokens,omitempty"`
	Bytes              int    `json:"bytes"`
	DurationMS         int64  `json:"duration_ms"`
	Sentinel           string `json:"sentinel"`
	SentinelSeen       bool   `json:"sentinel_seen"`
	Completed          bool   `json:"completed"`
	Error              string `json:"error,omitempty"`
}

type Report struct {
	StartedAt time.Time `json:"started_at"`
	Results   []Result  `json:"results"`
}

func Run(ctx context.Context, cfg Config) Report {
	report := Report{StartedAt: time.Now().UTC()}
	for _, target := range cfg.Targets {
		result := runOne(ctx, cfg, target)
		report.Results = append(report.Results, result)
	}
	return report
}

func runOne(ctx context.Context, cfg Config, target int) Result {
	sentinel := fmt.Sprintf("CAGENT_SENTINEL_%d", target)
	result := Result{
		Mode:            cfg.Mode,
		Model:           cfg.Model,
		RequestedTokens: target,
		MaxOutputTokens: target,
		Sentinel:        sentinel,
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	turnCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()

	text, usage, err := runMode(turnCtx, cfg, target, Prompt(target, sentinel))
	result.DurationMS = time.Since(start).Milliseconds()
	result.Bytes = len([]byte(text))
	result.SentinelSeen = strings.Contains(text, sentinel)
	result.Completed = err == nil && result.SentinelSeen
	if usage != nil {
		result.ActualOutputTokens = usage.OutputTokens
		result.InputTokens = usage.InputTokens
		result.TotalTokens = usage.TotalTokens
	}
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func Prompt(target int, sentinel string) string {
	return fmt.Sprintf(`Generate approximately %d output tokens of plain ASCII text.
Use short numbered sentences.
Do not use markdown.
Do not stop early.
The final line must be exactly: %s`, target, sentinel)
}

func runMode(ctx context.Context, cfg Config, target int, prompt string) (string, *agent.Usage, error) {
	switch cfg.Mode {
	case ModeCodexCLI:
		return runCodexCLI(ctx, cfg, target, prompt)
	case ModeResponses:
		return runResponses(ctx, cfg, target, prompt)
	default:
		return "", nil, fmt.Errorf("unknown benchmark mode: %s", cfg.Mode)
	}
}

func runCodexCLI(ctx context.Context, cfg Config, target int, prompt string) (string, *agent.Usage, error) {
	bin, err := provider.ResolveCodexBinary(cfg.CodexBin)
	if err != nil {
		return "", nil, err
	}
	req := agent.AgentRequest{
		Model:                      cfg.Model,
		ReasoningEffort:            cfg.ReasoningEffort,
		MaxOutputTokens:            target,
		ModelContextWindow:         cfg.ModelContextWindow,
		ModelAutoCompactTokenLimit: cfg.ModelAutoCompactTokenLimit,
		CWD:                        cfg.CWD,
		Input:                      []agent.ContentPart{{Type: "text", Text: prompt}},
		Sandbox:                    agent.SandboxConfig{Mode: "read-only"},
		ApprovalPolicy:             "never",
		SkipGitRepoCheck:           true,
	}
	args, stdin := provider.BuildCodexExecArgs(req)
	cmd := exec.CommandContext(ctx, bin, args...)
	if cfg.CWD != "" {
		cmd.Dir = cfg.CWD
	}
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", nil, err
	}
	return collectCodexJSONL(bytes.NewReader(out))
}

func collectCodexJSONL(r io.Reader) (string, *agent.Usage, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	text := ""
	var usage *agent.Usage
	for scanner.Scan() {
		event := provider.ParseCodexEvent(scanner.Text())
		switch event.Type {
		case agent.EventMessage:
			if event.Message != "" {
				text = event.Message
			}
		case agent.EventDone:
			if event.Final != "" {
				text = event.Final
			}
			if event.Usage != nil {
				usage = event.Usage
			}
		case agent.EventError:
			return text, usage, fmt.Errorf("%s", event.Err)
		}
	}
	return text, usage, scanner.Err()
}

func runResponses(ctx context.Context, cfg Config, target int, prompt string) (string, *agent.Usage, error) {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080/v1"
	}
	body := map[string]any{
		"model":             firstNonEmpty(cfg.Model, "codex-default"),
		"input":             prompt,
		"max_output_tokens": target,
		"stream":            false,
	}
	if cfg.ReasoningEffort != "" {
		body["reasoning_effort"] = cfg.ReasoningEffort
	}
	if cfg.ModelContextWindow > 0 {
		body["model_context_window"] = cfg.ModelContextWindow
	}
	if cfg.ModelAutoCompactTokenLimit > 0 {
		body["model_auto_compact_token_limit"] = cfg.ModelAutoCompactTokenLimit
	}
	if cfg.CWD != "" {
		body["cwd"] = cfg.CWD
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/responses", bytes.NewReader(data))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("content-type", "application/json")
	if cfg.APIToken != "" {
		req.Header.Set("authorization", "Bearer "+cfg.APIToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var parsed struct {
		OutputText string       `json:"output_text"`
		Usage      *agent.Usage `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", nil, err
	}
	return parsed.OutputText, parsed.Usage, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
