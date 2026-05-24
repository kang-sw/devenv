---
title: Gemini CLI stream-json backend contract
related:
  260429-research-host-neutral-ws-plugin: host-neutral backend architecture anchor
related-mental-model:
  - named-agent-runtime
completed: 2026-05-24
---

# Gemini CLI stream-json backend contract

## Background

The ws named-agent runtime already treats `gemini-*` model names as
`backend: gemini` for configuration and diagnostics, but the runtime has no
Gemini runner. Before implementing one, the Gemini CLI headless contract needs
to be captured from observed behavior instead of assuming it matches Codex or
Claude exactly.

Local observations used `@google/gemini-cli` `0.29.6`.

## Authentication Findings

Interactive Gemini setup does not make `gemini -p` work when the configured
auth type is `gemini-api-key` and `GEMINI_API_KEY` is absent from the process
environment. In that state, headless invocation exits with code `41` and asks
for `GEMINI_API_KEY`.

OAuth headless mode works when the process environment includes
`GOOGLE_GENAI_USE_GCA=true`. With that environment, `gemini -p "hello"` and
`gemini --output-format stream-json ...` can run successfully from the ws
workspace.

## Stream JSON Shape

Successful `stream-json` output is newline-delimited JSON, but not every stdout
line is JSON. Gemini may print plain text prelude lines before the first JSON
event, including true-color warnings, YOLO notices, and tool fallback messages.
It may also print plain text retry diagnostics between JSON events when model
capacity is temporarily exhausted.

The normal successful event sequence is:

```text
init -> message(role=user) -> message(role=assistant, delta=true)* -> result(status=success)
```

`init` contains `session_id` and `model`. `message` events carry `role`,
`content`, and, for streamed assistant chunks, `delta: true`. The terminal
`result` event contains `status` and `stats`, but it does not contain the final
assistant text. A ws Gemini parser must accumulate assistant `message.content`
chunks to produce `RunnerResult.Text`.

Observed terminal success example fields:

```json
{"type":"result","status":"success","stats":{"total_tokens":11616,"input_tokens":11576,"output_tokens":2,"tool_calls":0}}
```

## Prompt And Resume Findings

Gemini headless mode accepts prompts through stdin. A command shaped as:

```text
printf '<prompt>' | GOOGLE_GENAI_USE_GCA=true gemini --output-format stream-json --approval-mode yolo
```

emits the same `init`, `message`, and `result` shape as `-p`.

`--resume <session_id>` works with the `session_id` captured from `init`. The
resumed call emits a new `init` event whose `session_id` is the resumed id, then
continues with the new user prompt and assistant response.

## Tool Event Shape

A safe cwd-identification prompt produced:

```text
tool_use -> tool_result -> message(role=assistant, delta=true) -> result(status=success)
```

`tool_use` carries `tool_name`, `tool_id`, and `parameters`. `tool_result`
carries `tool_id` and `status`; successful results may omit a textual `output`
field. The ws runner should preserve tool events in diagnostic streams, but
final caller-facing text should still come only from assistant message chunks.

## Parser Implications

Gemini parsing must be more tolerant than the current Codex JSONL parser:

- Ignore non-JSON stdout lines before, between, and after valid Gemini JSON
  events, while preserving raw streams for diagnostics.
- Capture `SessionID` from the first valid `init.session_id`.
- Append assistant `message.content` chunks in arrival order.
- Treat `result.status == "success"` as terminal success.
- Treat `result.status == "error"` as a backend error using
  `error.type` and `error.message` when present.
- Fail if the process exits without a terminal `result` event or without any
  accumulated assistant text on a nominal success.

This behavior differs from Codex, where non-JSON output before the final result
is fatal. Gemini's CLI emits operational notices on stdout even in
`stream-json` mode, so strict Codex parsing would reject valid Gemini runs.

## Closeout

The observed Gemini stream-json contract has been consumed by the implemented
Gemini named-agent runner and documented in the named-agent runtime spec.
