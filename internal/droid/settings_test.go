package droid

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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

func TestWriteRuntimeSettingsFileContainsCagentDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime-settings.json")
	written, err := WriteRuntimeSettingsFile(path, SetupOptions{
		BaseURL:         "http://localhost:8080/v1",
		APIToken:        "local-cagent-token",
		SkipScrutiny:    true,
		SkipUserTesting: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if written != path {
		t.Fatalf("written path = %q, want %q", written, path)
	}
	settings, err := readJSONMap(path)
	if err != nil {
		t.Fatal(err)
	}
	session := settings["sessionDefaultSettings"].(map[string]any)
	if session["model"] != "custom:cagent-gpt-5-5-xhigh-128k-max" || session["reasoningEffort"] != "xhigh" {
		t.Fatalf("session defaults = %#v", session)
	}
	mission := settings["missionModelSettings"].(map[string]any)
	if mission["workerModel"] != "custom:cagent-gpt-5-5-xhigh-128k-max" || mission["workerReasoningEffort"] != "xhigh" {
		t.Fatalf("mission defaults = %#v", mission)
	}
	if intValue(settings["compactionTokenLimit"]) != 900000 {
		t.Fatalf("compactionTokenLimit = %#v, want 900000", settings["compactionTokenLimit"])
	}
}

func TestRepairMissionsUpdatesActiveMissionSnapshots(t *testing.T) {
	missionsDir := t.TempDir()
	missionDir := filepath.Join(missionsDir, "mission-1")
	if err := os.MkdirAll(missionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, filepath.Join(missionDir, "state.json"), map[string]any{
		"missionId": "mis_test",
		"state":     "paused",
	})
	writeTestJSON(t, filepath.Join(missionDir, "model-settings.json"), map[string]any{
		"workerModel":                     "custom:cagent-GPT-5.5-XHigh-128K-Max-5",
		"workerReasoningEffort":           "none",
		"validationWorkerModel":           "custom:cagent-GPT-5.5-XHigh-128K-Max-5",
		"validationWorkerReasoningEffort": "none",
	})
	writeTestJSON(t, filepath.Join(missionDir, "runtime-custom-models.json"), map[string]any{
		"customModels": []any{
			map[string]any{
				"id":              "custom:cagent-GPT-5.5-XHigh-128K-Max-5",
				"model":           "codex:gpt-5.5:xhigh",
				"displayName":     "cagent GPT-5.5 XHigh 128K Max",
				"baseUrl":         "http://localhost:8080/v1",
				"apiKey":          "old",
				"provider":        "openai",
				"maxContextLimit": 1000000,
				"maxOutputTokens": 128000,
			},
			map[string]any{
				"id":          "custom:other",
				"model":       "other-model",
				"displayName": "other",
				"baseUrl":     "https://example.com/v1",
				"apiKey":      "secret",
				"provider":    "openai",
			},
		},
	})

	result, err := RepairMissions(MissionRepairOptions{
		Setup: NormalizeSetupOptions(SetupOptions{
			BaseURL:         "http://localhost:8080/v1",
			APIToken:        "local-cagent-token",
			SkipScrutiny:    true,
			SkipUserTesting: true,
		}),
		MissionsDir: missionsDir,
		Backup:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Repaired) != 1 {
		t.Fatalf("repaired = %d, want 1: %#v", len(result.Repaired), result)
	}
	if len(result.Repaired[0].BackupPaths) != 2 {
		t.Fatalf("backup paths = %#v, want 2 backups", result.Repaired[0].BackupPaths)
	}
	for _, backupPath := range result.Repaired[0].BackupPaths {
		if _, err := os.Stat(backupPath); err != nil {
			t.Fatalf("backup missing %s: %v", backupPath, err)
		}
	}

	modelSettings, err := readJSONMap(filepath.Join(missionDir, "model-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	wantID := "custom:cagent-gpt-5-5-xhigh-128k-max"
	if modelSettings["workerModel"] != wantID || modelSettings["validationWorkerModel"] != wantID {
		t.Fatalf("mission models = %#v", modelSettings)
	}
	if modelSettings["workerReasoningEffort"] != "xhigh" || modelSettings["validationWorkerReasoningEffort"] != "xhigh" {
		t.Fatalf("mission reasoning = %#v", modelSettings)
	}
	if modelSettings["skipScrutiny"] != true || modelSettings["skipUserTesting"] != true {
		t.Fatalf("mission skip flags = %#v", modelSettings)
	}

	runtimeModels, err := readJSONMap(filepath.Join(missionDir, "runtime-custom-models.json"))
	if err != nil {
		t.Fatal(err)
	}
	customObjects, ok := existingCustomModelObjects(runtimeModels["customModels"])
	if !ok {
		t.Fatalf("runtime customModels type = %T", runtimeModels["customModels"])
	}
	ids := map[string]bool{}
	for _, obj := range customObjects {
		ids[stringValue(obj["id"])] = true
	}
	if !ids[wantID] {
		t.Fatalf("stable cagent model missing from runtime ids: %#v", ids)
	}
	if ids["custom:cagent-GPT-5.5-XHigh-128K-Max-5"] {
		t.Fatalf("stale cagent model was preserved: %#v", ids)
	}
	if !ids["custom:other"] {
		t.Fatalf("non-cagent custom model was not preserved: %#v", ids)
	}
}

func TestRepairMissionsSkipsTerminalMissionsByDefault(t *testing.T) {
	missionsDir := t.TempDir()
	missionDir := filepath.Join(missionsDir, "mission-1")
	if err := os.MkdirAll(missionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, filepath.Join(missionDir, "state.json"), map[string]any{
		"missionId": "mis_done",
		"state":     "completed",
	})
	writeTestJSON(t, filepath.Join(missionDir, "model-settings.json"), map[string]any{
		"workerModel": "custom:cagent-GPT-5.5-XHigh-128K-Max-5",
	})

	result, err := RepairMissions(MissionRepairOptions{
		Setup:       NormalizeSetupOptions(SetupOptions{SkipScrutiny: true, SkipUserTesting: true}),
		MissionsDir: missionsDir,
		Backup:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Repaired) != 0 || len(result.Skipped) != 1 {
		t.Fatalf("result = %#v, want 0 repaired and 1 skipped", result)
	}
	modelSettings, err := readJSONMap(filepath.Join(missionDir, "model-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if modelSettings["workerModel"] != "custom:cagent-GPT-5.5-XHigh-128K-Max-5" {
		t.Fatalf("terminal mission was modified: %#v", modelSettings)
	}
}

func TestRepairMissionsCanTargetSingleMission(t *testing.T) {
	missionsDir := t.TempDir()
	selectedDir := filepath.Join(missionsDir, "selected")
	otherDir := filepath.Join(missionsDir, "other")
	writeStaleMission(t, selectedDir, "mis_selected", "paused")
	writeStaleMission(t, otherDir, "mis_other", "paused")

	result, err := RepairMissions(MissionRepairOptions{
		Setup:       NormalizeSetupOptions(SetupOptions{SkipScrutiny: true, SkipUserTesting: true}),
		MissionsDir: missionsDir,
		Missions:    []string{"mis_selected"},
		Backup:      false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Repaired) != 1 {
		t.Fatalf("repaired = %d, want 1: %#v", len(result.Repaired), result)
	}
	if result.Repaired[0].ID != "mis_selected" {
		t.Fatalf("repaired mission = %q, want mis_selected", result.Repaired[0].ID)
	}

	selectedSettings, err := readJSONMap(filepath.Join(selectedDir, "model-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if selectedSettings["workerModel"] != "custom:cagent-gpt-5-5-xhigh-128k-max" {
		t.Fatalf("selected mission was not repaired: %#v", selectedSettings)
	}
	otherSettings, err := readJSONMap(filepath.Join(otherDir, "model-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if otherSettings["workerModel"] != "custom:cagent-GPT-5.5-XHigh-128K-Max-5" {
		t.Fatalf("unselected mission was modified: %#v", otherSettings)
	}
}

func TestListMissionsReportsRepairStatusAndFilters(t *testing.T) {
	missionsDir := t.TempDir()
	staleDir := filepath.Join(missionsDir, "stale")
	goodDir := filepath.Join(missionsDir, "good")
	doneDir := filepath.Join(missionsDir, "done")
	writeStaleMission(t, staleDir, "mis_stale", "paused")
	writeStaleMission(t, goodDir, "mis_good", "paused")
	writeStaleMission(t, doneDir, "mis_done", "completed")
	writeTestJSON(t, filepath.Join(staleDir, "state.json"), map[string]any{
		"missionId":        "mis_stale",
		"state":            "paused",
		"workingDirectory": "/tmp/stale",
		"updatedAt":        "2026-04-28T20:00:00.000Z",
	})
	writeTestJSON(t, filepath.Join(goodDir, "state.json"), map[string]any{
		"missionId":        "mis_good",
		"state":            "paused",
		"workingDirectory": "/tmp/good",
		"updatedAt":        "2026-04-28T21:00:00.000Z",
	})
	_, err := RepairMissions(MissionRepairOptions{
		Setup:       NormalizeSetupOptions(SetupOptions{SkipScrutiny: true, SkipUserTesting: true}),
		MissionsDir: missionsDir,
		Missions:    []string{"mis_good"},
		Backup:      false,
	})
	if err != nil {
		t.Fatal(err)
	}

	missions, err := ListMissions(MissionListOptions{
		Setup:           NormalizeSetupOptions(SetupOptions{SkipScrutiny: true, SkipUserTesting: true}),
		MissionsDir:     missionsDir,
		NeedsRepairOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missions) != 1 {
		t.Fatalf("repair-needed missions = %d, want 1: %#v", len(missions), missions)
	}
	if missions[0].ID != "mis_stale" || !missions[0].NeedsRepair {
		t.Fatalf("repair-needed mission = %#v, want mis_stale needing repair", missions[0])
	}
	if missions[0].WorkingDirectory != "/tmp/stale" {
		t.Fatalf("working directory = %q, want /tmp/stale", missions[0].WorkingDirectory)
	}

	active, err := ListMissions(MissionListOptions{
		Setup:       NormalizeSetupOptions(SetupOptions{SkipScrutiny: true, SkipUserTesting: true}),
		MissionsDir: missionsDir,
		ActiveOnly:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 {
		t.Fatalf("active missions = %d, want 2: %#v", len(active), active)
	}
	if active[0].ID != "mis_good" {
		t.Fatalf("missions not sorted by updatedAt desc: %#v", active)
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

func writeStaleMission(t *testing.T, dir string, missionID string, state string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, filepath.Join(dir, "state.json"), map[string]any{
		"missionId": missionID,
		"state":     state,
	})
	writeTestJSON(t, filepath.Join(dir, "model-settings.json"), map[string]any{
		"workerModel":                     "custom:cagent-GPT-5.5-XHigh-128K-Max-5",
		"workerReasoningEffort":           "none",
		"validationWorkerModel":           "custom:cagent-GPT-5.5-XHigh-128K-Max-5",
		"validationWorkerReasoningEffort": "none",
	})
	writeTestJSON(t, filepath.Join(dir, "runtime-custom-models.json"), map[string]any{
		"customModels": []any{},
	})
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
