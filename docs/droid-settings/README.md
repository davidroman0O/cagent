# Droid BYOK Settings

Factory Droid can use `cagent` through its BYOK custom model support. `cagent` exposes OpenAI-compatible endpoints under `/v1`, so Droid should point `baseUrl` at the local server.

## Start cagent

```sh
CAGENT_TOKEN=local-cagent-token \
CAGENT_ADDR=:8080 \
CAGENT_CODEX_MODEL_CONTEXT_WINDOW=1000000 \
CAGENT_CODEX_MODEL_AUTO_COMPACT_TOKEN_LIMIT=900000 \
CAGENT_DEFAULT_REASONING_EFFORT=xhigh \
cagent serve
```

When running from source:

```sh
CAGENT_TOKEN=local-cagent-token \
CAGENT_ADDR=:8080 \
CAGENT_CODEX_MODEL_CONTEXT_WINDOW=1000000 \
CAGENT_CODEX_MODEL_AUTO_COMPACT_TOKEN_LIMIT=900000 \
CAGENT_DEFAULT_REASONING_EFFORT=xhigh \
go run ./cmd/cagent serve
```

Those two Codex settings are forwarded as `-c model_context_window=1000000` and `-c model_auto_compact_token_limit=900000` on each `codex exec` call.

`CAGENT_DEFAULT_REASONING_EFFORT` is optional. Use it when you want every Droid request that does not specify a reasoning profile to run with the same Codex effort.

The same settings can be supplied as flags:

```sh
cagent serve \
  --addr :8080 \
  --token local-cagent-token \
  --model-context-window 1000000 \
  --model-auto-compact-token-limit 900000 \
  --default-reasoning-effort xhigh
```

## One-command setup

Run:

```sh
cagent droid setup
cagent droid doctor
```

From source:

```sh
go run ./cmd/cagent droid setup
go run ./cmd/cagent droid doctor
```

`setup` updates `~/.factory/settings.json`, writes a timestamped backup, and makes cagent the default for the main Droid session plus all three mission roles: orchestrator, worker, and validator.

By default, setup skips Droid's automatic milestone scrutiny and user-testing validators. The implementation worker still runs through Droid Mission mode and still calls `EndFeatureRun`; this default avoids the extra validation workers that currently make BYOK/cagent missions slow and brittle. To force the full Droid validation phase, run:

```sh
cagent droid setup --skip-scrutiny=false --skip-user-testing=false
```

The selected custom model id is stable:

```text
custom:cagent-gpt-5-5-xhigh-128k-max
```

Use `cagent droid launch --cwd /path/to/repo` to start interactive Droid after setup.

Use `cagent droid exec --cwd /path/to/repo "mission prompt"` for non-interactive mission runs. That wrapper passes Droid's `--model`, `--worker-model`, and `--validator-model` flags directly.

## Recommended settings.json

`cagent droid setup` writes the equivalent of this to `~/.factory/settings.json`:

```json
{
  "sessionDefaultSettings": {
    "interactionMode": "auto",
    "autonomyLevel": "high",
    "autonomyMode": "auto-high",
    "model": "custom:cagent-gpt-5-5-xhigh-128k-max",
    "reasoningEffort": "xhigh"
  },
  "missionOrchestratorModel": "custom:cagent-gpt-5-5-xhigh-128k-max",
  "missionOrchestratorReasoningEffort": "xhigh",
  "missionModelSettings": {
    "workerModel": "custom:cagent-gpt-5-5-xhigh-128k-max",
    "workerReasoningEffort": "xhigh",
    "validationWorkerModel": "custom:cagent-gpt-5-5-xhigh-128k-max",
    "validationWorkerReasoningEffort": "xhigh",
    "skipScrutiny": true,
    "skipUserTesting": true
  },
  "compactionTokenLimit": 900000,
  "compactionTokenLimitPerModel": {
    "gpt-5.5": 900000,
    "codex-default": 900000,
    "codex:gpt-5.5:medium": 900000,
    "codex:gpt-5.5:high": 900000,
    "codex:gpt-5.5:xhigh": 900000,
    "custom:cagent-gpt-5-5-medium-64k-safe": 900000,
    "custom:cagent-gpt-5-5-medium-128k-max": 900000,
    "custom:cagent-gpt-5-5-high-64k-safe": 900000,
    "custom:cagent-gpt-5-5-high-128k-max": 900000,
    "custom:cagent-gpt-5-5-xhigh-64k-safe": 900000,
    "custom:cagent-gpt-5-5-xhigh-128k-max": 900000,
    "custom:cagent-codex-default-chat-64k-safe": 900000
  },
  "customModels": [
    {
      "id": "custom:cagent-gpt-5-5-medium-64k-safe",
      "model": "codex:gpt-5.5:medium",
      "displayName": "cagent GPT-5.5 Medium 64K Safe",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "openai",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 64000
    },
    {
      "id": "custom:cagent-gpt-5-5-medium-128k-max",
      "model": "codex:gpt-5.5:medium",
      "displayName": "cagent GPT-5.5 Medium 128K Max",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "openai",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 128000
    },
    {
      "id": "custom:cagent-gpt-5-5-high-64k-safe",
      "model": "codex:gpt-5.5:high",
      "displayName": "cagent GPT-5.5 High 64K Safe",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "openai",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 64000
    },
    {
      "id": "custom:cagent-gpt-5-5-high-128k-max",
      "model": "codex:gpt-5.5:high",
      "displayName": "cagent GPT-5.5 High 128K Max",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "openai",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 128000
    },
    {
      "id": "custom:cagent-gpt-5-5-xhigh-64k-safe",
      "model": "codex:gpt-5.5:xhigh",
      "displayName": "cagent GPT-5.5 XHigh 64K Safe",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "openai",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 64000
    },
    {
      "id": "custom:cagent-gpt-5-5-xhigh-128k-max",
      "model": "codex:gpt-5.5:xhigh",
      "displayName": "cagent GPT-5.5 XHigh 128K Max",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "openai",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 128000
    },
    {
      "id": "custom:cagent-codex-default-chat-64k-safe",
      "model": "codex-default",
      "displayName": "cagent Codex Default Chat 64K Safe",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "generic-chat-completion-api",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 64000
    }
  ]
}
```

Droid's `/context` command uses the selected custom model id, not only the underlying `codex:gpt-5.5:xhigh` value. That is why the exact `custom:cagent-...` keys are required in `compactionTokenLimitPerModel`.

Naming convention:

- `Medium`, `High`, and `XHigh` are Codex reasoning efforts.
- `64K Safe` is the conservative Droid streaming profile.
- `128K Max` uses the official GPT-5.5 maximum output budget and should be treated as aggressive until long streaming is benchmarked.

Use the `openai` provider when you want Droid to call `POST /v1/responses`.

Use `generic-chat-completion-api` when you want Droid to call `POST /v1/chat/completions`.

Mission mode requires the `openai` provider. Droid sends mission actions as OpenAI Responses tools, and `cagent` translates Codex's tool-call signal back into Responses `function_call` stream events for Droid to execute.

Droid's LLM-visible mission tool names are PascalCase. `cagent` preserves those exact names in the streamed `function_call`, while accepting snake_case and kebab-case aliases when Codex emits them:

| Droid function name | Common aliases accepted by cagent | Where it is used |
| --- | --- | --- |
| `ProposeMission` | `propose_mission`, `propose-mission` | Orchestrator proposes and initializes a mission |
| `StartMissionRun` | `start_mission_run`, `start-mission-run` | Orchestrator starts or resumes the mission runner |
| `DismissHandoffItems` | `dismiss_handoff_items`, `dismiss-handoff-items` | Orchestrator dismisses explicit handoff items |
| `EndFeatureRun` | `end_feature_run`, `end-feature-run` | Worker reports feature completion and hands off |

Pause/resume controls are split in Droid. `StartMissionRun` is the LLM tool used to resume a paused mission, including `resumeWorkerSessionId` and `restartFeature`. Pausing itself is not an LLM tool; Droid handles it internally through session interruption (`droid.interrupt_session`) and records `mission_paused` in mission progress.

For a new mission, `ProposeMission` is only the proposal step. After the user accepts the proposal, the orchestrator must create the runner artifacts before calling `StartMissionRun`:

- `features.json`
- `validation-contract.md`
- `validation-state.json`
- `AGENTS.md`
- `services.yaml`
- `skills/<skillName>/SKILL.md`

For worker sessions, returning a normal assistant message is not enough to finish the feature. The worker must call `EndFeatureRun` with the structured handoff payload after implementation or when blocked.

When the deterministic mission-resume bridge fires, the server logs:

```text
responses tool bridge auto_call tool=StartMissionRun
```

For other client tools, `cagent` asks Codex to emit:

```json
{"cagent_tool_call":{"name":"StartMissionRun","arguments":{"resumeWorkerSessionId":"..."}}}
```

and returns the matching Droid Responses `function_call` stream events.

## Model selection

Factory custom models are selected with the `custom:` prefix. `cagent droid setup` gives them stable ids, so the XHigh 128K profile is:

Example:

```sh
droid exec --model "custom:cagent-gpt-5-5-xhigh-128k-max" "analyze this repository"
```

## Reasoning profiles

The most reliable way to force Codex reasoning through Droid is to put the cagent reasoning suffix in the custom model's underlying `model` value:

```json
{
  "model": "codex:gpt-5.5:xhigh",
  "displayName": "cagent GPT-5.5 XHigh",
  "baseUrl": "http://localhost:8080/v1",
  "apiKey": "local-cagent-token",
  "provider": "openai",
  "maxContextLimit": 1000000,
  "maxOutputTokens": 128000
}
```

cagent parses that as model `gpt-5.5` with reasoning effort `xhigh`, then forwards `-c model_reasoning_effort="xhigh"` to `codex exec`.

As a server-wide fallback, run cagent with `CAGENT_DEFAULT_REASONING_EFFORT=high` or `CAGENT_DEFAULT_REASONING_EFFORT=xhigh`. You can also encode the same fallback in `CAGENT_DEFAULT_MODEL=codex:gpt-5.5:xhigh` for requests that use `codex-default`.

For Droid's own reasoning selector on custom models, the installed Droid CLI also accepts `enableThinking`, `thinkingMaxTokens`, and `reasoningEffort` in `customModels`. Treat that as Droid/provider metadata. The cagent model suffix is the deterministic Codex path, especially for `xhigh`.

## Token values

`maxOutputTokens` is Droid's configured response budget for the custom model.

`maxContextLimit` is Droid's custom-model context ceiling. It is accepted by the installed Droid CLI even though it is not listed in Factory's public BYOK docs as of 2026-04-28.

`compactionTokenLimitPerModel` controls Droid's `/context` denominator and auto-compaction threshold. Without it, Droid defaults custom/BYOK sessions to a 250000-token compaction limit even when `cagent` and Codex are configured for a larger context window.

Current known values:

| Model | Official context | Official max output | Recommended Droid output |
| --- | ---: | ---: | ---: |
| `gpt-5.5` | 1050000 | 128000 | 64000 |
| `gpt-5.4` | 1050000 | 128000 | 64000 |
| `gpt-5.4-mini` | 400000 | 128000 | 64000 |
| `gpt-5.3-codex` | 400000 | 128000 | 64000 |
| `gpt-5.2-codex` | 400000 | 128000 | 64000 |
| `gpt-5-codex` | 400000 | 128000 | 64000 |

OpenAI's current model catalog lists `gpt-5.5` with `128K` max output and reasoning levels including `xhigh`: https://developers.openai.com/api/docs/models

`64000` is the safer default until the full Droid to cagent to Codex path is benchmarked with long streaming.

`128000` is the official max output for GPT-5.5 and the listed OpenAI models, including when the reasoning effort is `xhigh`; it should be treated as an aggressive profile for Droid streaming.

## Local capability matrix

Generate a matrix from the local Codex catalog plus curated official caps:

```sh
cagent models --format markdown
```

From source:

```sh
go run ./cmd/cagent models --format markdown
```

## Local benchmark before Droid

Benchmark direct Codex CLI:

```sh
cagent bench --mode codex-cli --targets 8192,16384,32768,64000 --timeout 30m
```

To benchmark with the same Codex context overrides used by the server:

```sh
cagent bench \
  --mode codex-cli \
  --model-context-window 1000000 \
  --model-auto-compact-token-limit 900000 \
  --targets 8192,16384,32768,64000 \
  --timeout 30m
```

Benchmark through the HTTP Responses endpoint:

```sh
cagent bench \
  --mode responses \
  --base-url http://localhost:8080/v1 \
  --api-token local-cagent-token \
  --targets 8192,16384,32768,64000 \
  --timeout 30m
```

The benchmark reports requested tokens, actual output tokens when available, duration, output bytes, completion status, and sentinel detection.

## Current caveat

In `codex exec` mode, `maxOutputTokens` is not proven to be a hard limiter. A local 512-token benchmark returned more than 512 output tokens. Direct Responses passthrough is the likely path for strict `max_output_tokens` enforcement.
