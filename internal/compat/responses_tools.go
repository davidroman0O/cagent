package compat

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

type ResponsesToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ResponsesToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

func (c ResponsesToolCall) ArgumentsString() string {
	args := bytes.TrimSpace(c.Arguments)
	if len(args) == 0 || bytes.Equal(args, []byte("null")) {
		return "{}"
	}
	return string(args)
}

func ResponsesToolSpecs(raw json.RawMessage) []ResponsesToolSpec {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil
	}
	var specs []ResponsesToolSpec
	collectResponsesToolSpecs(value, &specs)
	return dedupeToolSpecs(specs)
}

func ResponsesToolNames(raw json.RawMessage) []string {
	specs := ResponsesToolSpecs(raw)
	names := make([]string, 0, len(specs))
	seen := make(map[string]bool, len(specs))
	for _, spec := range specs {
		if spec.Name == "" || seen[spec.Name] {
			continue
		}
		seen[spec.Name] = true
		names = append(names, spec.Name)
	}
	return names
}

func AutoResponsesToolCall(req ResponsesRequest, allowedNames []string) (*ResponsesToolCall, bool) {
	allowed := allowedToolNames(allowedNames)
	missionToolName, hasMissionTool := findAllowedToolName(allowed, "StartMissionRun")
	if !hasMissionTool {
		return nil, false
	}
	inputText := rawInputText(req.Input)
	allText := strings.Join([]string{req.Instructions, inputText}, "\n")
	toolChoice := ResponsesToolChoiceName(req.ToolChoice)
	if toolChoice != "" {
		if !sameToolName(toolChoice, missionToolName) {
			return nil, false
		}
	} else if !missionResumeRequested(inputText) {
		return nil, false
	}

	args := map[string]any{}
	if id := resumeWorkerSessionID(allText); id != "" {
		args["resumeWorkerSessionId"] = id
	}
	if restartFeatureRequested(inputText) {
		args["restartFeature"] = true
	}
	rawArgs, _ := json.Marshal(args)
	return &ResponsesToolCall{Name: missionToolName, Arguments: rawArgs}, true
}

func ResponsesToolChoiceName(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	return toolChoiceNameFromAny(value)
}

func ParseResponsesToolCall(text string, allowedNames []string) (*ResponsesToolCall, bool) {
	allowed := allowedToolNames(allowedNames)
	for _, candidate := range jsonCandidates(text) {
		var value any
		decoder := json.NewDecoder(strings.NewReader(candidate))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			continue
		}
		if call, ok := responseToolCallFromAny(value, allowed); ok {
			return call, true
		}
	}
	if call, ok := parseToolCallSyntax(text, allowed); ok {
		return call, true
	}
	if call, ok := parseToolLabelCall(text, allowed); ok {
		return call, true
	}
	return nil, false
}

func responsesToolBridgePrompt(req ResponsesRequest) string {
	specs := ResponsesToolSpecs(req.Tools)
	if len(specs) == 0 {
		return ""
	}
	exampleToolName := exampleToolNameFromSpecs(specs)
	var b strings.Builder
	b.WriteString("[CLIENT TOOLS]\n")
	b.WriteString("The upstream OpenAI Responses client supplied tools that are executed by the client, not by Codex.\n")
	b.WriteString("When the correct next action is to call one of these tools, respond with exactly one JSON object and no markdown or surrounding text:\n")
	example, _ := json.Marshal(map[string]any{
		"cagent_tool_call": map[string]any{
			"name":      exampleToolName,
			"arguments": map[string]any{},
		},
	})
	b.Write(example)
	b.WriteString("\nUse the exact tool name from Available client tools in the JSON name field, and JSON arguments that match its schema.\n")
	b.WriteString("If the instruction uses a snake_case or kebab-case alias, call the matching available client tool. Include resumeWorkerSessionId when it is provided for StartMissionRun.\n")
	if aliases := droidMissionToolAliases(specs); len(aliases) > 0 {
		b.WriteString("Known Droid mission tool aliases available in this request:\n")
		for _, alias := range aliases {
			b.WriteString("- ")
			b.WriteString(alias)
			b.WriteString("\n")
		}
	}
	if hasDroidMissionTools(specs) {
		b.WriteString("Droid mission workflow notes:\n")
		b.WriteString("- For a new mission, call ProposeMission first. It creates the proposal/mission markdown only; it does not create runner artifacts.\n")
		b.WriteString("- After ProposeMission is accepted, create the required mission files before StartMissionRun: features.json, validation-contract.md, validation-state.json, AGENTS.md, services.yaml, and skills/<skillName>/SKILL.md.\n")
		b.WriteString("- Only call StartMissionRun after those files exist. Do not treat a resumeWorkerSessionId shown in a failed StartMissionRun result as a user resume request.\n")
	}
	if hasDroidWorkerTool(specs) {
		b.WriteString("Droid worker workflow notes:\n")
		b.WriteString("- When a worker feature is complete or blocked, call EndFeatureRun. A normal assistant summary does not finish the feature.\n")
		b.WriteString("- For successful work, call EndFeatureRun with successState=\"success\", returnToOrchestrator=false unless orchestrator attention is needed, validatorsPassed=true, and a structured handoff covering implementation, verification commands, tests, and discovered issues.\n")
		b.WriteString("- For blocked work, call EndFeatureRun with successState=\"failure\" or \"partial\" and returnToOrchestrator=true with the blocking details in the handoff.\n")
	}
	b.WriteString("Available client tools:\n")
	for i, spec := range specs {
		if i >= 64 {
			b.WriteString("- ...additional tools omitted\n")
			break
		}
		b.WriteString("- ")
		b.WriteString(spec.Name)
		if spec.Description != "" {
			b.WriteString(": ")
			b.WriteString(oneLine(spec.Description, 500))
		}
		b.WriteString("\n")
		if len(spec.Parameters) > 0 {
			b.WriteString("  parameters: ")
			b.WriteString(compactRawJSON(spec.Parameters, 2000))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func parseToolCallSyntax(text string, allowed map[string]string) (*ResponsesToolCall, bool) {
	for i := 0; i < len(text); i++ {
		if !isToolNameStart(text[i]) {
			continue
		}
		nameStart := i
		i++
		for i < len(text) && isToolNameChar(text[i]) {
			i++
		}
		name := text[nameStart:i]
		actualName, ok := findAllowedToolName(allowed, name)
		if !ok {
			i--
			continue
		}
		j := i
		for j < len(text) && isSpaceByte(text[j]) {
			j++
		}
		if j >= len(text) || text[j] != '(' {
			i--
			continue
		}
		end := closingParenIndex(text, j)
		if end < 0 {
			i--
			continue
		}
		argsText := strings.TrimSpace(text[j+1 : end])
		args := json.RawMessage(`{}`)
		if argsText != "" {
			if !json.Valid([]byte(argsText)) {
				i = end
				continue
			}
			args = json.RawMessage(argsText)
		}
		return &ResponsesToolCall{Name: actualName, Arguments: args}, true
	}
	return nil, false
}

func parseToolLabelCall(text string, allowed map[string]string) (*ResponsesToolCall, bool) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		key, value, ok := splitLabel(line)
		if !ok {
			continue
		}
		normalizedKey := canonicalToolName(key)
		if normalizedKey != "tool" && normalizedKey != "toolname" && normalizedKey != "name" {
			continue
		}
		actualName, ok := findAllowedToolName(allowed, value)
		if !ok {
			continue
		}
		args := json.RawMessage(`{}`)
		for j := i + 1; j < len(lines); j++ {
			candidateLine := lines[j]
			argKey, argValue, hasLabel := splitLabel(candidateLine)
			if hasLabel && (canonicalToolName(argKey) == "arguments" || canonicalToolName(argKey) == "args" || canonicalToolName(argKey) == "input") {
				argValue = strings.TrimSpace(argValue)
				if argValue == "" {
					continue
				}
				if raw, ok := jsonFromLineBlock(lines, j, argValue); ok {
					args = raw
					return &ResponsesToolCall{Name: actualName, Arguments: args}, true
				}
			}
			trimmed := strings.TrimSpace(candidateLine)
			if strings.HasPrefix(trimmed, "{") {
				if raw, ok := jsonFromLineBlock(lines, j, trimmed); ok {
					args = raw
					return &ResponsesToolCall{Name: actualName, Arguments: args}, true
				}
			}
		}
		return &ResponsesToolCall{Name: actualName, Arguments: args}, true
	}
	return nil, false
}

func jsonFromLineBlock(lines []string, start int, first string) (json.RawMessage, bool) {
	var b strings.Builder
	if first != "" {
		b.WriteString(strings.TrimSpace(first))
	}
	if json.Valid([]byte(b.String())) {
		return json.RawMessage(b.String()), true
	}
	for i := start + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		candidate := b.String()
		if json.Valid([]byte(candidate)) {
			return json.RawMessage(candidate), true
		}
		if !strings.ContainsAny(line, "{}[],:\"") {
			break
		}
	}
	return nil, false
}

func collectResponsesToolSpecs(value any, specs *[]ResponsesToolSpec) {
	switch typed := value.(type) {
	case []any:
		for _, elem := range typed {
			collectResponsesToolSpecs(elem, specs)
		}
	case map[string]any:
		if tools, ok := typed["tools"]; ok {
			collectResponsesToolSpecs(tools, specs)
		}
		if spec := responseToolSpecFromMap(typed); spec.Name != "" {
			*specs = append(*specs, spec)
		}
	}
}

func responseToolSpecFromMap(data map[string]any) ResponsesToolSpec {
	target := data
	if fn := mapFromAny(data["function"]); fn != nil {
		target = fn
	}
	name := firstStringFromMaps(target, data, "name")
	if name == "" {
		return ResponsesToolSpec{}
	}
	description := firstStringFromMaps(target, data, "description")
	parameters := rawJSONFromFirst(target, data, "parameters", "input_schema", "schema")
	return ResponsesToolSpec{Name: name, Description: description, Parameters: parameters}
}

func responseToolCallFromAny(value any, allowed map[string]string) (*ResponsesToolCall, bool) {
	switch typed := value.(type) {
	case []any:
		for _, elem := range typed {
			if call, ok := responseToolCallFromAny(elem, allowed); ok {
				return call, true
			}
		}
	case map[string]any:
		for _, key := range []string{"cagent_tool_call", "tool_call", "function_call"} {
			if nested, ok := typed[key]; ok {
				if call, ok := responseToolCallFromAny(nested, allowed); ok {
					return call, true
				}
			}
		}
		name := stringFromMap(typed, "name")
		if name == "" {
			name = stringFromMap(typed, "tool_name")
		}
		if name == "" {
			name = stringFromMap(typed, "tool")
		}
		if name == "" {
			if fn := mapFromAny(typed["function"]); fn != nil {
				name = stringFromMap(fn, "name")
			}
		}
		actualName, ok := findAllowedToolName(allowed, name)
		if name == "" || !ok {
			return nil, false
		}
		args := normalizeArguments(typed["arguments"])
		if len(args) == 0 {
			args = normalizeArguments(typed["args"])
		}
		if len(args) == 0 {
			args = normalizeArguments(typed["input"])
		}
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		return &ResponsesToolCall{Name: actualName, Arguments: args}, true
	}
	return nil, false
}

func toolChoiceNameFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		if typed == "auto" || typed == "none" || typed == "required" {
			return ""
		}
		return typed
	case map[string]any:
		if name := stringFromMap(typed, "name"); name != "" {
			return name
		}
		if fn := mapFromAny(typed["function"]); fn != nil {
			if name := stringFromMap(fn, "name"); name != "" {
				return name
			}
		}
		if choice := mapFromAny(typed["tool"]); choice != nil {
			if name := stringFromMap(choice, "name"); name != "" {
				return name
			}
		}
	}
	return ""
}

func normalizeArguments(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return json.RawMessage(`{}`)
		}
		if json.Valid([]byte(trimmed)) {
			return json.RawMessage(trimmed)
		}
		raw, _ := json.Marshal(map[string]any{"value": typed})
		return raw
	default:
		raw, _ := json.Marshal(typed)
		return raw
	}
}

func jsonCandidates(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	candidates := []string{stripJSONFence(trimmed)}
	seen := map[string]bool{candidates[0]: true}
	for _, candidate := range balancedJSONObjectSubstrings(trimmed) {
		if !seen[candidate] {
			seen[candidate] = true
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func stripJSONFence(text string) string {
	if !strings.HasPrefix(text, "```") {
		return text
	}
	firstLine := strings.IndexByte(text, '\n')
	if firstLine < 0 {
		return text
	}
	body := strings.TrimSpace(text[firstLine+1:])
	if lastFence := strings.LastIndex(body, "```"); lastFence >= 0 {
		body = strings.TrimSpace(body[:lastFence])
	}
	return body
}

func balancedJSONObjectSubstrings(text string) []string {
	var out []string
	start := -1
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					out = append(out, text[start:i+1])
					start = -1
				}
			}
		}
	}
	return out
}

func closingParenIndex(text string, open int) int {
	depth := 0
	inString := false
	escaped := false
	for i := open; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitLabel(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", false
	}
	if strings.HasPrefix(line, "-") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
	}
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])
	if key == "" || value == "" {
		return "", "", false
	}
	return key, value, true
}

func isToolNameStart(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_'
}

func isToolNameChar(ch byte) bool {
	return isToolNameStart(ch) || (ch >= '0' && ch <= '9') || ch == '-'
}

func isSpaceByte(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
}

func allowedToolNames(names []string) map[string]string {
	allowed := make(map[string]string, len(names)*2)
	for _, name := range names {
		if name != "" {
			allowed[name] = name
			if canonical := canonicalToolName(name); canonical != "" {
				allowed[canonical] = name
			}
		}
	}
	return allowed
}

func findAllowedToolName(allowed map[string]string, name string) (string, bool) {
	name = strings.Trim(strings.TrimSpace(name), "`\"'")
	if name == "" {
		return "", false
	}
	if actual, ok := allowed[name]; ok {
		return actual, true
	}
	actual, ok := allowed[canonicalToolName(name)]
	return actual, ok
}

func sameToolName(left, right string) bool {
	return left != "" && right != "" && canonicalToolName(left) == canonicalToolName(right)
}

func canonicalToolName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "-", "")
	return name
}

func exampleToolNameFromSpecs(specs []ResponsesToolSpec) string {
	for _, spec := range specs {
		if sameToolName(spec.Name, "StartMissionRun") {
			return spec.Name
		}
	}
	if len(specs) > 0 && specs[0].Name != "" {
		return specs[0].Name
	}
	return "StartMissionRun"
}

func droidMissionToolAliases(specs []ResponsesToolSpec) []string {
	known := []struct {
		name    string
		aliases []string
	}{
		{name: "ProposeMission", aliases: []string{"propose_mission", "propose-mission"}},
		{name: "StartMissionRun", aliases: []string{"start_mission_run", "start-mission-run"}},
		{name: "DismissHandoffItems", aliases: []string{"dismiss_handoff_items", "dismiss-handoff-items"}},
		{name: "EndFeatureRun", aliases: []string{"end_feature_run", "end-feature-run"}},
	}
	var out []string
	for _, spec := range specs {
		for _, item := range known {
			if !sameToolName(spec.Name, item.name) {
				continue
			}
			out = append(out, spec.Name+" accepts aliases "+strings.Join(item.aliases, ", "))
			break
		}
	}
	return out
}

func hasDroidMissionTools(specs []ResponsesToolSpec) bool {
	hasPropose := false
	hasStart := false
	for _, spec := range specs {
		hasPropose = hasPropose || sameToolName(spec.Name, "ProposeMission")
		hasStart = hasStart || sameToolName(spec.Name, "StartMissionRun")
	}
	return hasPropose && hasStart
}

func hasDroidWorkerTool(specs []ResponsesToolSpec) bool {
	for _, spec := range specs {
		if sameToolName(spec.Name, "EndFeatureRun") {
			return true
		}
	}
	return false
}

func dedupeToolSpecs(specs []ResponsesToolSpec) []ResponsesToolSpec {
	out := make([]ResponsesToolSpec, 0, len(specs))
	seen := make(map[string]bool, len(specs))
	for _, spec := range specs {
		if spec.Name == "" || seen[spec.Name] {
			continue
		}
		seen[spec.Name] = true
		out = append(out, spec)
	}
	return out
}

func rawJSONFromFirst(primary, fallback map[string]any, keys ...string) json.RawMessage {
	for _, data := range []map[string]any{primary, fallback} {
		for _, key := range keys {
			if value, ok := data[key]; ok && value != nil {
				raw, _ := json.Marshal(value)
				return raw
			}
		}
	}
	return nil
}

func firstStringFromMaps(primary, fallback map[string]any, key string) string {
	if value := stringFromMap(primary, key); value != "" {
		return value
	}
	return stringFromMap(fallback, key)
}

func compactRawJSON(raw json.RawMessage, limit int) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return oneLine(string(raw), limit)
	}
	return oneLine(buf.String(), limit)
}

func oneLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if limit > 0 && len(text) > limit {
		return text[:limit] + "..."
	}
	return text
}

var (
	resumeWorkerIDRE      = regexp.MustCompile(`(?i)resumeWorkerSessionId["'\s:=]+([A-Za-z0-9._:-]{6,})`)
	quotedWorkerSessionRE = regexp.MustCompile(`(?i)worker session ["']([A-Za-z0-9._:-]{6,})["']`)
	uuidLikeSessionIDRE   = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	restartFeatureFlagRE  = regexp.MustCompile(`(?i)restartFeature["'\s:=]+true`)
)

func resumeWorkerSessionID(text string) string {
	for _, re := range []*regexp.Regexp{resumeWorkerIDRE, quotedWorkerSessionRE} {
		match := re.FindStringSubmatch(text)
		if len(match) > 1 {
			return strings.Trim(match[1], `"'.,;`)
		}
	}
	return uuidLikeSessionIDRE.FindString(text)
}

func missionResumeRequested(inputText string) bool {
	lower := strings.ToLower(inputText)
	for _, phrase := range []string{
		"[resume the mission]",
		"resume the mission",
		"resume mission",
		"resume this mission",
		"continue the mission",
		"continue mission",
		"continue this mission",
		"resume work",
		"continue work",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func restartFeatureRequested(inputText string) bool {
	lower := strings.ToLower(inputText)
	return restartFeatureFlagRE.MatchString(inputText) ||
		strings.Contains(lower, "restart the feature") ||
		strings.Contains(lower, "start fresh")
}
