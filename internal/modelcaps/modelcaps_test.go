package modelcaps

import "testing"

func TestRecommendedUsesSafeDefaultForLargeOfficialCaps(t *testing.T) {
	got := recommended(Caps{OfficialMaxOutputTokens: 128000})
	if got != 64000 {
		t.Fatalf("recommended = %d", got)
	}
}

func TestRecommendedUsesKnownSmallerOfficialCap(t *testing.T) {
	got := recommended(Caps{OfficialMaxOutputTokens: 16384})
	if got != 16384 {
		t.Fatalf("recommended = %d", got)
	}
}

func TestMarkdownTable(t *testing.T) {
	table := MarkdownTable([]Caps{{
		ID:                         "gpt-test",
		LocalContextWindow:         100,
		OfficialMaxOutputTokens:    200,
		RecommendedMaxOutputTokens: 200,
		SupportedReasoning:         []string{"low", "high"},
		Notes:                      "ok",
	}})
	if table == "" || table[0] != '|' {
		t.Fatalf("bad table: %q", table)
	}
}
