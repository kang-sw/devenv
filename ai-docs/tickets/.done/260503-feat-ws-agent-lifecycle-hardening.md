---
title: ws agent lifecycle hardening
parent: 260503-epic-ws-agent-workflow-stability
related:
  260503-epic-ws-agent-workflow-stability: parent stabilization epic
  260503-feat-ws-mcp-git-read-primitives: dogfood run that exposed lifecycle failures
plans:
  phase-1: 2026-05/03-260503-epic-ws-agent-workflow-stability-phase-1
completed: 2026-05-03
---

# ws agent lifecycle hardening

## Background

The first non-trivial `write-code` dogfood run exposed process lifecycle
failures in the ws agent runtime: oneshot timeout, async wait ambiguity,
cancellation that did not clean the owned process tree, completed-worker
recovery gaps, and reviewer-style long-running calls that were hard to inspect.

This child ticket preserves the completed Phase 1 scope split out of
`260503-epic-ws-agent-workflow-stability`.

## Result (2c6f90f) - 2026-05-03

Implemented the first lifecycle hardening slice in `agents-plugin-tool`'s
`wsagent` runtime. `agents.status` preserved its text shape while adding
lifecycle fields a lead can act on without raw tails: `active`,
`cleanup_needed`, `cancel_pid`, stream paths, `runtime_log_path`, completed
`output_path`, and status-specific `follow_up` hints.

`agents.wait` now records `wait.timeout` in the runtime log and returns
`wait_timeout: true` plus safe follow-up commands instead of a bare timeout
prefix, so host-side wait timeouts no longer look like backend failures.

Cancellation now targets the runtime-owned process tree. Async Codex calls
inherit the worker process group, while synchronous calls keep their isolated
group for timeout cancellation. Unix cancellation also walks the descendant
process tree with `ps` and kills discovered process groups and PIDs, covering
tool children that create their own process groups. Windows kept a conservative
PID-kill fallback pending native Windows runtime smoke.

The real cancel smoke reproduced the important pre-fix failure: cancelling an
async Codex worker that launched `sleep 60` left the sleep process orphaned.
After the fix, the same smoke left no matching child process. A short
post-cancel liveness retry was added to avoid over-reporting `cleanup_needed`
while the killed worker was still being reaped.

Verification covered `go test ./internal/wsagent`, `go test ./...` from
`agents-plugin-tool`, runtime JSON parsing, `claude plugin validate
agents-plugin`, `git diff --check`, Windows compile-only coverage for
`cmd/ws-mcp`, a real Codex-backed async completion smoke, and a real
Codex-backed cancel smoke.
