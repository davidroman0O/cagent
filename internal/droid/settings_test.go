package droid

import "testing"

func TestApplySettingsSetsStableCustomMissionDefaultsAndCompaction(t *testing.T) {
	settings := map[string]any{
		"sessionDefaultSettings": map[string]any{
			"interactionMode": "auto",
			"autonomyLevel":   "high",
			"model":           "kimi-k2.6",
			"reasoningEffort": "high",
		},
		"missionOrchestratorModel": "claude-opus-4-7",
		"missionModelSettings": map[string]any{
			"workerModel":                     "kimi-k2.6",
			"workerReasoningEffort":           "high",
			"validationWorkerModel":           "gpt-5.3-codex",
			"validationWorkerReasoningEffort": "high",
			"skipUserTesting":                 false,
		},
		"customModels": []any{
			map[string]any{
				"model":       "codex:gpt-5.5:xhigh",
				"displayName": "cagent GPT-5.5 XHigh 128K Max",
				"baseUrl":     "http://localhost:8080/v1",
				"apiKey":      "old",
				"provider":    "openai",
			},
			map[string]any{
				"model":        "other-model",
				"id":           "custom:other",
				"displayName":  "other",
				"baseUrl":      "https://example.com/v1",
				"apiKey":       "secret",
				"provider":     "openai",
				"extraHeaders": map[string]any{"x-test": "1"},
			},
		},
	}

	opts := NormalizeSetupOptions(SetupOptions{
		BaseURL:            "http://localhost:8080/v1",
		APIToken:           "local-cagent-token",
		SetSessionDefault:  true,
		SetMissionDefaults: true,
		SkipScrutiny:       true,
		SkipUserTesting:    true,
	})
	result := ApplySettings(settings, opts)
	wantID := "custom:cagent-gpt-5-5-xhigh-128k-max"
	if result.SelectedModelID != wantID {
		t.Fatalf("selected id = %q, want %q", result.SelectedModelID, wantID)
	}

	session := settings["sessionDefaultSettings"].(map[string]any)
	if session["model"] != wantID || session["reasoningEffort"] != "xhigh" {
		t.Fatalf("session default = %#v", session)
	}
	if settings["missionOrchestratorModel"] != wantID || settings["missionOrchestratorReasoningEffort"] != "xhigh" {
		t.Fatalf("mission orchestrator not set: %#v", settings)
	}
	mission := settings["missionModelSettings"].(map[string]any)
	if mission["workerModel"] != wantID || mission["validationWorkerModel"] != wantID {
		t.Fatalf("mission model settings = %#v", mission)
	}
	if mission["skipScrutiny"] != true || mission["skipUserTesting"] != true {
		t.Fatalf("mission validation toggles = %#v", mission)
	}

	compaction := settings["compactionTokenLimitPerModel"].(map[string]int)
	if compaction[wantID] != 900000 {
		t.Fatalf("compaction for selected id = %d", compaction[wantID])
	}
	if compaction["codex:gpt-5.5:xhigh"] != 900000 {
		t.Fatalf("compaction for underlying model = %d", compaction["codex:gpt-5.5:xhigh"])
	}

	custom, ok := settings["customModels"].([]map[string]any)
	if !ok {
		t.Fatalf("customModels type = %T", settings["customModels"])
	}
	if len(custom) != 8 {
		t.Fatalf("custom model count = %d, want 8", len(custom))
	}
	if custom[0]["id"] != "custom:other" {
		t.Fatalf("non-cagent custom model was not preserved first: %#v", custom[0])
	}
	if _, ok := custom[0]["extraHeaders"]; !ok {
		t.Fatalf("non-cagent custom model extra fields were not preserved: %#v", custom[0])
	}
}

func TestCheckSettingsReportsReadyAfterApply(t *testing.T) {
	settings := map[string]any{}
	opts := NormalizeSetupOptions(SetupOptions{
		SetSessionDefault:  true,
		SetMissionDefaults: true,
		SkipScrutiny:       true,
		SkipUserTesting:    true,
	})
	ApplySettings(settings, opts)
	report := CheckSettings(settings, opts)
	if !report.OK {
		t.Fatalf("report not ok: %#v", report.Checks)
	}
}

func TestExecArgsUsesMissionRoleOverrides(t *testing.T) {
	args := ExecArgs(ExecOptions{Mission: true, CWD: "/tmp/project"}, []string{"do work"})
	want := []string{
		"exec",
		"--mission",
		"--auto", "high",
		"--model", "custom:cagent-gpt-5-5-xhigh-128k-max",
		"--reasoning-effort", "xhigh",
		"--worker-model", "custom:cagent-gpt-5-5-xhigh-128k-max",
		"--worker-reasoning-effort", "xhigh",
		"--validator-model", "custom:cagent-gpt-5-5-xhigh-128k-max",
		"--validator-reasoning-effort", "xhigh",
		"--cwd", "/tmp/project",
		"do work",
	}
	if len(args) != len(want) {
		t.Fatalf("args length = %d, want %d: %#v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q\nall args: %#v", i, args[i], want[i], args)
		}
	}
}
