package compat

import "strings"

func ParseModel(raw string) (model string, reasoningEffort string, provider string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "codex" || raw == "codex-default" || raw == "default" {
		return "", "", ""
	}
	parts := strings.Split(raw, ":")
	if len(parts) > 1 && isProviderPrefix(parts[0]) {
		provider = parts[0]
		parts = parts[1:]
	}
	if len(parts) > 1 && isReasoningEffort(parts[len(parts)-1]) {
		reasoningEffort = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}
	model = strings.Join(parts, ":")
	if model == "codex-default" || model == "default" {
		model = ""
	}
	return model, reasoningEffort, provider
}

func isProviderPrefix(value string) bool {
	switch value {
	case "codex", "claude-code", "gemini-cli", "cursor", "aider", "opencode":
		return true
	default:
		return false
	}
}

func isReasoningEffort(value string) bool {
	switch value {
	case "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}
