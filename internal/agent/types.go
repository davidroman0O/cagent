package agent

import (
	"context"
	"time"
)

type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Path     string `json:"path,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Data     string `json:"data,omitempty"`
}

type SandboxConfig struct {
	Mode                    string `json:"mode,omitempty"`
	DangerouslyBypassSafety bool   `json:"dangerously_bypass_safety,omitempty"`
}

type AgentRequest struct {
	SessionID                  string         `json:"session_id,omitempty"`
	ProviderSessionID          string         `json:"provider_session_id,omitempty"`
	Provider                   string         `json:"provider,omitempty"`
	Model                      string         `json:"model,omitempty"`
	ReasoningEffort            string         `json:"reasoning_effort,omitempty"`
	MaxOutputTokens            int            `json:"max_output_tokens,omitempty"`
	ModelContextWindow         int            `json:"model_context_window,omitempty"`
	ModelAutoCompactTokenLimit int            `json:"model_auto_compact_token_limit,omitempty"`
	CWD                        string         `json:"cwd,omitempty"`
	Input                      []ContentPart  `json:"input,omitempty"`
	Images                     []string       `json:"images,omitempty"`
	OutputSchemaPath           string         `json:"output_schema_path,omitempty"`
	Sandbox                    SandboxConfig  `json:"sandbox,omitempty"`
	ApprovalPolicy             string         `json:"approval_policy,omitempty"`
	SkipGitRepoCheck           bool           `json:"skip_git_repo_check"`
	Metadata                   map[string]any `json:"metadata,omitempty"`
}

type Usage struct {
	InputTokens           int `json:"input_tokens,omitempty"`
	OutputTokens          int `json:"output_tokens,omitempty"`
	TotalTokens           int `json:"total_tokens,omitempty"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens,omitempty"`
	CachedInputTokens     int `json:"cached_input_tokens,omitempty"`
}

type EventType string

const (
	EventStarted         EventType = "started"
	EventProgress        EventType = "progress"
	EventMessage         EventType = "message"
	EventDelta           EventType = "delta"
	EventCommand         EventType = "command"
	EventFileChange      EventType = "file_change"
	EventApprovalRequest EventType = "approval_request"
	EventQuestion        EventType = "question"
	EventUsage           EventType = "usage"
	EventDone            EventType = "done"
	EventError           EventType = "error"
)

type AgentEvent struct {
	Type              EventType      `json:"type"`
	SessionID         string         `json:"session_id,omitempty"`
	ProviderSessionID string         `json:"provider_session_id,omitempty"`
	TurnID            string         `json:"turn_id,omitempty"`
	Message           string         `json:"message,omitempty"`
	Delta             string         `json:"delta,omitempty"`
	Final             string         `json:"final,omitempty"`
	Usage             *Usage         `json:"usage,omitempty"`
	Data              map[string]any `json:"data,omitempty"`
	Err               string         `json:"error,omitempty"`
	Raw               map[string]any `json:"raw,omitempty"`
	At                time.Time      `json:"at"`
}

func NewEvent(t EventType) AgentEvent {
	return AgentEvent{Type: t, At: time.Now().UTC()}
}

type ProviderCapabilities struct {
	Streaming        bool `json:"streaming"`
	Resume           bool `json:"resume"`
	Images           bool `json:"images"`
	Files            bool `json:"files"`
	StructuredOutput bool `json:"structured_output"`
	Approvals        bool `json:"approvals"`
	Questions        bool `json:"questions"`
	Commands         bool `json:"commands"`
	FileChanges      bool `json:"file_changes"`
}

type Provider interface {
	Name() string
	Capabilities() ProviderCapabilities
	Run(ctx context.Context, req AgentRequest) (<-chan AgentEvent, error)
}
