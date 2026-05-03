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
