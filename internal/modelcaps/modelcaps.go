package modelcaps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/davidroman0O/cagent/internal/provider"
)

type Caps struct {
	ID                         string   `json:"id"`
	DisplayName                string   `json:"display_name,omitempty"`
	LocalContextWindow         int      `json:"local_context_window,omitempty"`
	LocalMaxContextWindow      int      `json:"local_max_context_window,omitempty"`
	LocalMaxOutputTokens       int      `json:"local_max_output_tokens,omitempty"`
	ConfiguredContextWindow    int      `json:"configured_context_window,omitempty"`
	OfficialContextWindow      int      `json:"official_context_window,omitempty"`
	OfficialMaxOutputTokens    int      `json:"official_max_output_tokens,omitempty"`
	RecommendedMaxOutputTokens int      `json:"recommended_max_output_tokens,omitempty"`
	SupportedInAPI             bool     `json:"supported_in_api,omitempty"`
	DefaultReasoning           string   `json:"default_reasoning,omitempty"`
	SupportedReasoning         []string `json:"supported_reasoning,omitempty"`
	SpeedTiers                 []string `json:"speed_tiers,omitempty"`
	Sources                    []string `json:"sources,omitempty"`
	Notes                      string   `json:"notes,omitempty"`
}

type localCatalog struct {
	Models []localModel `json:"models"`
}

type localModel struct {
	Slug                     string           `json:"slug"`
	DisplayName              string           `json:"display_name"`
	ContextWindow            int              `json:"context_window"`
	MaxContextWindow         int              `json:"max_context_window"`
	MaxOutputTokens          int              `json:"max_output_tokens"`
	SupportedInAPI           bool             `json:"supported_in_api"`
	DefaultReasoningLevel    string           `json:"default_reasoning_level"`
	SupportedReasoningLevels []reasoningLevel `json:"supported_reasoning_levels"`
	AdditionalSpeedTiers     []string         `json:"additional_speed_tiers"`
}

type reasoningLevel struct {
	Effort string `json:"effort"`
}

func Build(ctx context.Context, codexBin, configPath string) ([]Caps, error) {
	models, err := loadLocalCodexModels(ctx, codexBin)
	if err != nil {
		return nil, err
	}
	configuredContext := readConfiguredContextWindow(configPath)
	out := make([]Caps, 0, len(models))
	for _, model := range models {
		caps := Caps{
			ID:                      model.Slug,
			DisplayName:             model.DisplayName,
			LocalContextWindow:      model.ContextWindow,
			LocalMaxContextWindow:   model.MaxContextWindow,
			LocalMaxOutputTokens:    model.MaxOutputTokens,
			ConfiguredContextWindow: configuredContext,
			SupportedInAPI:          model.SupportedInAPI,
			DefaultReasoning:        model.DefaultReasoningLevel,
			SupportedReasoning:      reasoningEfforts(model.SupportedReasoningLevels),
			SpeedTiers:              model.AdditionalSpeedTiers,
			Sources:                 []string{"local-codex-catalog"},
		}
		if official, ok := OfficialCaps[model.Slug]; ok {
			caps.OfficialContextWindow = official.ContextWindow
			caps.OfficialMaxOutputTokens = official.MaxOutputTokens
			caps.Sources = append(caps.Sources, official.Source)
			caps.Notes = official.Notes
		}
		caps.RecommendedMaxOutputTokens = recommended(caps)
		out = append(out, caps)
	}
	return out, nil
}

func MarkdownTable(caps []Caps) string {
	var b strings.Builder
	b.WriteString("| Model | Local context | Local max context | Config context | Official context | Official max output | Recommended Droid output | Reasoning | Notes |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- |\n")
	for _, cap := range caps {
		fmt.Fprintf(
			&b,
			"| `%s` | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			cap.ID,
			formatInt(cap.LocalContextWindow),
			formatInt(cap.LocalMaxContextWindow),
			formatInt(cap.ConfiguredContextWindow),
			formatInt(cap.OfficialContextWindow),
			formatInt(cap.OfficialMaxOutputTokens),
			formatInt(cap.RecommendedMaxOutputTokens),
			escape(strings.Join(cap.SupportedReasoning, "/")),
			escape(cap.Notes),
		)
	}
	return b.String()
}

func loadLocalCodexModels(ctx context.Context, codexBin string) ([]localModel, error) {
	bin, err := provider.ResolveCodexBinary(codexBin)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, "debug", "models")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("codex debug models: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	var catalog localCatalog
	if err := json.Unmarshal(out, &catalog); err != nil {
		return nil, err
	}
	return catalog.Models, nil
}

func readConfiguredContextWindow(configPath string) int {
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return 0
		}
		configPath = home + "/.codex/config.toml"
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "model_context_window") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"`)
		parsed, _ := strconv.Atoi(value)
		return parsed
	}
	return 0
}

func reasoningEfforts(levels []reasoningLevel) []string {
	out := make([]string, 0, len(levels))
	for _, level := range levels {
		if level.Effort != "" {
			out = append(out, level.Effort)
		}
	}
	return out
}

func recommended(c Caps) int {
	if c.OfficialMaxOutputTokens >= 128000 {
		return 64000
	}
	if c.OfficialMaxOutputTokens > 0 {
		return c.OfficialMaxOutputTokens
	}
	if c.LocalMaxOutputTokens > 0 {
		return c.LocalMaxOutputTokens
	}
	return 32768
}

func formatInt(value int) string {
	if value <= 0 {
		return "unknown"
	}
	return strconv.Itoa(value)
}

func escape(value string) string {
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
