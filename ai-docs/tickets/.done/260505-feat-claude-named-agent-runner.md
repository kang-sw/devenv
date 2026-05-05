---
title: Claude named-agent runner
related:
  260505-feat-agent-backend-failure-diagnostics: diagnostics already expose Claude availability but execution remains unsupported
spec:
  - 260505-claude-agent-runner
related-mental-model:
  - named-agent-runtime
completed: 2026-05-05
---

# Claude named-agent runner

## Background

The named-agent runtime can store `backend: claude` through registration and tier
configuration, and backend failure diagnostics already detect the `claude`
binary. However, actual named-agent execution still supports only the Codex
runner. Claude-backed agents therefore fail as unsupported even when Claude is
installed and usable.

The legacy `claude-plugin/bin/ws-named-agent` script contains proven Claude CLI
invocation details, including `claude -p --output-format json`, session id
handling, resume behavior, and hook settings. That script should be used as
prior art for CLI flags and hook shape, not copied as an architecture. The Go
runtime should keep the current `Runner` interface and add Claude as a backend
adapter that fits the existing manager lifecycle.

## Decisions

- Keep `Runner.Call(RunnerRequest) (RunnerResult, error)` as the backend
  boundary.
- Do not port the legacy Python registry, config, compression, or output-print
  structure.
- Add a Claude runner that consumes only `RunnerRequest` fields and returns
  `RunnerResult`.
- Generate a Claude session id inside the runner when a first call has no stored
  session id, then report it through `OnSessionID` so the existing manager
  persistence path is reused.
- Use the legacy Claude script only for verified CLI details: output JSON shape,
  `--session-id`, `--resume`, `--system-prompt`, and hook settings.
- Leave Gemini unsupported unless a separate ticket adds a Gemini runner.

## Phases

### Phase 1: Spec coverage

Add a planned entry to the named-agent runtime spec covering Claude-backed
runner behavior before promoting this ticket to `todo/`. The entry should define
the caller-visible contract without over-specifying internal filenames.

Success criteria:

- The ticket has a `spec:` frontmatter entry after promotion.
- The spec distinguishes the existing Codex JSONL adapter from the new Claude
  JSON adapter.
- The spec states that Claude support is implemented through the same
  named-agent lifecycle, result, status, tail, interrupt, and diagnostics
  surfaces.

### Result - 2026-05-05

Added `260505-claude-agent-runner` to the named-agent runtime spec, populated
the ticket `spec:` frontmatter, promoted the ticket to `todo/`, and added the
ticket to the project queue.

### Phase 2: Claude runner adapter

Implement a `ClaudeRunner` in the Go `wsagent` runtime and route
`backend: claude` registrations to it through the existing manager call path.
Keep manager-level branching minimal: backend selection should choose a runner,
while session persistence, stream capture, status transitions, inbox delivery,
and failure diagnostics remain shared.

Suggested approach:

- Introduce a backend runner factory for default runner selection.
- Build Claude commands from `RunnerRequest`:
  - `Root` as subprocess working directory.
  - `Prompt` as the user prompt input.
  - `Model` as `--model` except for backend shorthand values that should use
    Claude defaults.
  - `SessionID` as `--resume` when present.
  - a generated session id passed through `--session-id` when absent.
  - `SystemPromptPath` read into `--system-prompt`.
  - `InterruptHookCommand` encoded into Claude `--settings` JSON.
- Parse Claude JSON output into final text and error status.
- Preserve raw Claude process failures through existing backend invocation
  diagnostics.

Success criteria:

- Registering and calling a Claude-backed named agent no longer fails as an
  unsupported backend when `claude` is available.
- A first Claude call persists a session id and a later call resumes it.
- `agents.result`, `agents.status`, and `agents.tail` work through the existing
  async current-call state.
- Failed Claude invocations include the raw backend error plus existing PATH and
  reconfiguration hints.
- Tests cover command construction, first-call session id persistence, resume,
  JSON result parsing, and non-zero Claude exit behavior.

### Result - 2026-05-05

Implemented `ClaudeRunner` as a backend adapter behind the existing
`Runner.Call` interface. Default runner selection now routes `backend: claude`
to the Claude adapter while leaving Gemini unsupported through the existing
diagnostic path. The runner generates first-call Claude session ids, resumes
stored sessions, reads the registered system prompt into `--system-prompt`,
encodes interrupt hooks through Claude `--settings`, parses Claude JSON output,
and preserves non-zero process stderr for shared backend diagnostics.

Tests cover direct Claude command construction, backend shorthand handling,
first-call session persistence, resume behavior, JSON result parsing, non-zero
Claude failures, and an async manager call/result flow for a Claude-backed named
agent.
