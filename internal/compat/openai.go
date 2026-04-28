package compat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/davidroman0O/cagent/internal/agent"
)

type ChatCompletionRequest struct {
	Model                      string          `json:"model"`
	Messages                   []ChatMessage   `json:"messages"`
	Stream                     bool            `json:"stream"`
	User                       string          `json:"user,omitempty"`
	SessionID                  string          `json:"session_id,omitempty"`
	ConversationID             string          `json:"conversation_id,omitempty"`
	ThreadID                   string          `json:"thread_id,omitempty"`
	Provider                   string          `json:"provider,omitempty"`
	ReasoningEffort            string          `json:"reasoning_effort,omitempty"`
	Reasoning                  ReasoningConfig `json:"reasoning,omitempty"`
	MaxTokens                  int             `json:"max_tokens,omitempty"`
	MaxOutputTokens            int             `json:"max_output_tokens,omitempty"`
	MaxCompletionTokens        int             `json:"max_completion_tokens,omitempty"`
	ModelContextWindow         int             `json:"model_context_window,omitempty"`
	ModelAutoCompactTokenLimit int             `json:"model_auto_compact_token_limit,omitempty"`
	CWD                        string          `json:"cwd,omitempty"`
	SandboxMode                string          `json:"sandbox_mode,omitempty"`
	ApprovalPolicy             string          `json:"approval_policy,omitempty"`
	Metadata                   map[string]any  `json:"metadata,omitempty"`
	ResponseFormat             json.RawMessage `json:"response_format,omitempty"`
}

type ChatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	Name       string          `json:"name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
}

type ResponsesRequest struct {
	Model                      string          `json:"model"`
	Input                      json.RawMessage `json:"input"`
	Instructions               string          `json:"instructions,omitempty"`
	Stream                     bool            `json:"stream"`
	Tools                      json.RawMessage `json:"tools,omitempty"`
	ToolChoice                 json.RawMessage `json:"tool_choice,omitempty"`
	SessionID                  string          `json:"session_id,omitempty"`
	ConversationID             string          `json:"conversation_id,omitempty"`
	ThreadID                   string          `json:"thread_id,omitempty"`
	Provider                   string          `json:"provider,omitempty"`
	ReasoningEffort            string          `json:"reasoning_effort,omitempty"`
	Reasoning                  ReasoningConfig `json:"reasoning,omitempty"`
	MaxOutputTokens            int             `json:"max_output_tokens,omitempty"`
	MaxTokens                  int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens        int             `json:"max_completion_tokens,omitempty"`
	ModelContextWindow         int             `json:"model_context_window,omitempty"`
	ModelAutoCompactTokenLimit int             `json:"model_auto_compact_token_limit,omitempty"`
	CWD                        string          `json:"cwd,omitempty"`
	SandboxMode                string          `json:"sandbox_mode,omitempty"`
	ApprovalPolicy             string          `json:"approval_policy,omitempty"`
	Metadata                   map[string]any  `json:"metadata,omitempty"`
}

type ReasoningConfig struct {
	Effort string `json:"effort,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   *Usage       `json:"usage,omitempty"`
}

type ChatChoice struct {
	Index        int            `json:"index"`
	Message      ChatMessageOut `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type ChatMessageOut struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type ResponsesObject struct {
	ID         string            `json:"id"`
	Object     string            `json:"object"`
	CreatedAt  int64             `json:"created_at"`
	Status     string            `json:"status"`
	Model      string            `json:"model"`
	Output     []ResponsesOutput `json:"output"`
	OutputText string            `json:"output_text"`
	Usage      *agent.Usage      `json:"usage,omitempty"`
}

type ResponsesOutput struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Role      string                 `json:"role,omitempty"`
	Content   []ResponsesContentPart `json:"content,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
	Status    string                 `json:"status,omitempty"`
}

type ResponsesContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func ChatToAgent(req ChatCompletionRequest, defaults AgentDefaults) agent.AgentRequest {
	sessionID := firstNonEmpty(req.SessionID, req.ConversationID, req.ThreadID)
	model, modelEffort, modelProvider := ParseModel(req.Model)
	defaultModel, defaultEffort, defaultProvider := ParseModel(defaults.Model)
	parts := []agent.ContentPart{{Type: "text", Text: chatPrompt(req.Messages, sessionID != "")}}
	parts = append(parts, imagePartsFromMessages(req.Messages)...)
	return agent.AgentRequest{
		SessionID:                  sessionID,
		Provider:                   firstNonEmpty(req.Provider, modelProvider, defaultProvider),
		Model:                      firstNonEmpty(model, defaultModel),
		ReasoningEffort:            firstNonEmpty(req.ReasoningEffort, req.Reasoning.Effort, stringFromMap(req.Metadata, "reasoning_effort"), stringFromMap(req.Metadata, "model_reasoning_effort"), modelEffort, defaults.ReasoningEffort, defaultEffort),
		MaxOutputTokens:            firstPositive(req.MaxOutputTokens, req.MaxCompletionTokens, req.MaxTokens, intFromMap(req.Metadata, "max_output_tokens"), intFromMap(req.Metadata, "max_tokens")),
		ModelContextWindow:         firstPositive(req.ModelContextWindow, intFromMap(req.Metadata, "model_context_window"), intFromMap(req.Metadata, "context_window"), intFromMap(req.Metadata, "max_context_tokens")),
		ModelAutoCompactTokenLimit: firstPositive(req.ModelAutoCompactTokenLimit, intFromMap(req.Metadata, "model_auto_compact_token_limit"), intFromMap(req.Metadata, "auto_compact_token_limit")),
		CWD:                        firstNonEmpty(req.CWD, stringFromMap(req.Metadata, "cwd"), defaults.CWD),
		Input:                      parts,
		Sandbox:                    agent.SandboxConfig{Mode: firstNonEmpty(req.SandboxMode, stringFromMap(req.Metadata, "sandbox_mode"))},
		ApprovalPolicy:             firstNonEmpty(req.ApprovalPolicy, stringFromMap(req.Metadata, "approval_policy")),
		SkipGitRepoCheck:           true,
		Metadata:                   req.Metadata,
	}
}

func ResponsesToAgent(req ResponsesRequest, defaults AgentDefaults) agent.AgentRequest {
	sessionID := firstNonEmpty(req.SessionID, req.ConversationID, req.ThreadID)
	model, modelEffort, modelProvider := ParseModel(req.Model)
	defaultModel, defaultEffort, defaultProvider := ParseModel(defaults.Model)
	text := responsesPrompt(req)
	return agent.AgentRequest{
		SessionID:                  sessionID,
		Provider:                   firstNonEmpty(req.Provider, modelProvider, defaultProvider),
		Model:                      firstNonEmpty(model, defaultModel),
		ReasoningEffort:            firstNonEmpty(req.ReasoningEffort, req.Reasoning.Effort, stringFromMap(req.Metadata, "reasoning_effort"), stringFromMap(req.Metadata, "model_reasoning_effort"), modelEffort, defaults.ReasoningEffort, defaultEffort),
		MaxOutputTokens:            firstPositive(req.MaxOutputTokens, req.MaxCompletionTokens, req.MaxTokens, intFromMap(req.Metadata, "max_output_tokens"), intFromMap(req.Metadata, "max_tokens")),
		ModelContextWindow:         firstPositive(req.ModelContextWindow, intFromMap(req.Metadata, "model_context_window"), intFromMap(req.Metadata, "context_window"), intFromMap(req.Metadata, "max_context_tokens")),
		ModelAutoCompactTokenLimit: firstPositive(req.ModelAutoCompactTokenLimit, intFromMap(req.Metadata, "model_auto_compact_token_limit"), intFromMap(req.Metadata, "auto_compact_token_limit")),
		CWD:                        firstNonEmpty(req.CWD, stringFromMap(req.Metadata, "cwd"), defaults.CWD),
		Input:                      []agent.ContentPart{{Type: "text", Text: text}},
		Sandbox:                    agent.SandboxConfig{Mode: firstNonEmpty(req.SandboxMode, stringFromMap(req.Metadata, "sandbox_mode"))},
		ApprovalPolicy:             firstNonEmpty(req.ApprovalPolicy, stringFromMap(req.Metadata, "approval_policy")),
		SkipGitRepoCheck:           true,
		Metadata:                   req.Metadata,
	}
}

type AgentDefaults struct {
	Model           string
	ReasoningEffort string
	CWD             string
}

func NewChatCompletion(id, model, text string, usage *agent.Usage) ChatCompletionResponse {
	return ChatCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatChoice{{
			Index: 0,
			Message: ChatMessageOut{
				Role:    "assistant",
				Content: text,
			},
			FinishReason: "stop",
		}},
		Usage: toOpenAIUsage(usage),
	}
}

func NewResponsesObject(id, model, text string, usage *agent.Usage) ResponsesObject {
	return ResponsesObject{
		ID:         id,
		Object:     "response",
		CreatedAt:  time.Now().Unix(),
		Status:     "completed",
		Model:      model,
		OutputText: text,
		Usage:      usage,
		Output: []ResponsesOutput{{
			ID:   "msg_" + id,
			Type: "message",
			Role: "assistant",
			Content: []ResponsesContentPart{{
				Type: "output_text",
				Text: text,
			}},
		}},
	}
}

func NewResponsesFunctionCallObject(id, model string, call ResponsesToolCall, usage *agent.Usage) ResponsesObject {
	itemID := "fc_" + id
	callID := "call_" + id
	return ResponsesObject{
		ID:        id,
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Status:    "completed",
		Model:     model,
		Usage:     usage,
		Output: []ResponsesOutput{{
			ID:        itemID,
			Type:      "function_call",
			CallID:    callID,
			Name:      call.Name,
			Arguments: call.ArgumentsString(),
			Status:    "completed",
		}},
	}
}

func toOpenAIUsage(usage *agent.Usage) *Usage {
	if usage == nil {
		return nil
	}
	total := usage.TotalTokens
	if total == 0 {
		total = usage.InputTokens + usage.OutputTokens
	}
	return &Usage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      total,
	}
}

func chatPrompt(messages []ChatMessage, sessionMode bool) string {
	if len(messages) == 0 {
		return ""
	}
	selected := messages
	if sessionMode {
		selected = messagesForSessionTurn(messages)
	}
	var b strings.Builder
	for _, msg := range selected {
		text := messageText(msg)
		if text == "" && len(msg.ToolCalls) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("[")
		b.WriteString(strings.ToUpper(msg.Role))
		if msg.Name != "" {
			b.WriteString(" ")
			b.WriteString(msg.Name)
		}
		if msg.ToolCallID != "" {
			b.WriteString(" tool_call_id=")
			b.WriteString(msg.ToolCallID)
		}
		b.WriteString("]\n")
		b.WriteString(text)
		if len(msg.ToolCalls) > 0 && string(msg.ToolCalls) != "null" {
			b.WriteString("\nTool calls: ")
			b.Write(msg.ToolCalls)
		}
	}
	return b.String()
}

func messagesForSessionTurn(messages []ChatMessage) []ChatMessage {
	selected := make([]ChatMessage, 0)
	for _, msg := range messages {
		if msg.Role == "system" || msg.Role == "developer" {
			selected = append(selected, msg)
		}
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" || messages[i].Role == "tool" {
			selected = append(selected, messages[i])
			break
		}
	}
	return selected
}

func messageText(msg ChatMessage) string {
	if len(msg.Content) == 0 || string(msg.Content) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		return s
	}
	var parts []map[string]any
	if err := json.Unmarshal(msg.Content, &parts); err == nil {
		var b strings.Builder
		for _, part := range parts {
			t := stringFromMap(part, "type")
			switch t {
			case "text", "input_text":
				writeSeparated(&b, stringFromMap(part, "text"))
			case "image_url", "input_image":
				url := stringFromMap(part, "image_url")
				if url == "" {
					if image := mapFromAny(part["image_url"]); image != nil {
						url = stringFromMap(image, "url")
					}
				}
				if url != "" {
					writeSeparated(&b, "[image: "+url+"]")
				}
			case "file", "input_file":
				path := stringFromMap(part, "file_id")
				if path == "" {
					path = stringFromMap(part, "filename")
				}
				if path != "" {
					writeSeparated(&b, "[file: "+path+"]")
				}
			}
		}
		return b.String()
	}
	var raw any
	if err := json.Unmarshal(msg.Content, &raw); err == nil {
		return fmt.Sprintf("%v", raw)
	}
	return string(msg.Content)
}

func imagePartsFromMessages(messages []ChatMessage) []agent.ContentPart {
	var out []agent.ContentPart
	for _, msg := range messages {
		var parts []map[string]any
		if err := json.Unmarshal(msg.Content, &parts); err != nil {
			continue
		}
		for _, part := range parts {
			t := stringFromMap(part, "type")
			if t != "image_url" && t != "input_image" && t != "local_image" {
				continue
			}
			path := stringFromMap(part, "path")
			if path == "" {
				path = stringFromMap(part, "image_url")
			}
			if image := mapFromAny(part["image_url"]); path == "" && image != nil {
				path = stringFromMap(image, "url")
			}
			if strings.HasPrefix(path, "file://") {
				path = strings.TrimPrefix(path, "file://")
			}
			if path != "" && !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") && !strings.HasPrefix(path, "data:") {
				out = append(out, agent.ContentPart{Type: "local_image", Path: path})
			}
		}
	}
	return out
}

func responsesPrompt(req ResponsesRequest) string {
	var b strings.Builder
	if req.Instructions != "" {
		b.WriteString("[SYSTEM]\n")
		b.WriteString(req.Instructions)
	}
	input := strings.TrimSpace(rawInputText(req.Input))
	if input != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(input)
	}
	if bridge := responsesToolBridgePrompt(req); bridge != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(bridge)
	}
	return b.String()
}

func rawInputText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err == nil {
		var b strings.Builder
		for _, item := range items {
			role := stringFromMap(item, "role")
			if role == "" {
				role = stringFromMap(item, "type")
			}
			content := contentTextFromAny(item["content"])
			if content == "" {
				content = stringFromMap(item, "text")
			}
			if content == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString("[")
			b.WriteString(strings.ToUpper(role))
			b.WriteString("]\n")
			b.WriteString(content)
		}
		return b.String()
	}
	return string(raw)
}

func contentTextFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var b strings.Builder
		for _, elem := range typed {
			part := mapFromAny(elem)
			if part == nil {
				continue
			}
			text := stringFromMap(part, "text")
			if text == "" {
				text = stringFromMap(part, "input_text")
			}
			writeSeparated(&b, text)
		}
		return b.String()
	default:
		return ""
	}
}

func writeSeparated(b *strings.Builder, text string) {
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringFromMap(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

func intFromMap(data map[string]any, key string) int {
	if data == nil {
		return 0
	}
	value, ok := data[key]
	if !ok || value == nil {
		return 0
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
	default:
		return 0
	}
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
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
