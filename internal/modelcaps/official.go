package modelcaps

type Official struct {
	ContextWindow   int
	MaxOutputTokens int
	Source          string
	Notes           string
}

var OfficialCaps = map[string]Official{
	"gpt-5.5": {
		ContextWindow:   1050000,
		MaxOutputTokens: 128000,
		Source:          "openai-docs:gpt-5.5",
		Notes:           "Official API docs list 1.05M context and 128K max output.",
	},
	"gpt-5.4": {
		ContextWindow:   1050000,
		MaxOutputTokens: 128000,
		Source:          "openai-docs:gpt-5.4",
		Notes:           "Official API docs list 1.05M context and 128K max output.",
	},
	"gpt-5.4-mini": {
		ContextWindow:   400000,
		MaxOutputTokens: 128000,
		Source:          "openai-docs:gpt-5.4-mini",
		Notes:           "Official API docs list 400K context and 128K max output.",
	},
	"gpt-5.3-codex": {
		ContextWindow:   400000,
		MaxOutputTokens: 128000,
		Source:          "openai-docs:gpt-5.3-codex",
		Notes:           "Official API docs list 400K context and 128K max output.",
	},
	"gpt-5.2-codex": {
		ContextWindow:   400000,
		MaxOutputTokens: 128000,
		Source:          "openai-docs:gpt-5.2-codex",
		Notes:           "Official API docs list 400K context and 128K max output.",
	},
	"gpt-5.2": {
		ContextWindow:   400000,
		MaxOutputTokens: 128000,
		Source:          "openai-docs:gpt-5.2",
		Notes:           "Official API docs list 400K context and 128K max output.",
	},
	"gpt-5-codex": {
		ContextWindow:   400000,
		MaxOutputTokens: 128000,
		Source:          "openai-docs:gpt-5-codex",
		Notes:           "Official API docs list 400K context and 128K max output.",
	},
}
