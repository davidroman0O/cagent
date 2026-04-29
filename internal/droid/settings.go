package droid

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	DefaultBaseURL              = "http://localhost:8080/v1"
	DefaultAPIToken             = "local-cagent-token"
	DefaultCodexModel           = "gpt-5.5"
	DefaultReasoningEffort      = "xhigh"
	DefaultMaxContextLimit      = 1000000
	DefaultCompactionTokenLimit = 900000
	DefaultSafeMaxOutputTokens  = 64000
	DefaultMaxOutputTokens      = 128000
	RuntimeSettingsPathEnv      = "FACTORY_RUNTIME_SETTINGS_PATH"
)

type SetupOptions struct {
	SettingsPath         string
	BaseURL              string
	APIToken             string
	CodexModel           string
	ReasoningEffort      string
	MaxContextLimit      int
	CompactionTokenLimit int
	SafeMaxOutputTokens  int
	MaxOutputTokens      int
	SetSessionDefault    bool
	SetMissionDefaults   bool
	SkipScrutiny         bool
	SkipUserTesting      bool
	Backup               bool
}

type SetupResult struct {
	SettingsPath       string
	BackupPath         string
	SelectedModelID    string
	CustomModelIDs     []string
	CompactionKeys     []string
	SessionDefaultSet  bool
	MissionDefaultsSet bool
}

type MissionRepairOptions struct {
	Setup       SetupOptions
	MissionsDir string
	Missions    []string
	All         bool
	DryRun      bool
	Backup      bool
}

type MissionListOptions struct {
	Setup           SetupOptions
	MissionsDir     string
	ActiveOnly      bool
	NeedsRepairOnly bool
}

type MissionRepairResult struct {
	MissionsDir string
	Repaired    []MissionRepair
	Skipped     []MissionRepairSkip
}

type MissionRepair struct {
	Dir          string
	ID           string
	State        string
	ChangedFiles []string
	BackupPaths  []string
}

type MissionRepairSkip struct {
	Dir    string
	ID     string
	State  string
	Reason string
}

type MissionInfo struct {
	Dir                             string `json:"dir"`
	DirectoryID                     string `json:"directoryId"`
	ID                              string `json:"id"`
	State                           string `json:"state"`
	WorkingDirectory                string `json:"workingDirectory"`
	CreatedAt                       string `json:"createdAt"`
	UpdatedAt                       string `json:"updatedAt"`
	NeedsRepair                     bool   `json:"needsRepair"`
	WorkerModel                     string `json:"workerModel,omitempty"`
	WorkerReasoningEffort           string `json:"workerReasoningEffort,omitempty"`
	ValidationWorkerModel           string `json:"validationWorkerModel,omitempty"`
	ValidationWorkerReasoningEffort string `json:"validationWorkerReasoningEffort,omitempty"`
	SkipScrutiny                    bool   `json:"skipScrutiny,omitempty"`
	SkipUserTesting                 bool   `json:"skipUserTesting,omitempty"`
}

type Check struct {
	OK      bool
	Message string
}

type Report struct {
	OK     bool
	Checks []Check
}

type CustomModel struct {
	Model           string `json:"model"`
	ID              string `json:"id"`
	DisplayName     string `json:"displayName"`
	BaseURL         string `json:"baseUrl"`
	APIKey          string `json:"apiKey"`
	Provider        string `json:"provider"`
	MaxContextLimit int    `json:"maxContextLimit"`
	MaxOutputTokens int    `json:"maxOutputTokens"`
	NoImageSupport  bool   `json:"noImageSupport,omitempty"`
}

type ModelProfile struct {
	ID              string
	Model           string
	DisplayName     string
	Provider        string
	MaxOutputTokens int
}

func DefaultSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".factory", "settings.json")
	}
	return filepath.Join(home, ".factory", "settings.json")
}

func DefaultMissionsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".factory", "missions")
	}
	return filepath.Join(home, ".factory", "missions")
}

func NormalizeSetupOptions(opts SetupOptions) SetupOptions {
	if opts.SettingsPath == "" {
		opts.SettingsPath = DefaultSettingsPath()
	}
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if opts.APIToken == "" {
		opts.APIToken = os.Getenv("CAGENT_TOKEN")
	}
	if opts.APIToken == "" {
		opts.APIToken = DefaultAPIToken
	}
	if opts.CodexModel == "" {
		opts.CodexModel = DefaultCodexModel
	}
	if opts.ReasoningEffort == "" {
		opts.ReasoningEffort = DefaultReasoningEffort
	}
	if opts.MaxContextLimit <= 0 {
		opts.MaxContextLimit = DefaultMaxContextLimit
	}
	if opts.CompactionTokenLimit <= 0 {
		opts.CompactionTokenLimit = DefaultCompactionTokenLimit
	}
	if opts.SafeMaxOutputTokens <= 0 {
		opts.SafeMaxOutputTokens = DefaultSafeMaxOutputTokens
	}
	if opts.MaxOutputTokens <= 0 {
		opts.MaxOutputTokens = DefaultMaxOutputTokens
	}
	return opts
}

func DefaultSelectedModelID(codexModel, reasoningEffort string) string {
	opts := NormalizeSetupOptions(SetupOptions{
		CodexModel:      codexModel,
		ReasoningEffort: reasoningEffort,
	})
	return modelID(opts.CodexModel, opts.ReasoningEffort, opts.MaxOutputTokens, "max")
}

func ApplySettingsFile(opts SetupOptions) (SetupResult, error) {
	opts = NormalizeSetupOptions(opts)
	settings, err := readSettings(opts.SettingsPath)
	if err != nil {
		return SetupResult{}, err
	}
	var backupPath string
	if opts.Backup {
		backupPath, err = backupFile(opts.SettingsPath)
		if err != nil {
			return SetupResult{}, err
		}
	}
	result := ApplySettings(settings, opts)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return SetupResult{}, err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(opts.SettingsPath), 0o755); err != nil {
		return SetupResult{}, err
	}
	if err := os.WriteFile(opts.SettingsPath, data, 0o600); err != nil {
		return SetupResult{}, err
	}
	result.SettingsPath = opts.SettingsPath
	result.BackupPath = backupPath
	return result, nil
}

func WriteRuntimeSettingsFile(path string, opts SetupOptions) (string, error) {
	opts = NormalizeSetupOptions(opts)
	opts.SetSessionDefault = true
	opts.SetMissionDefaults = true
	settings := map[string]any{}
	ApplySettings(settings, opts)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if path == "" {
		if err := os.MkdirAll(filepath.Join(os.TempDir(), "cagent"), 0o755); err != nil {
			return "", err
		}
		file, err := os.CreateTemp(filepath.Join(os.TempDir(), "cagent"), "droid-settings-*.json")
		if err != nil {
			return "", err
		}
		path = file.Name()
		if _, err := file.Write(data); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func CheckSettingsFile(opts SetupOptions) (Report, error) {
	opts = NormalizeSetupOptions(opts)
	settings, err := readSettings(opts.SettingsPath)
	if err != nil {
		return Report{}, err
	}
	return CheckSettings(settings, opts), nil
}

func ListMissions(opts MissionListOptions) ([]MissionInfo, error) {
	opts.Setup = NormalizeSetupOptions(opts.Setup)
	if opts.MissionsDir == "" {
		opts.MissionsDir = DefaultMissionsDir()
	}
	entries, err := os.ReadDir(opts.MissionsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	repairScan, err := RepairMissions(MissionRepairOptions{
		Setup:       opts.Setup,
		MissionsDir: opts.MissionsDir,
		DryRun:      true,
		Backup:      false,
	})
	if err != nil {
		return nil, err
	}
	needsRepair := map[string]bool{}
	for _, repair := range repairScan.Repaired {
		needsRepair[repair.Dir] = true
	}

	missions := make([]MissionInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(opts.MissionsDir, entry.Name())
		stateMap, err := readJSONMap(filepath.Join(dir, "state.json"))
		if err != nil {
			return nil, err
		}
		state := stringValue(stateMap["state"])
		if opts.ActiveOnly && !shouldRepairMissionState(state, false) {
			continue
		}
		if opts.NeedsRepairOnly && !needsRepair[dir] {
			continue
		}
		modelSettings, err := readJSONMap(filepath.Join(dir, "model-settings.json"))
		if err != nil {
			return nil, err
		}
		info := MissionInfo{
			Dir:                             dir,
			DirectoryID:                     entry.Name(),
			ID:                              firstString(stateMap["missionId"], entry.Name()),
			State:                           state,
			WorkingDirectory:                stringValue(stateMap["workingDirectory"]),
			CreatedAt:                       stringValue(stateMap["createdAt"]),
			UpdatedAt:                       stringValue(stateMap["updatedAt"]),
			NeedsRepair:                     needsRepair[dir],
			WorkerModel:                     stringValue(modelSettings["workerModel"]),
			WorkerReasoningEffort:           stringValue(modelSettings["workerReasoningEffort"]),
			ValidationWorkerModel:           stringValue(modelSettings["validationWorkerModel"]),
			ValidationWorkerReasoningEffort: stringValue(modelSettings["validationWorkerReasoningEffort"]),
			SkipScrutiny:                    boolValue(modelSettings["skipScrutiny"]),
			SkipUserTesting:                 boolValue(modelSettings["skipUserTesting"]),
		}
		missions = append(missions, info)
	}
	sort.SliceStable(missions, func(i, j int) bool {
		if missions[i].UpdatedAt != missions[j].UpdatedAt {
			return missions[i].UpdatedAt > missions[j].UpdatedAt
		}
		return missions[i].DirectoryID < missions[j].DirectoryID
	})
	return missions, nil
}

func RepairMissions(opts MissionRepairOptions) (MissionRepairResult, error) {
	opts.Setup = NormalizeSetupOptions(opts.Setup)
	if opts.MissionsDir == "" {
		opts.MissionsDir = DefaultMissionsDir()
	}
	result := MissionRepairResult{MissionsDir: opts.MissionsDir}
	entries, err := os.ReadDir(opts.MissionsDir)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}

	targets := missionTargetSet(opts.Missions)
	selectedID := DefaultSelectedModelID(opts.Setup.CodexModel, opts.Setup.ReasoningEffort)
	profiles := Profiles(opts.Setup)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(opts.MissionsDir, entry.Name())
		stateMap, err := readJSONMap(filepath.Join(dir, "state.json"))
		if err != nil {
			return result, err
		}
		missionID := firstString(stateMap["missionId"], entry.Name())
		state := stringValue(stateMap["state"])
		if len(targets) > 0 && !missionTargetMatches(targets, entry.Name(), missionID, dir) {
			result.Skipped = append(result.Skipped, MissionRepairSkip{
				Dir:    dir,
				ID:     missionID,
				State:  state,
				Reason: "not selected",
			})
			continue
		}
		if !shouldRepairMissionState(state, opts.All) {
			result.Skipped = append(result.Skipped, MissionRepairSkip{
				Dir:    dir,
				ID:     missionID,
				State:  state,
				Reason: "terminal state",
			})
			continue
		}

		repair := MissionRepair{Dir: dir, ID: missionID, State: state}
		modelSettingsPath := filepath.Join(dir, "model-settings.json")
		modelSettings, err := readJSONMap(modelSettingsPath)
		if err != nil {
			return result, err
		}
		changed := false
		changed = setIfDifferent(modelSettings, "workerModel", selectedID) || changed
		changed = setIfDifferent(modelSettings, "workerReasoningEffort", opts.Setup.ReasoningEffort) || changed
		changed = setIfDifferent(modelSettings, "validationWorkerModel", selectedID) || changed
		changed = setIfDifferent(modelSettings, "validationWorkerReasoningEffort", opts.Setup.ReasoningEffort) || changed
		changed = setIfDifferent(modelSettings, "skipScrutiny", opts.Setup.SkipScrutiny) || changed
		changed = setIfDifferent(modelSettings, "skipUserTesting", opts.Setup.SkipUserTesting) || changed
		if changed {
			backupPath, err := writeJSONMap(modelSettingsPath, modelSettings, opts.DryRun, opts.Backup)
			if err != nil {
				return result, err
			}
			repair.ChangedFiles = append(repair.ChangedFiles, modelSettingsPath)
			if backupPath != "" {
				repair.BackupPaths = append(repair.BackupPaths, backupPath)
			}
		}

		runtimeModelsPath := filepath.Join(dir, "runtime-custom-models.json")
		runtimeModels, err := readJSONMap(runtimeModelsPath)
		if err != nil {
			return result, err
		}
		runtimeModels["customModels"] = desiredCustomModels(runtimeModels["customModels"], profiles, opts.Setup)
		if !jsonFileEqual(runtimeModelsPath, runtimeModels) {
			backupPath, err := writeJSONMap(runtimeModelsPath, runtimeModels, opts.DryRun, opts.Backup)
			if err != nil {
				return result, err
			}
			repair.ChangedFiles = append(repair.ChangedFiles, runtimeModelsPath)
			if backupPath != "" {
				repair.BackupPaths = append(repair.BackupPaths, backupPath)
			}
		}

		if len(repair.ChangedFiles) > 0 {
			result.Repaired = append(result.Repaired, repair)
		}
	}
	return result, nil
}

func CheckSettings(settings map[string]any, opts SetupOptions) Report {
	opts = NormalizeSetupOptions(opts)
	selectedID := DefaultSelectedModelID(opts.CodexModel, opts.ReasoningEffort)
	checks := []Check{}
	add := func(ok bool, message string) {
		checks = append(checks, Check{OK: ok, Message: message})
	}

	custom := parseCustomModels(settings["customModels"])
	selected := CustomModel{}
	for _, model := range custom {
		if model.ID == selectedID {
			selected = model
			break
		}
	}
	add(selected.ID == selectedID, "selected custom model exists: "+selectedID)
	if selected.ID == selectedID {
		add(selected.Model == "codex:"+opts.CodexModel+":"+opts.ReasoningEffort, "selected model forwards to Codex "+opts.CodexModel+" "+opts.ReasoningEffort)
		add(selected.BaseURL == opts.BaseURL, "selected model baseUrl is "+opts.BaseURL)
		add(selected.APIKey == opts.APIToken, "selected model apiKey matches expected token")
		add(selected.Provider == "openai", "selected model uses Droid openai provider")
		add(selected.MaxContextLimit == opts.MaxContextLimit, fmt.Sprintf("selected model maxContextLimit is %d", opts.MaxContextLimit))
	}

	compaction := parseIntMap(settings["compactionTokenLimitPerModel"])
	add(intValue(settings["compactionTokenLimit"]) == opts.CompactionTokenLimit, fmt.Sprintf("global compactionTokenLimit is %d", opts.CompactionTokenLimit))
	add(compaction[selectedID] == opts.CompactionTokenLimit, fmt.Sprintf("compactionTokenLimitPerModel[%s] is %d", selectedID, opts.CompactionTokenLimit))
	add(compaction["codex:"+opts.CodexModel+":"+opts.ReasoningEffort] == opts.CompactionTokenLimit, "underlying cagent model compaction limit is set")

	session := objectMap(settings["sessionDefaultSettings"])
	add(stringValue(session["model"]) == selectedID, "session default model is cagent")
	add(stringValue(session["reasoningEffort"]) == opts.ReasoningEffort, "session default reasoning effort is "+opts.ReasoningEffort)

	mission := objectMap(settings["missionModelSettings"])
	add(stringValue(settings["missionOrchestratorModel"]) == selectedID, "mission orchestrator model is cagent")
	add(stringValue(settings["missionOrchestratorReasoningEffort"]) == opts.ReasoningEffort, "mission orchestrator reasoning effort is "+opts.ReasoningEffort)
	add(stringValue(mission["workerModel"]) == selectedID, "mission worker model is cagent")
	add(stringValue(mission["workerReasoningEffort"]) == opts.ReasoningEffort, "mission worker reasoning effort is "+opts.ReasoningEffort)
	add(stringValue(mission["validationWorkerModel"]) == selectedID, "mission validator model is cagent")
	add(stringValue(mission["validationWorkerReasoningEffort"]) == opts.ReasoningEffort, "mission validator reasoning effort is "+opts.ReasoningEffort)
	add(boolValue(mission["skipScrutiny"]) == opts.SkipScrutiny, fmt.Sprintf("mission skipScrutiny is %t", opts.SkipScrutiny))
	add(boolValue(mission["skipUserTesting"]) == opts.SkipUserTesting, fmt.Sprintf("mission skipUserTesting is %t", opts.SkipUserTesting))

	ok := true
	for _, check := range checks {
		if !check.OK {
			ok = false
			break
		}
	}
	return Report{OK: ok, Checks: checks}
}

func ApplySettings(settings map[string]any, opts SetupOptions) SetupResult {
	opts = NormalizeSetupOptions(opts)
	profiles := Profiles(opts)
	selectedID := DefaultSelectedModelID(opts.CodexModel, opts.ReasoningEffort)

	settings["customModels"] = desiredCustomModels(settings["customModels"], profiles, opts)
	settings["compactionTokenLimit"] = opts.CompactionTokenLimit
	settings["compactionTokenLimitPerModel"] = desiredCompaction(settings["compactionTokenLimitPerModel"], profiles, opts)

	sessionDefaultSet := false
	if opts.SetSessionDefault {
		session := objectMap(settings["sessionDefaultSettings"])
		session["interactionMode"] = firstString(session["interactionMode"], "auto")
		session["autonomyLevel"] = firstString(session["autonomyLevel"], "high")
		session["autonomyMode"] = firstString(session["autonomyMode"], "auto-high")
		session["model"] = selectedID
		session["reasoningEffort"] = opts.ReasoningEffort
		settings["sessionDefaultSettings"] = session
		sessionDefaultSet = true
	}

	missionDefaultsSet := false
	if opts.SetMissionDefaults {
		settings["missionOrchestratorModel"] = selectedID
		settings["missionOrchestratorReasoningEffort"] = opts.ReasoningEffort
		mission := objectMap(settings["missionModelSettings"])
		mission["workerModel"] = selectedID
		mission["workerReasoningEffort"] = opts.ReasoningEffort
		mission["validationWorkerModel"] = selectedID
		mission["validationWorkerReasoningEffort"] = opts.ReasoningEffort
		mission["skipScrutiny"] = opts.SkipScrutiny
		mission["skipUserTesting"] = opts.SkipUserTesting
		settings["missionModelSettings"] = mission
		missionDefaultsSet = true
	}

	ids := make([]string, 0, len(profiles))
	keys := make([]string, 0, len(profiles)+3)
	seenKeys := map[string]bool{}
	for _, profile := range profiles {
		ids = append(ids, profile.ID)
		for _, key := range []string{profile.ID, profile.Model} {
			if key != "" && !seenKeys[key] {
				keys = append(keys, key)
				seenKeys[key] = true
			}
		}
	}
	for _, key := range []string{opts.CodexModel, "codex-default"} {
		if key != "" && !seenKeys[key] {
			keys = append(keys, key)
			seenKeys[key] = true
		}
	}
	sort.Strings(keys)
	return SetupResult{
		SelectedModelID:    selectedID,
		CustomModelIDs:     ids,
		CompactionKeys:     keys,
		SessionDefaultSet:  sessionDefaultSet,
		MissionDefaultsSet: missionDefaultsSet,
	}
}

func Profiles(opts SetupOptions) []ModelProfile {
	opts = NormalizeSetupOptions(opts)
	efforts := []string{"medium", "high", "xhigh"}
	profiles := make([]ModelProfile, 0, len(efforts)*2+1)
	for _, effort := range efforts {
		for _, out := range []struct {
			tokens int
			label  string
		}{
			{opts.SafeMaxOutputTokens, "safe"},
			{opts.MaxOutputTokens, "max"},
		} {
			profiles = append(profiles, ModelProfile{
				ID:              modelID(opts.CodexModel, effort, out.tokens, out.label),
				Model:           "codex:" + opts.CodexModel + ":" + effort,
				DisplayName:     fmt.Sprintf("cagent %s %s %s", displayModel(opts.CodexModel), displayEffort(effort), outputLabel(out.tokens, out.label)),
				Provider:        "openai",
				MaxOutputTokens: out.tokens,
			})
		}
	}
	profiles = append(profiles, ModelProfile{
		ID:              "custom:cagent-codex-default-chat-64k-safe",
		Model:           "codex-default",
		DisplayName:     "cagent Codex Default Chat 64K Safe",
		Provider:        "generic-chat-completion-api",
		MaxOutputTokens: opts.SafeMaxOutputTokens,
	})
	return profiles
}

func ExecArgs(opts ExecOptions, promptArgs []string) []string {
	opts = NormalizeExecOptions(opts)
	args := []string{}
	if opts.SettingsPath != "" {
		args = append(args, "--settings", opts.SettingsPath)
	}
	args = append(args, "exec")
	if opts.Mission {
		args = append(args, "--mission")
	}
	if opts.Auto != "" {
		args = append(args, "--auto", opts.Auto)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.ReasoningEffort != "" {
		args = append(args, "--reasoning-effort", opts.ReasoningEffort)
	}
	if opts.Mission {
		if opts.WorkerModel != "" {
			args = append(args, "--worker-model", opts.WorkerModel)
		}
		if opts.WorkerReasoningEffort != "" {
			args = append(args, "--worker-reasoning-effort", opts.WorkerReasoningEffort)
		}
		if opts.ValidatorModel != "" {
			args = append(args, "--validator-model", opts.ValidatorModel)
		}
		if opts.ValidatorReasoningEffort != "" {
			args = append(args, "--validator-reasoning-effort", opts.ValidatorReasoningEffort)
		}
	}
	if opts.CWD != "" {
		args = append(args, "--cwd", opts.CWD)
	}
	if opts.ListTools {
		args = append(args, "--list-tools")
	}
	args = append(args, promptArgs...)
	return args
}

type ExecOptions struct {
	SettingsPath             string
	CWD                      string
	Model                    string
	ReasoningEffort          string
	Mission                  bool
	Auto                     string
	WorkerModel              string
	WorkerReasoningEffort    string
	ValidatorModel           string
	ValidatorReasoningEffort string
	ListTools                bool
}

func NormalizeExecOptions(opts ExecOptions) ExecOptions {
	if opts.Model == "" {
		opts.Model = DefaultSelectedModelID(DefaultCodexModel, DefaultReasoningEffort)
	}
	if opts.ReasoningEffort == "" {
		opts.ReasoningEffort = DefaultReasoningEffort
	}
	if opts.Mission && opts.Auto == "" {
		opts.Auto = "high"
	}
	if opts.Mission {
		if opts.WorkerModel == "" {
			opts.WorkerModel = opts.Model
		}
		if opts.WorkerReasoningEffort == "" {
			opts.WorkerReasoningEffort = opts.ReasoningEffort
		}
		if opts.ValidatorModel == "" {
			opts.ValidatorModel = opts.Model
		}
		if opts.ValidatorReasoningEffort == "" {
			opts.ValidatorReasoningEffort = opts.ReasoningEffort
		}
	}
	return opts
}

func LaunchArgs(settingsPath, cwd string, promptArgs []string) []string {
	args := make([]string, 0, 4+len(promptArgs))
	if settingsPath != "" {
		args = append(args, "--settings", settingsPath)
	}
	if cwd != "" {
		args = append(args, "--cwd", cwd)
	}
	args = append(args, promptArgs...)
	return args
}

func readSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return map[string]any{}, nil
	}
	var settings map[string]any
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

func readJSONMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}

func writeJSONMap(path string, value map[string]any, dryRun bool, backup bool) (string, error) {
	if dryRun {
		return "", nil
	}
	var backupPath string
	var err error
	if backup {
		backupPath, err = backupFile(path)
		if err != nil {
			return "", err
		}
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return backupPath, nil
}

func jsonFileEqual(path string, value map[string]any) bool {
	existing, err := readJSONMap(path)
	if err != nil {
		return false
	}
	existingData, err := json.Marshal(existing)
	if err != nil {
		return false
	}
	nextData, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return bytes.Equal(existingData, nextData)
}

func backupFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	backupPath := fmt.Sprintf("%s.cagent-backup-%s", path, time.Now().Format("20060102150405"))
	out, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := out.Write(data); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return backupPath, nil
}

func shouldRepairMissionState(state string, all bool) bool {
	if all {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "completed", "cancelled", "canceled":
		return false
	default:
		return true
	}
}

func missionTargetSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[value] = true
		out[filepath.Base(value)] = true
	}
	return out
}

func missionTargetMatches(targets map[string]bool, dirName string, missionID string, dir string) bool {
	if targets[dirName] || targets[missionID] || targets[dir] {
		return true
	}
	return targets[filepath.Base(dir)]
}

func setIfDifferent(obj map[string]any, key string, value any) bool {
	if obj[key] == value {
		return false
	}
	obj[key] = value
	return true
}

func desiredCustomModels(existingValue any, profiles []ModelProfile, opts SetupOptions) []map[string]any {
	existing, _ := existingCustomModelObjects(existingValue)
	out := make([]map[string]any, 0, len(existing)+len(profiles))
	for _, item := range existing {
		if isManagedCustomModel(customModelFromMap(item)) {
			continue
		}
		out = append(out, item)
	}
	for _, profile := range profiles {
		out = append(out, map[string]any{
			"model":           profile.Model,
			"id":              profile.ID,
			"displayName":     profile.DisplayName,
			"baseUrl":         opts.BaseURL,
			"apiKey":          opts.APIToken,
			"provider":        profile.Provider,
			"maxContextLimit": opts.MaxContextLimit,
			"maxOutputTokens": profile.MaxOutputTokens,
		})
	}
	return out
}

func desiredCompaction(existingValue any, profiles []ModelProfile, opts SetupOptions) map[string]int {
	out := parseIntMap(existingValue)
	for _, profile := range profiles {
		out[profile.ID] = opts.CompactionTokenLimit
		out[profile.Model] = opts.CompactionTokenLimit
	}
	out[opts.CodexModel] = opts.CompactionTokenLimit
	out["codex-default"] = opts.CompactionTokenLimit
	return out
}

func parseCustomModels(value any) []CustomModel {
	objects, ok := existingCustomModelObjects(value)
	if !ok {
		return nil
	}
	out := make([]CustomModel, 0, len(objects))
	for _, obj := range objects {
		model := customModelFromMap(obj)
		if model.Model == "" || model.BaseURL == "" || model.APIKey == "" || model.Provider == "" {
			continue
		}
		out = append(out, model)
	}
	return out
}

func existingCustomModelObjects(value any) ([]map[string]any, bool) {
	if raw, ok := value.([]map[string]any); ok {
		out := make([]map[string]any, 0, len(raw))
		for _, obj := range raw {
			copyObj := make(map[string]any, len(obj))
			for key, value := range obj {
				copyObj[key] = value
			}
			out = append(out, copyObj)
		}
		return out, true
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		copyObj := make(map[string]any, len(obj))
		for key, value := range obj {
			copyObj[key] = value
		}
		out = append(out, copyObj)
	}
	return out, true
}

func customModelFromMap(obj map[string]any) CustomModel {
	return CustomModel{
		Model:           stringValue(obj["model"]),
		ID:              stringValue(obj["id"]),
		DisplayName:     stringValue(obj["displayName"]),
		BaseURL:         stringValue(obj["baseUrl"]),
		APIKey:          stringValue(obj["apiKey"]),
		Provider:        stringValue(obj["provider"]),
		MaxContextLimit: intValue(obj["maxContextLimit"]),
		MaxOutputTokens: intValue(obj["maxOutputTokens"]),
		NoImageSupport:  boolValue(obj["noImageSupport"]),
	}
}

func parseIntMap(value any) map[string]int {
	out := map[string]int{}
	if obj, ok := value.(map[string]int); ok {
		for key, value := range obj {
			if key != "" && value > 0 {
				out[key] = value
			}
		}
		return out
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return out
	}
	for key, value := range obj {
		if key == "" {
			continue
		}
		if parsed := intValue(value); parsed > 0 {
			out[key] = parsed
		}
	}
	return out
}

func objectMap(value any) map[string]any {
	if obj, ok := value.(map[string]any); ok {
		return obj
	}
	return map[string]any{}
}

func isManagedCustomModel(model CustomModel) bool {
	if strings.HasPrefix(model.ID, "custom:cagent-") {
		return true
	}
	if strings.HasPrefix(model.DisplayName, "cagent ") {
		return true
	}
	if model.Model == "codex-default" || strings.HasPrefix(model.Model, "codex:") {
		return true
	}
	return false
}

func modelID(codexModel, effort string, outputTokens int, label string) string {
	return "custom:cagent-" + slug(codexModel) + "-" + slug(effort) + "-" + slug(outputLabel(outputTokens, label))
}

func outputLabel(tokens int, label string) string {
	if label == "safe" {
		return fmt.Sprintf("%dK Safe", tokens/1000)
	}
	if label == "max" {
		return fmt.Sprintf("%dK Max", tokens/1000)
	}
	return fmt.Sprintf("%dK", tokens/1000)
}

func displayModel(model string) string {
	if strings.HasPrefix(model, "gpt-") {
		return "GPT-" + strings.TrimPrefix(model, "gpt-")
	}
	return model
}

func displayEffort(effort string) string {
	switch strings.ToLower(effort) {
	case "xhigh":
		return "XHigh"
	case "high":
		return "High"
	case "medium":
		return "Medium"
	case "low":
		return "Low"
	default:
		return effort
	}
}

var nonID = regexp.MustCompile(`[^a-z0-9]+`)

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonID.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return value
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func firstString(value any, fallback string) string {
	if s := stringValue(value); s != "" {
		return s
	}
	return fallback
}

func boolValue(value any) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return false
}

func intValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		parsed, _ := v.Int64()
		return int(parsed)
	default:
		return 0
	}
}
