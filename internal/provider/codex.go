package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/davidroman0O/cagent/internal/agent"
)

type CodexProvider struct {
	Bin                        string
	ModelContextWindow         int
	ModelAutoCompactTokenLimit int
}

func NewCodexProvider(bin string) (*CodexProvider, error) {
	return NewCodexProviderWithOptions(CodexOptions{Bin: bin})
}

type CodexOptions struct {
	Bin                        string
	ModelContextWindow         int
	ModelAutoCompactTokenLimit int
}

func NewCodexProviderWithOptions(opts CodexOptions) (*CodexProvider, error) {
	resolved, err := ResolveCodexBinary(opts.Bin)
	if err != nil {
		return nil, err
	}
	return &CodexProvider{
		Bin:                        resolved,
		ModelContextWindow:         positive(opts.ModelContextWindow),
		ModelAutoCompactTokenLimit: positive(opts.ModelAutoCompactTokenLimit),
	}, nil
}

func (p *CodexProvider) Name() string {
	return "codex"
}

func (p *CodexProvider) Capabilities() agent.ProviderCapabilities {
	return agent.ProviderCapabilities{
		Streaming:        true,
		Resume:           true,
		Images:           true,
		Files:            false,
		StructuredOutput: true,
		Approvals:        true,
		Questions:        true,
		Commands:         true,
		FileChanges:      true,
	}
}

func (p *CodexProvider) Run(ctx context.Context, req agent.AgentRequest) (<-chan agent.AgentEvent, error) {
	args, prompt := p.BuildExecArgs(req)
	cmd := exec.CommandContext(ctx, p.Bin, args...)
	if req.CWD != "" {
		cmd.Dir = req.CWD
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	events := make(chan agent.AgentEvent, 32)
	go func() {
		defer close(events)

		stderrDone := make(chan string, 1)
		go func() {
			data, _ := io.ReadAll(stderr)
			stderrDone <- strings.TrimSpace(string(data))
		}()

		go func() {
			_, _ = io.WriteString(stdin, prompt)
			_ = stdin.Close()
		}()

		lastMessage := ""
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			event := ParseCodexEvent(line)
			event.SessionID = req.SessionID
			event.TurnID = stringFromMap(event.Data, "turn_id")
			if event.Type == agent.EventMessage && event.Message != "" {
				lastMessage = event.Message
			}
			if event.Type == agent.EventDone && event.Final == "" {
				event.Final = lastMessage
			}
			events <- event
		}
		if err := scanner.Err(); err != nil {
			events <- errorEvent(fmt.Errorf("read codex stdout: %w", err))
		}

		waitErr := cmd.Wait()
		stderrText := <-stderrDone
		if waitErr != nil {
			if ctx.Err() != nil {
				events <- errorEvent(ctx.Err())
				return
			}
			if stderrText != "" {
				events <- errorEvent(fmt.Errorf("%w: %s", waitErr, stderrText))
				return
			}
			events <- errorEvent(waitErr)
			return
		}
		if lastMessage != "" {
			return
		}
		if stderrText != "" && !isKnownNonFatalStderr(stderrText) {
			ev := agent.NewEvent(agent.EventProgress)
			ev.Message = stderrText
			events <- ev
		}
	}()

	return events, nil
}

func (p *CodexProvider) BuildExecArgs(req agent.AgentRequest) ([]string, string) {
	return BuildCodexExecArgsWithConfig(req, CodexExecConfig{
		ModelContextWindow:         p.ModelContextWindow,
		ModelAutoCompactTokenLimit: p.ModelAutoCompactTokenLimit,
	})
}

func ResolveCodexBinary(explicit string) (string, error) {
	if explicit != "" {
		return validateExecutable(explicit)
	}
	names := []string{"codex"}
	if runtime.GOOS == "windows" {
		names = []string{"codex.cmd", "codex.exe", "codex"}
	}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	for _, candidate := range commonCodexPaths() {
		if path, err := validateExecutable(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("codex executable not found; set CAGENT_CODEX_BIN")
}

func commonCodexPaths() []string {
	home, _ := os.UserHomeDir()
	paths := []string{
		"/opt/homebrew/bin/codex",
		"/usr/local/bin/codex",
	}
	if home != "" {
		paths = append(paths,
			filepath.Join(home, ".bun", "bin", "codex"),
			filepath.Join(home, ".npm-global", "bin", "codex"),
		)
	}
	return paths
}

func validateExecutable(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
		return "", fmt.Errorf("%s is not executable", path)
	}
	return path, nil
}

type CodexExecConfig struct {
	ModelContextWindow         int
	ModelAutoCompactTokenLimit int
}

func BuildCodexExecArgs(req agent.AgentRequest) ([]string, string) {
	return BuildCodexExecArgsWithConfig(req, CodexExecConfig{})
}

func BuildCodexExecArgsWithConfig(req agent.AgentRequest, cfg CodexExecConfig) ([]string, string) {
	prompt := PromptFromContent(req.Input)
	args := []string{"exec"}
	if req.ProviderSessionID != "" {
		args = append(args, "resume")
	}

	if req.Sandbox.DangerouslyBypassSafety {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else if req.Sandbox.Mode != "" {
		args = append(args, "--sandbox", req.Sandbox.Mode)
	}
	if req.ApprovalPolicy != "" {
		args = append(args, "-c", fmt.Sprintf("approval_policy=%q", req.ApprovalPolicy))
	}
	if req.ReasoningEffort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", req.ReasoningEffort))
	}
	if contextWindow := firstPositive(req.ModelContextWindow, cfg.ModelContextWindow); contextWindow > 0 {
		args = append(args, "-c", fmt.Sprintf("model_context_window=%d", contextWindow))
	}
	if compactLimit := firstPositive(req.ModelAutoCompactTokenLimit, cfg.ModelAutoCompactTokenLimit); compactLimit > 0 {
		args = append(args, "-c", fmt.Sprintf("model_auto_compact_token_limit=%d", compactLimit))
	}
	args = append(args, "--json")
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.CWD != "" {
		args = append(args, "--cd", req.CWD)
	}
	if req.SkipGitRepoCheck {
		args = append(args, "--skip-git-repo-check")
	}
	for _, image := range append(req.Images, imagesFromContent(req.Input)...) {
		if image != "" {
			args = append(args, "--image", image)
		}
	}
	if req.OutputSchemaPath != "" {
		args = append(args, "--output-schema", req.OutputSchemaPath)
	}
	if req.ProviderSessionID != "" {
		args = append(args, req.ProviderSessionID)
	}
	args = append(args, "-")
	return args, prompt
}

func positive(value int) int {
	if value > 0 {
		return value
	}
	return 0
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func PromptFromContent(parts []agent.ContentPart) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		switch part.Type {
		case "", "text", "input_text":
			if part.Text != "" {
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
				b.WriteString(part.Text)
			}
		case "file", "input_file":
			if part.Path != "" {
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
				b.WriteString("[file: ")
				b.WriteString(part.Path)
				b.WriteString("]")
			}
		}
	}
	return b.String()
}

func imagesFromContent(parts []agent.ContentPart) []string {
	images := make([]string, 0)
	for _, part := range parts {
		if (part.Type == "image" || part.Type == "input_image" || part.Type == "local_image") && part.Path != "" {
			images = append(images, part.Path)
		}
	}
	return images
}

func ParseCodexEvent(line string) agent.AgentEvent {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		ev := agent.NewEvent(agent.EventProgress)
		ev.Message = line
		return ev
	}

	ev := agent.NewEvent(agent.EventProgress)
	ev.Raw = raw
	ev.Data = copyMap(raw)

	eventType := stringFromMap(raw, "type")
	switch eventType {
	case "thread.started", "session.started":
		ev.Type = agent.EventStarted
		ev.ProviderSessionID = firstString(raw, "session_id", "thread_id", "id")
		ev.Message = "thread started"
	case "turn.started":
		ev.Type = agent.EventStarted
		ev.Message = "turn started"
	case "turn.completed":
		ev.Type = agent.EventDone
		ev.Final = firstString(raw, "output", "final", "last_message", "text")
		ev.Usage = usageFromAny(raw["usage"])
	case "turn.failed", "error":
		ev.Type = agent.EventError
		ev.Err = firstString(raw, "error", "message", "details")
		if ev.Err == "" {
			ev.Err = "codex turn failed"
		}
	case "item.completed", "item.updated", "item.started":
		item := mapFromAny(raw["item"])
		parseCodexItem(&ev, item, eventType)
	default:
		ev.Message = line
	}
	return ev
}

func parseCodexItem(ev *agent.AgentEvent, item map[string]any, eventType string) {
	if len(item) == 0 {
		ev.Message = eventType
		return
	}
	itemType := stringFromMap(item, "type")
	ev.Data = copyMap(item)
	switch itemType {
	case "agent_message", "message":
		ev.Type = agent.EventMessage
		ev.Message = firstString(item, "text", "message", "content", "output")
	case "reasoning", "todo_list":
		ev.Type = agent.EventProgress
		ev.Message = firstString(item, "text", "summary", "message", "content")
	case "command_execution", "shell_call", "local_shell_call":
		ev.Type = agent.EventCommand
		ev.Message = firstString(item, "command", "cmd", "text", "message")
	case "file_change":
		ev.Type = agent.EventFileChange
		ev.Message = firstString(item, "path", "file", "message")
	case "mcp_approval_request", "approval_request":
		ev.Type = agent.EventApprovalRequest
		ev.Message = firstString(item, "message", "reason", "command")
	case "question", "input_request", "elicitation":
		ev.Type = agent.EventQuestion
		ev.Message = firstString(item, "message", "question", "prompt")
	default:
		ev.Type = agent.EventProgress
		ev.Message = firstString(item, "text", "message", "content")
	}
}

func usageFromAny(value any) *agent.Usage {
	data := mapFromAny(value)
	if len(data) == 0 {
		return nil
	}
	usage := &agent.Usage{
		InputTokens:           intFromMap(data, "input_tokens", "prompt_tokens"),
		OutputTokens:          intFromMap(data, "output_tokens", "completion_tokens"),
		TotalTokens:           intFromMap(data, "total_tokens"),
		ReasoningOutputTokens: intFromMap(data, "reasoning_output_tokens"),
		CachedInputTokens:     intFromMap(data, "cached_input_tokens"),
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return usage
}

func errorEvent(err error) agent.AgentEvent {
	ev := agent.NewEvent(agent.EventError)
	ev.Err = err.Error()
	return ev
}

func isKnownNonFatalStderr(text string) bool {
	return strings.Contains(text, "Reading additional input from stdin")
}

func copyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mapFromAny(value any) map[string]any {
	if value == nil {
		return nil
	}
	if data, ok := value.(map[string]any); ok {
		return data
	}
	return nil
}

func firstString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringFromMap(data, key); value != "" {
			return value
		}
	}
	return ""
}

func stringFromMap(data map[string]any, key string) string {
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func intFromMap(data map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := data[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case int:
			return typed
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		case json.Number:
			parsed, _ := typed.Int64()
			return int(parsed)
		}
	}
	return 0
}

func NewCodexSmokeCommand(ctx context.Context, bin string) *exec.Cmd {
	return exec.CommandContext(ctx, bin, "--version")
}
