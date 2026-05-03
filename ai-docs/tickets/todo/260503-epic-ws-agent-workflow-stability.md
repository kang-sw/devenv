---
title: ws agent workflow stability
related:
  260503-feat-agents-plugin-agent-session-runtime: initial named and oneshot agent runtime
  260503-feat-agents-plugin-async-agent-calls: async call, wait, status, tail, and cancel surface
  260503-feat-agents-plugin-write-code-port: first production workflow that dogfoods named agents
  260503-feat-ws-mcp-git-read-primitives: dogfood run that exposed lifecycle failures under write-code
  260503-epic-agents-plugin-skill-porting: skill roadmap that depends on stable orchestration
---

# ws agent workflow stability

## Background

`agents-plugin` now has enough MCP-backed agent runtime to port and dogfood
`write-code`, but the first non-trivial run showed that the runtime is not yet
stable enough to be treated as the default production path. The implementation
work completed successfully, and the named implementer/reviewer pattern found a
real correctness issue, but the orchestration itself consumed too much lead
attention because timeouts, cancellation, and child process cleanup were not
predictable.

This epic is a live stabilization document for the named-agent workflow. It
tracks the runtime fixes required before `write-code`, later `implement`, and
other delegated production workflows can be considered low-friction defaults
rather than expensive dogfood exercises.

## Scope

In scope:

- Stabilize `ws/agents.*` process lifecycle for Codex-backed named and oneshot
  agents.
- Make async calls observable without forcing the lead to repeatedly inspect
  raw tail output.
- Ensure cancellation, timeout, and cleanup semantics match the process tree.
- Reduce lead context usage by returning concise structured status and final
  summaries.
- Preserve the durable named-agent conversation model: `agents.register` resets
  or creates a task-scoped agent name, and repeated `agents.call_async` calls on
  that name continue the conversation.
- Keep the runtime host-neutral where possible while allowing Codex-specific
  adapter behavior where Codex CLI semantics require it.

Out of scope:

- Reworking the whole skill-porting sequence.
- Adding `ws/git.commit` or ticket/spec graph tools; those belong to
  `260503-epic-ws-mcp-vcs-reference-tools`.
- Updating repository specs or mental models on this branch.
- Treating project-specific build/test commands as ws-owned MCP behavior.

## Decisions

- Treat `write-code` dogfood failures as runtime blockers before porting heavier
  orchestration skills such as `implement`, `proceed`, or `sprint`.
- Keep `call_async` as the explicit asynchronous verb. `call` remains the
  synchronous operation, and `oneshot` remains a convenience route that should be
  implemented in terms of lower-level runtime primitives where practical.
- Prefer structured lifecycle fields over model interpretation of raw logs:
  process state, backend state, session id, started/completed timestamps,
  cancellation result, exit code, and final-output availability should be visible
  through MCP.
- Cleanup must be conservative: the runtime may terminate child processes that
  it owns, but it must not kill unrelated Codex or shell processes outside the
  recorded process tree.

## Current Failure Evidence

During `260503-feat-ws-mcp-git-read-primitives`, `write-code` was used as the
driver workflow. The run exposed these issues:

- `agents.oneshot` project survey timed out and left nested Codex/MCP processes
  that required manual cleanup.
- `agents.wait` and `agents.status` hit the host tool-call timeout ceiling even
  when the async worker later completed and wrote a usable final output.
- `agents.cancel` marked runtime state as cancelled while some child Codex
  processes survived and had to be found with `ps`.
- Reviewer re-checks could consume lead context because the lead had to inspect
  tail/status output repeatedly instead of receiving a compact terminal summary.
- The runtime has no persistent append-only diagnostic log suitable for later
  debugging after a failed or timed-out MCP call.

These failures do not invalidate the named-agent pattern. They show that the
runtime needs a lifecycle and observability hardening pass before broader use.

## Planned Child Work

- Lifecycle hardening: make start, wait, status, tail, cancel, and cleanup
  reflect the actual owned process tree.
- Timeout semantics: distinguish host MCP call timeout, backend still-running
  state, backend completed state, and backend failed state.
- Diagnostic logging: append concise runtime events to project-scoped state so a
  later session can inspect failures without reconstructing from chat context.
- Summary ergonomics: return compact final summaries and change/review metadata
  so the lead does not need to read large tails unless debugging.
- Workflow retry policy: document when a lead should retry, cancel, resume, or
  fall back to direct execution.

## Phases

### Phase 1: Dogfood failure hardening

Fix the concrete failures observed during the Git read primitive dogfood run.
The first implementation slice should make it possible to run a comparable
`write-code` delegation without manual `ps` cleanup or repeated raw tail
inspection.

Success criteria:

- Reproduce or directly test the process-lifecycle cases from the dogfood run:
  oneshot timeout, async wait timeout, cancellation, completed-worker recovery,
  and reviewer-style long-running calls.
- `agents.cancel` either terminates all runtime-owned child processes or reports
  exactly which owned processes remain and why.
- `agents.status` returns enough structured state to distinguish running,
  completed, failed, cancelled, and cleanup-needed calls without reading raw
  logs.
- `agents.wait` can be used safely with bounded host timeouts; if the backend is
  still running, the result must preserve a follow-up path instead of looking
  like an ambiguous failure.
- Runtime diagnostics are appended under the existing ws project state root.
- Local Go tests and a real Codex-backed smoke cover the fixed behavior.

### Phase 2: Lead-context compression

Reduce the amount of lead context required for normal delegated workflows.
Status, wait, print, and tail should return compact summaries by default and
make large raw logs an explicit debugging action.

Success criteria:

- `write-code` can inspect implementer and reviewer outcomes through compact
  final summaries or structured output paths.
- Tail output supports bounded line counts and does not encourage reading full
  model transcripts during normal operation.
- Review relay instructions can point agents at files and summaries instead of
  requiring the lead to paste long review bodies through chat.

### Phase 3: Workflow regression dogfood

Re-run `write-code` on a small but real implementation after Phase 1 and Phase 2
land. Use the result to update this epic and any affected skill wording.

Success criteria:

- The dogfood run completes without orphaned runtime-owned processes.
- The lead can recover implementer/reviewer state from MCP summaries and
  diagnostic logs.
- Remaining workflow gaps are captured as new child tickets or later phases
  before this epic closes.
