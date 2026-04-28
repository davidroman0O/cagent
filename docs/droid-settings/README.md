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

## Recommended settings.json

Add this to `~/.factory/settings.json`:

```json
{
  "compactionTokenLimitPerModel": {
    "gpt-5.5": 900000,
    "codex-default": 900000,
    "codex:gpt-5.5:high": 900000,
    "codex:gpt-5.5:xhigh": 900000
  },
  "customModels": [
    {
      "model": "gpt-5.5",
      "displayName": "cagent GPT-5.5 64K Safe",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "openai",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 64000
    },
    {
      "model": "gpt-5.5",
      "displayName": "cagent GPT-5.5 128K Max",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "openai",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 128000
    },
    {
      "model": "codex-default",
      "displayName": "cagent Codex Default Chat",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "generic-chat-completion-api",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 64000
    },
    {
      "model": "codex:gpt-5.5:high",
      "displayName": "cagent GPT-5.5 High 64K",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "openai",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 64000
    },
    {
      "model": "codex:gpt-5.5:xhigh",
      "displayName": "cagent GPT-5.5 XHigh 64K",
      "baseUrl": "http://localhost:8080/v1",
      "apiKey": "local-cagent-token",
      "provider": "openai",
      "maxContextLimit": 1000000,
      "maxOutputTokens": 64000
    }
  ]
}
```

Use the `openai` provider when you want Droid to call `POST /v1/responses`.

Use `generic-chat-completion-api` when you want Droid to call `POST /v1/chat/completions`.

## Model selection

Factory custom models are selected with the `custom:` prefix. The exact alias depends on Droid's custom model naming/indexing behavior.

Example:

```sh
droid exec --model "custom:cagent-GPT-5.5-64K-Safe-0" "analyze this repository"
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
  "maxOutputTokens": 64000
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

`64000` is the safer default until the full Droid to cagent to Codex path is benchmarked with long streaming.

`128000` is the official max output for the listed OpenAI models, but it should be treated as an aggressive profile.

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
