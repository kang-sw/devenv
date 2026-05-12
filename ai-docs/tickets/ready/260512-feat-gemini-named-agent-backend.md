---
title: Gemini named-agent backend
related:
  260512-research-gemini-cli-stream-json: observed Gemini CLI headless contract
  260429-research-host-neutral-ws-plugin: backend abstraction architecture anchor
spec:
  - 260512-gemini-agent-runner
skeletons:
  phase-1: a2f9d4b
related-mental-model:
  - named-agent-runtime
---

# Gemini named-agent backend

## Background

The ws named-agent runtime supports Codex and Claude runners behind a shared
`Runner` contract. Configuration already infers `backend: gemini` for
`gemini-*` model names and diagnostics already mention a local `gemini` binary,
but actual invocation still fails because `runnerForBackend` has no Gemini
runner.

Add Gemini CLI as a third backend while preserving the existing manager
lifecycle: registration, async calls, stream capture, session persistence,
status/result/tail diagnostics, cancellation, and backend failure formatting
should stay on the shared path.

## Decisions

- Use Gemini CLI headless `stream-json` output, not final `json`, so the runner
  matches the Codex-style streaming parser direction.
- Deliver prompts through stdin so multiline prompts and system-prompt wrapping
  avoid argv-size and quoting issues.
- Build final `RunnerResult.Text` from assistant `message` delta chunks because
  Gemini `result` events carry stats and terminal status, not final text.
- Accept Gemini stdout prelude and retry noise as diagnostics instead of parse
  failure; require terminal JSON `result` before declaring success.
- Preserve auth failures as backend invocation failures. Do not add login or
  credential probes to registration/config paths.

## Constraints

- Headless Gemini may require auth environment such as `GOOGLE_GENAI_USE_GCA=true`
  for OAuth or `GEMINI_API_KEY` for API-key mode. The runner should not read or
  store credentials.
- Gemini `stream-json` may interleave non-JSON stdout lines with valid JSONL.
  Parser tests must cover this explicitly.
- Gemini successful `tool_result` events can omit textual output. Tool events
  are diagnostics, not final answer text.
- Interrupt parity should be conservative in the first implementation. Existing
  ws inbox messages can be prepended on the next resumed call; do not claim
  Gemini live hook delivery until a stable hook contract is implemented.

## Prior Art

`CodexRunner` already shows the intended streaming shape: build an invocation,
start a subprocess, tee stdout to diagnostics, parse JSONL incrementally, call
`OnSessionID` as soon as the session id appears, and return `RunnerResult` only
after a complete terminal result. Gemini should follow that structure with a
Gemini-specific parser.

`ClaudeRunner` remains useful for session resume and system-prompt handling, but
it parses one final JSON object and should not be copied for the stream parser.

## Phases

### Phase 1: Gemini stream parser

Add a Gemini stream parser that reads stdout incrementally and returns the
shared `RunnerResult` shape.

Success criteria:

- Captures `SessionID` from `init.session_id`.
- Accumulates assistant `message.content` chunks into `Text`.
- Marks terminal success only after `result.status == "success"`.
- Returns backend errors for `result.status == "error"`.
- Ignores non-JSON stdout noise before and between JSON events while leaving raw
  streams available through existing diagnostics.
- Fails on missing terminal `result`, missing session id, or missing assistant
  text on nominal success.

### Phase 2: Gemini runner invocation

Implement `GeminiRunner`, add it to `runnerForBackend`, and invoke the Gemini
CLI through the shared manager path.

Suggested approach:

- Start first calls with `gemini --output-format stream-json --approval-mode yolo`.
- Add `-m <model>` when a concrete model is resolved.
- Add `--resume <session_id>` for resumed calls.
- Send the final prompt over stdin.
- If `SystemPromptPath` is present, read it and prepend a clearly delimited
  system-instruction block to the stdin prompt inside the Gemini runner.
- Preserve `ToolProfile` environment behavior where it already flows through
  backend subprocesses.

### Phase 3: Tests, docs, and diagnostics

Update focused tests and workflow docs for the new backend.

Success criteria:

- Parser tests cover normal text, stdin-shaped prompt output, resume-shaped
  output, tool events, terminal error events, and non-JSON noise.
- Runner tests use a fake `gemini` executable and verify args, stdin prompt
  delivery, session persistence, and backend version capture where available.
- Unsupported-backend diagnostics are adjusted because `gemini` becomes
  supported.
- `ai-docs/spec/named-agent-runtime.md` and the named-agent mental model record
  Gemini runner behavior, auth expectations, and noise-tolerant parsing.
