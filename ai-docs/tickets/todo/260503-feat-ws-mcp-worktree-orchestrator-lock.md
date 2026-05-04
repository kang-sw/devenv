---
title: ws-mcp worktree orchestrator lock
parent: 260503-epic-ws-agent-workflow-stability
related:
  260503-epic-ws-agent-workflow-stability: Phase 6 child ticket for delegated tool-profile containment
  260503-feat-agents-plugin-write-code-port: dogfood workflow that exposed recursive reviewer spawning
---

# ws-mcp worktree orchestrator lock

## Background

The `config.show` `write-code` dogfood run showed that environment-only MCP tool
profiles do not reliably contain delegated Codex agents in plugin-managed
sessions. The implementer was intended to run as a `leaf` worker, but it
successfully called `ws/agents.call`, `ws/agents.wait`, and `ws/agents.erase`
to spawn and manage an internal reviewer. That result suggests the
plugin-managed MCP server may be session-level or otherwise not governed by the
subprocess environment that `CodexRunner` sets.

The runtime needs a profile boundary that does not depend on a nested Codex
process receiving a fresh MCP-server environment. Since ws already maintains
worktree-scoped project state, the first MCP server that opens a worktree can
claim lead orchestration authority with a worktree-local lock. Later MCP
servers for the same worktree become delegates even if their environment asks
for `lead`.

There is also a prompt-orientation gap. The Claude-era
`claude-plugin/infra/workflow-for-agent.md` document is not currently embedded
or injected into the Codex `agents-plugin` prompt bundle; `write-code`
implementers currently receive `implementer` plus `impl-playbook`. That may
contribute to delegated agents not understanding that they are subagents, but
prompt orientation is not a sufficient enforcement boundary. The lock must
enforce tool availability independently of whether the agent follows the prompt.

## Decisions

- Orchestration authority is worktree-local, not repository-global. Linked
  worktrees are separate working spaces and may each have one lead MCP server.
- The authority lock grants lead-level orchestration and mutation permissions;
  it is not a general file-edit mutex.
- Environment profiles become additional restrictions only. They may reduce a
  lock owner to `delegate` or `leaf`, but they may not raise a non-owner to
  `lead`.
- Non-owner MCP servers keep read/reference tools and bounded helpers according
  to the effective profile, but they must not expose lead-level orchestration or
  mutation tools such as `agents.*`, `config.*`, or future commit tools.
- Use lockfile creation rather than advisory flock so the mechanism remains
  portable across macOS, Linux, and Windows.

## Phases

### Phase 1: Worktree-local authority lock

Add a worktree-local lock directory to the ws state layout and implement
orchestrator lock acquisition during MCP server startup. The lock should live
under the worktree-local state directory rather than the project shared
directory.

Suggested layout:

```text
~/.cache/ws@kang-sw-devenv/proj/<worktree-key>/
  locks/
    orchestrator.lock
```

Suggested lock payload:

```json
{
  "schema_version": 1,
  "pid": 12345,
  "started_at": "2026-05-03T00:00:00Z",
  "root": "/Users/kang-sw/devenv",
  "worktree_key": "17da6bdc",
  "version": "0.1.0-dev"
}
```

Success criteria:

- The first live MCP server for a worktree becomes the lock owner.
- A second MCP server for the same worktree cannot acquire lead authority.
- A separate linked worktree can acquire its own independent lock.
- Stale lock recovery removes a lock whose recorded process is no longer alive.
- If liveness cannot be checked safely, the server behaves conservatively as a
  non-owner instead of stealing the lock.

### Result (6203e71) - 2026-05-04

Implemented a worktree-local `locks/orchestrator.lock` primitive in
`internal/wsstate` and added it to the ensured worktree layout. The lock payload
records schema version, pid, start time, worktree root, worktree key, and runtime
version. Acquisition uses `O_CREATE|O_EXCL`; stale recovery removes locks whose
pid is no longer alive. Unix liveness uses `kill(pid, 0)`, and Windows liveness
uses `OpenProcess` with query-limited access so the implementation compiles for
the planned Windows target.

Tests cover first-owner acquisition, second acquisition observing the existing
owner, stale-lock recovery, and linked worktrees acquiring independent locks.

### Phase 2: Effective role enforcement

Replace environment-only profile selection with an effective role calculation:

```text
base role:
  lock owner     -> lead
  lock non-owner -> delegate

effective role:
  min(base role, requested env profile)
```

The order is `lead > delegate > leaf`; env/profile may only move downward.

Success criteria:

- A lock owner with no env override sees the full lead tool surface.
- A non-owner with `WS_MCP_TOOL_PROFILE=lead` still receives delegate behavior.
- A lock owner with `WS_MCP_TOOL_PROFILE=leaf` receives leaf behavior.
- Delegate and leaf profiles hide and reject `agents.*`, `config.*`, and future
  mutation/orchestration namespaces.
- Existing `WS_MCP_ALLOWED_TOOLS` behavior is either preserved as a test-only
  explicit allowlist or deliberately narrowed so it cannot bypass the lock in
  production.

### Result (6203e71) - 2026-05-04

`internal/mcp.Server` now derives an effective role at startup from the
worktree-local lock plus `WS_MCP_TOOL_PROFILE`. Lock owners receive base `lead`;
non-owners receive base `delegate`; env profile can only reduce authority.
Delegate and leaf roles hide and reject `agents.*` and `config.*`; leaf also
hides and rejects `subquery`. `WS_MCP_ALLOWED_TOOLS` now narrows only after role
checks, so it cannot bypass lock-derived containment.

Unit tests cover owner leaf downgrade, non-owner lead escalation failure, tool
hidden-vs-rejected behavior, and allowlist non-bypass. A direct two-process
stdio smoke held one `ws-mcp` server open, then confirmed a second server for the
same worktree with `WS_MCP_TOOL_PROFILE=lead` could not see `agents.status` or
`config.show` while still seeing delegate-level `subquery`.

### Phase 3: Plugin-managed Codex smoke

Add or document a smoke test that exercises the actual failure mode from the
`config.show` dogfood run.

Success criteria:

- A delegated Codex implementer cannot see or call `agents.status` or
  `agents.call` in a plugin-managed session.
- The smoke distinguishes "tool hidden from tools/list" from "tool call rejected"
  and verifies both when feasible.
- The stability epic records the observed result and any host-specific
  limitation that remains.

### Result (manual smoke) - 2026-05-04

After reinstall/cache refresh, plugin-managed Codex delegation was smoke-tested
from the lead session. The lead MCP server could call `runtime.info`,
`config.show`, and `agents.*`. A lead-spawned delegated Codex agent reported
`config.show` and `agents.status` as unavailable while `subquery` remained
available. This confirms the worktree lock enforces the practical
lead-to-delegate containment boundary that blocks recursive `agents.*`
orchestration in the observed failure mode.

The smoke also showed a remaining host-specific gap: the async worker still sets
`WS_MCP_TOOL_PROFILE=leaf`, but plugin-managed Codex did not appear to propagate
that env restriction into the child MCP tool surface. The effective runtime
boundary is therefore delegate-level containment, not leaf-level containment,
until a durable role assignment mechanism replaces environment propagation for
leaf workers.

### Phase 4: Host-neutral agent workflow orientation

Decide how to port the useful parts of `workflow-for-agent.md` into the
Codex/open-conventions prompt bundle without reintroducing Claude-only CLI
commands or stale helper names.

Success criteria:

- Delegated implementer/reviewer prompts explicitly state that they are
  subagents and must not perform lead-owned orchestration, reviewer fanout,
  branch/merge control, ticket lifecycle management, or spec/mental-model
  lifecycle work unless the lead explicitly delegates that exact operation.
- The prompt text uses host-neutral MCP notation and current surfaces
  (`agents.register` + `agents.call` + `agents.wait/status/print/erase`) rather
  than Claude-only `ws-new-named-agent` or `ws-call-named-agent` commands.
- The prompt orientation complements the lock-based enforcement but does not
  replace it.

### Result - 2026-05-04

Added a host-neutral embedded `delegate-orientation` prompt and made public
`agents.register` prepend it before caller prompt material. The orientation
states that delegated workers must not spawn/manage agents, perform reviewer
fanout, or own ticket/spec/mental-model/release/merge lifecycle unless the lead
explicitly assigns that exact operation. This gives lead-spawned implementers
and reviewers a prompt-level first defense while keeping the worktree lock as
the enforcement boundary.

`subquery` intentionally does not receive the orientation. It uses the internal
`SubquerySystemPrompt` through `Subquery -> oneShot -> Register` with orientation
suppressed, because subquery is a scoped one-question helper and should not load
workflow orchestration instructions or self-reference subquery/ask-api guidance.

Follow-up: `current/prompt.md` is safe for a single call because workers start
after prompt write and state update, but same-agent concurrent `agents.call`
requests are not serialized by a file lock. A later stability slice should add a
per-agent current-call claim lock around `BeginCurrentCall` and prompt snapshot
creation.

### Phase 5: Current-call serialization and interrupt delivery

Implement the remaining named-agent runtime stability slice:

- serialize same-agent `agents.call` setup around `BeginCurrentCall`,
  `current/prompt.md`, and `current/state.json` updates so concurrent calls
  cannot overwrite each other's prompt snapshot or current-call state;
- expose `agents.interrupt` as a lead-owned MCP/CLI surface that writes durable
  lead-to-agent inbox messages;
- reuse the verified hook prior art where the host fires hooks, but keep a
  worker-side inbox watcher fallback for Codex `exec` hosts that accept inline
  hook config without firing it;
- resume the same Codex thread with delivered inbox messages and preserve
  file-backed status, tail, and recovery behavior.

Success criteria:

- A concurrent same-agent `agents.call` during setup is rejected before prompt
  or current state can be overwritten.
- `agents.interrupt` creates monotonic `inbox/<id>.json` messages and reports
  the queued message id.
- Active Codex async calls install an interrupt hook and, when interrupted,
  drain pending inbox messages into a follow-up resume prompt. If inline hooks
  do not fire on a host, the worker-side watcher still interrupts the Codex
  subprocess and resumes with the delivered inbox messages.
- `agents.wait`, `agents.status`, `agents.tail`, and `agents.print` continue to
  work from disk state after interrupt delivery.

### Result - 2026-05-04

Implemented same-agent current-call setup serialization with
`current/setup.lock`. `agents.call` now holds the setup lock around
`BeginCurrentCall`, prompt snapshot creation, state writes, and worker start; a
concurrent setup attempt is rejected before it can overwrite
`current/prompt.md` or `current/state.json`. Stale setup locks whose recorded
owner pid is no longer alive are recoverable.

Implemented durable interrupt delivery through `agents.interrupt`. Interrupts
append monotonic pending messages under `inbox/<id>.json`, and active workers
drain pending inbox messages into a resume prompt, mark them `delivered`, and
continue the same Codex thread. The MCP tool surface now includes
`agents.interrupt`; the CLI surface includes `agents interrupt` and the internal
`agents check-inbox` hook/check helper; `runtime.json` records both surfaces.

The implementation keeps the `PostToolUse` hook configuration shape, but WSL2
smoke on Codex CLI 0.128.0 showed inline hooks accepted through `-c` did not
fire during `codex exec --json`. To keep active interruption working on this
host, the Codex runner also watches durable inbox state and sends an interrupt
signal to the active Codex subprocess. A live smoke registered a temporary
agent, started a long-running shell instruction, queued `agents interrupt`, and
observed the worker log `call.interrupted`, deliver inbox message `000001`,
resume the same thread, and return `INTERRUPTED_DONE`.
