---
title: Exec output ask
parent: 260524-epic-async-exec-job-surface
related:
  260524-feat-exec-job-core-text-readers: supplies durable exec jobs and raw fallback readers
  260513-feat-async-exec-output-reader: original broad ticket absorbed by parent epic
  260524-epic-mcp-actor-setup-state: superseded — the actor-scoped setup model was removed by epic 260605; readers now bind to ephemeral session-key auth
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
  - plugin-runtime
---

# Exec output ask

## Background

The primary large-output UX for async exec jobs should be asking focused
questions about persisted stdout and stderr, not manually reading raw output.
Raw readers exist as fallback tools for inspection and debugging; `exec.ask`
should give the lead a compact answer grounded in the job output while keeping
large raw streams out of the lead context.

## Decisions

- Expose `exec.ask(exec_key, question, context_mode?)`.
- `context_mode` accepts `"fresh"` or `"resume"`. Omitted `context_mode`
  defaults to `"fresh"`.
- `"fresh"` answers in a new reader context for the current question.
- `"resume"` reuses a reader session associated with the same `exec_key` for
  follow-up questions.
- `exec.ask` should use the configured `light` model alias unless later spec
  work explicitly chooses a different alias policy.
- The reader receives the question, compact job metadata, stream byte counts,
  exit or running status, and controlled access to persisted stdout/stderr
  content. It must not receive arbitrary tool access or unrelated repository
  context.
- The answer should cite which stream or stream region it used when useful, but
  should avoid pasting large raw output back to the lead. Short excerpts are
  acceptable only when they directly support the answer.
- `exec.result` and launch responses for oversized output should point to
  `exec.ask` as the preferred next step, with `exec.raw.*` readers as fallback.

## Constraints

- Command output is untrusted input. The reader prompt must explicitly treat
  stdout and stderr as data, not instructions.
- Default fresh context is intentional. Resume mode is opt-in because prior
  reader assumptions plus untrusted output can compound stale or injected
  conclusions.
- `exec.ask` depends on the durable job records and persisted stream files from
  `260524-feat-exec-job-core-text-readers`.
- `exec.ask` must be hidden in wsflow no-agent mode.
- `exec.ask` reader sessions should bind to the ephemeral per-call session-key
  model (`ws.ferrule`-minted keys) introduced by epic 260605, which replaced the
  actor-scoped setup model; do not introduce an incompatible process-local reader
  identity. [staleness audit 2026-06-19: original clause referenced the removed
  actor-setup model]
- If the exec job is still running, `exec.ask` may answer from captured partial
  output, but the response must clearly state that the process was not terminal
  at the time of reading.
- If no output is available yet, `exec.ask` should return compact status and
  suggest `exec.status` or retrying later rather than fabricating an answer.

## Phases

### Phase 1: Add model-backed exec output questions

Implement `exec.ask` over persisted exec job output with fresh/resume context
mode, light-alias reader routing, prompt-injection safeguards, partial-output
handling, wsflow no-agent hiding, runtime metadata updates, and tests for:

- fresh context default;
- resume context for same-`exec_key` follow-up;
- rejection or clear failure for missing/unknown jobs;
- running jobs with partial output;
- no-output-yet jobs;
- large output answers that do not paste raw streams;
- stdout/stderr attribution in answers when useful;
- prompt-injection text embedded in command output;
- wsflow hidden-tool behavior and runtime contract drift.

Update `mcp-tools`, `named-agent-runtime`, `plugin-runtime`, and relevant
mental-model docs before promoting this child to `ready`.
