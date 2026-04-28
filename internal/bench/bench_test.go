package bench

import (
	"strings"
	"testing"
)

func TestPromptIncludesTargetAndSentinel(t *testing.T) {
	prompt := Prompt(1234, "SENTINEL")
	if !strings.Contains(prompt, "1234") {
		t.Fatalf("prompt missing target: %s", prompt)
	}
	if !strings.Contains(prompt, "SENTINEL") {
		t.Fatalf("prompt missing sentinel: %s", prompt)
	}
}

func TestCollectCodexJSONL(t *testing.T) {
	input := strings.NewReader(`{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}
{"type":"turn.completed","usage":{"input_tokens":3,"output_tokens":2}}
`)
	text, usage, err := collectCodexJSONL(input)
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello" {
		t.Fatalf("text = %q", text)
	}
	if usage == nil || usage.OutputTokens != 2 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %#v", usage)
	}
}
