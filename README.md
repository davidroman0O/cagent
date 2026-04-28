# cagent

`cagent` is a Go API gateway for coding-agent CLIs. The core is provider-neutral: HTTP protocols are translated into a normalized agent turn, then a provider adapter runs the actual coding agent and streams normalized events back.

The first provider is the local Codex CLI via `codex exec --json`.

## Current Shape

- OpenAI-compatible `POST /v1/chat/completions`
- OpenAI Responses-compatible `POST /v1/responses`
- `GET /v1/models`, `GET /health`, `GET /metrics`, `GET /api/providers`
- Runtime sessions with `POST /api/sessions` and `POST /api/sessions/{id}/turns`
- Codex CLI provider with resume-aware arg construction
- Codex JSONL event parser for messages, completion, usage, commands, file changes, approvals, and questions
- Streaming SSE for Chat Completions and Responses
- Droid-compatible Responses client tool bridge for mission tools such as `StartMissionRun` and `EndFeatureRun`
- Droid mission prompts that teach Codex the `ProposeMission` -> artifact creation -> `StartMissionRun` -> `EndFeatureRun` flow
- Bearer token or `x-api-key` auth when `CAGENT_TOKEN` is set
- Per-request start/completion logs with status, response bytes, and duration
- Prometheus-style HTTP counters and duration totals at `GET /metrics`
- Concurrency and queue limits
- Session/event persistence under `~/.cagent` by default

## Run

```sh
go run ./cmd/cagent serve
```

Running `cagent` without a subcommand also starts the server for compatibility.

Release binaries include:

- `cagent`, with `serve`, `bench`, and `models` subcommands

Create a GitHub release by pushing a version tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

Useful env vars:

```sh
CAGENT_ADDR=:8080
CAGENT_TOKEN=local-secret
CAGENT_CODEX_BIN=/Users/davidroman/.bun/bin/codex
CAGENT_DEFAULT_CWD=/path/to/repo
CAGENT_DEFAULT_MODEL=
CAGENT_DEFAULT_REASONING_EFFORT=
CAGENT_CODEX_MODEL_CONTEXT_WINDOW=1000000
CAGENT_CODEX_MODEL_AUTO_COMPACT_TOKEN_LIMIT=900000
CAGENT_MAX_CONCURRENT=2
CAGENT_QUEUE_LIMIT=8
CAGENT_REQUEST_TIMEOUT=10m
CAGENT_DATA_DIR=$HOME/.cagent
```

Equivalent server flags are available under `cagent serve`, for example:

```sh
cagent serve \
  --addr :8080 \
  --token local-secret \
  --model-context-window 1000000 \
  --model-auto-compact-token-limit 900000 \
  --default-reasoning-effort xhigh
```

`CAGENT_DEFAULT_MODEL` is intentionally empty by default. Empty means “let the local Codex config choose.” The exposed model id `codex-default` also maps to that behavior. The default model may include cagent hints, for example `CAGENT_DEFAULT_MODEL=codex:gpt-5.5:xhigh`.

`CAGENT_CODEX_MODEL_CONTEXT_WINDOW` and `CAGENT_CODEX_MODEL_AUTO_COMPACT_TOKEN_LIMIT` are forwarded to every Codex turn as:

```sh
codex exec -c model_context_window=1000000 -c model_auto_compact_token_limit=900000 ...
```

OpenAI-compatible requests can also override those per turn with top-level `model_context_window` and `model_auto_compact_token_limit` fields, or through metadata aliases such as `context_window` and `auto_compact_token_limit`.

Reasoning effort can be supplied in three ways:

- `reasoning: {"effort":"high"}` or `reasoning_effort: "high"` in the request body
- `metadata.reasoning_effort` or `metadata.model_reasoning_effort`
- a cagent model suffix such as `codex:gpt-5.5:high` or `codex:gpt-5.5:xhigh`

If requests omit reasoning, set a server-wide default with `CAGENT_DEFAULT_REASONING_EFFORT=high` or `CAGENT_DEFAULT_REASONING_EFFORT=xhigh`.

cagent forwards this to Codex as `-c model_reasoning_effort="..."`.

For Factory Droid BYOK, set both Droid's hidden `maxContextLimit` custom-model field and `compactionTokenLimitPerModel`. Droid's `/context` command displays the compaction limit, and the default is 250000 unless overridden in `~/.factory/settings.json`. Use the `openai` provider for mission mode; Droid sends mission actions as Responses tools with names like `ProposeMission`, `StartMissionRun`, `DismissHandoffItems`, and `EndFeatureRun`. See [docs/droid-settings/README.md](/Users/davidroman/Documents/code/github/cagent/docs/droid-settings/README.md).

Model ids can carry provider and reasoning hints:

```text
codex-default
codex:gpt-5-codex
codex:gpt-5-codex:high
```

## Examples

Chat Completions:

```sh
curl http://localhost:8080/v1/chat/completions \
  -H 'content-type: application/json' \
  -d '{
    "model": "codex-default",
    "messages": [{"role": "user", "content": "Reply with exactly: ok"}]
  }'
```

Streaming:

```sh
curl -N http://localhost:8080/v1/responses \
  -H 'content-type: application/json' \
  -d '{
    "model": "codex-default",
    "stream": true,
    "input": "Reply with exactly: ok"
  }'
```

Runtime session:

```sh
SESSION_ID=$(curl -s http://localhost:8080/api/sessions \
  -H 'content-type: application/json' \
  -d '{"provider":"codex","cwd":"/tmp"}' | jq -r .id)

curl http://localhost:8080/api/sessions/$SESSION_ID/turns \
  -H 'content-type: application/json' \
  -d '{"input":[{"type":"text","text":"Reply with exactly: ok"}]}'
```

## Model Capability Matrix

Generate the local matrix:

```sh
go run ./cmd/cagent models --format markdown
```

This table separates local Codex catalog values from official API values. `Config context` is read from `~/.codex/config.toml` when `model_context_window` is present. `Recommended Droid output` is intentionally conservative for 128K-capable models until long streaming is benchmarked end to end.

| Model | Local context | Local max context | Config context | Official context | Official max output | Recommended Droid output | Reasoning | Notes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- |
| `gpt-5.5` | 272000 | 272000 | 1000000 | 1050000 | 128000 | 64000 | low/medium/high/xhigh | Official API docs list 1.05M context and 128K max output. |
| `gpt-5.4` | 272000 | 1000000 | 1000000 | 1050000 | 128000 | 64000 | low/medium/high/xhigh | Official API docs list 1.05M context and 128K max output. |
| `gpt-5.4-mini` | 272000 | 272000 | 1000000 | 400000 | 128000 | 64000 | low/medium/high/xhigh | Official API docs list 400K context and 128K max output. |
| `gpt-5.3-codex` | 272000 | 272000 | 1000000 | 400000 | 128000 | 64000 | low/medium/high/xhigh | Official API docs list 400K context and 128K max output. |
| `gpt-5.2` | 272000 | 272000 | 1000000 | 400000 | 128000 | 64000 | low/medium/high/xhigh | Official API docs list 400K context and 128K max output. |
| `codex-auto-review` | 272000 | 1000000 | 1000000 | unknown | unknown | 32768 | low/medium/high/xhigh |  |
| `gpt-5.3-codex-spark` | 128000 | 128000 | 1000000 | unknown | unknown | 32768 | low/medium/high/xhigh |  |

Official caps are curated in [official.go](/Users/davidroman/Documents/code/github/cagent/internal/modelcaps/official.go). Sources used for the current values:

- https://developers.openai.com/api/docs/models
- https://developers.openai.com/api/docs/models/gpt-5.5
- https://developers.openai.com/api/docs/models/gpt-5.4
- https://developers.openai.com/api/docs/models/gpt-5.4-mini
- https://developers.openai.com/api/docs/models/gpt-5.3-codex
- https://developers.openai.com/api/docs/models/gpt-5.2
- https://developers.openai.com/api/docs/models/gpt-5.2-codex
- https://developers.openai.com/api/docs/models/gpt-5-codex

## Tests

Normal suite:

```sh
go test ./...
```

The normal suite includes a local Codex CLI smoke test that runs `codex --version`.

Real Codex exec integration:

```sh
CAGENT_CODEX_EXEC_TEST=1 go test ./internal/provider -run TestCodexExecIntegration -v
```

That test runs one real `codex exec --json` turn with `read-only` sandbox and `approval_policy=never`.

## Local Benchmarks

Benchmark direct Codex CLI output behavior without Droid:

```sh
go run ./cmd/cagent bench \
  --mode codex-cli \
  --model-context-window 1000000 \
  --model-auto-compact-token-limit 900000 \
  --targets 8192,16384,32768 \
  --timeout 20m
```

Benchmark the HTTP Responses path through a running `cagent` server:

```sh
go run ./cmd/cagent serve

go run ./cmd/cagent bench \
  --mode responses \
  --base-url http://localhost:8080/v1 \
  --targets 8192,16384,32768 \
  --timeout 20m
```

The benchmark prints JSON with requested tokens, actual usage tokens when Codex reports them, duration, bytes, completion status, and whether the final sentinel marker was seen.

## Next Provider Targets

The runtime is designed so these can be added as separate adapters:

- Claude Code CLI
- Gemini CLI
- Cursor Agent
- Aider
- OpenCode

Each provider should declare capabilities, translate normalized turns into its native CLI/API call, and emit normalized `agent.AgentEvent` values.
