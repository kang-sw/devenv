---
title: ws agent workflow stability
related:
  260503-feat-agents-plugin-agent-session-runtime: initial named and oneshot agent runtime
  260503-feat-agents-plugin-async-agent-calls: async call, wait, status, tail, and cancel surface
  260503-feat-agents-plugin-write-code-port: first production workflow that dogfoods named agents
  260503-feat-ws-mcp-git-read-primitives: dogfood run that exposed lifecycle failures under write-code
  260503-epic-agents-plugin-skill-porting: skill roadmap that depends on stable orchestration
  260503-feat-ws-agent-lifecycle-hardening: completed Phase 1 child
  260503-feat-ws-agent-debug-diagnostics: completed Phase 2 child
  260503-feat-ws-agent-config-show-dogfood: completed Phase 3 child
  260503-feat-ws-mcp-nonblocking-orchestration: completed Phase 4 child
  260503-feat-ws-agent-tier-config: completed Phase 5 child
  260503-feat-ws-mcp-worktree-orchestrator-lock: active Phase 6 child ticket for worktree-local authority lock
  260504-feat-ws-mcp-hook-driven-interrupt: follow-up for Codex hook-driven interrupt delivery without signal dependence
---

# ws agent workflow stability

## Background

`agents-plugin` now has enough MCP-backed agent runtime to port and dogfood
`write-code`, but early non-trivial runs showed that the runtime was not stable
enough to be treated as the default production path. The implementation work
completed successfully, and the named implementer/reviewer pattern found real
issues, but orchestration consumed too much lead attention because timeouts,
cancellation, child process cleanup, and delegation containment were not yet
predictable.

This epic is the stabilization parent for the named-agent workflow. Completed
implementation slices now live in child tickets so this file stays a readable
roadmap rather than a long incident log.

## Scope

In scope:

- Stabilize `ws/agents.*` process lifecycle for Codex-backed named agents.
- Make async calls observable without forcing the lead to repeatedly inspect
  raw tail output.
- Ensure cancellation, timeout, and cleanup semantics match the process tree.
- Reduce lead context usage by returning concise structured status and final
  summaries.
- Preserve the durable named-agent conversation model: `agents.register` resets
  or creates a task-scoped agent name, and repeated `agents.call` calls on that
  name continue the conversation.
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
- Make `agents.call` the asynchronous start operation and remove the temporary
  `agents.call_async` and generic `agents.oneshot` surfaces before release.
- Prefer structured lifecycle fields over model interpretation of raw logs:
  process state, backend state, session id, timestamps, cancellation result,
  exit code, and final-output availability should be visible through MCP.
- Cleanup must be conservative: the runtime may terminate child processes that
  it owns, but it must not kill unrelated Codex or shell processes outside the
  recorded process tree.
- Environment-only MCP tool profiles are not sufficient for plugin-managed
  Codex containment. Lead authority should move to a worktree-local
  orchestrator lock, with environment profiles allowed only as additional
  restrictions.

## Completed Children

- `260503-feat-ws-agent-lifecycle-hardening` — Phase 1 dogfood failure
  hardening: lifecycle fields, wait timeout semantics, process-tree
  cancellation, and Codex-backed async/cancel smokes.
- `260503-feat-ws-agent-debug-diagnostics` — Phase 2 raw-output containment:
  debug-namespaced diagnostic tools and CLI fallbacks.
- `260503-feat-ws-agent-config-show-dogfood` — Phase 3 regression dogfood:
  `config.show` read API and named-agent reviewer flow.
- `260503-feat-ws-mcp-nonblocking-orchestration` — Phase 4 nonblocking MCP:
  concurrent stdio request handling, async `agents.call`, short-poll waits,
  removal of `agents.call_async` and generic `agents.oneshot`, and initial
  MCP tool profiles.
- `260503-feat-ws-agent-tier-config` — Phase 5 user-local tier model config:
  `config.agents_tier`, backend inference, registration precedence, and CLI
  fallback.

## Active Children

- `260503-feat-ws-mcp-worktree-orchestrator-lock` — Phase 6 containment fix:
  replace env-only delegated profile containment with worktree-local MCP lead
  authority. The first live MCP server for a worktree becomes lead; later MCP
  servers for the same worktree become delegates; environment profiles can only
  further restrict the effective role.
- `260504-feat-ws-mcp-hook-driven-interrupt` — Follow-up Codex interrupt fix:
  make `agents.interrupt` use hook-injected mailbox delivery as the primary
  active-turn path and keep process termination under `agents.cancel`.

## Remaining Failure Evidence

The `config.show` `write-code` dogfood run showed that plugin-managed Codex
sessions can still bypass intended leaf containment. The implementer was
intended to run as a `leaf` worker, but it successfully called
`ws/agents.call`, `ws/agents.wait`, and `ws/agents.erase` to spawn and manage
an internal reviewer. That reviewer found a real missing-ticket-result issue,
so the failure was workflow containment rather than output quality.

The current hypothesis is that `CodexRunner` can set `WS_MCP_TOOL_PROFILE=leaf`
for the nested Codex process, but plugin-managed MCP servers may be session-level
or otherwise not governed by the subprocess environment. The active child ticket
tracks the worktree-local lock design needed to make effective role selection
state-based instead of env-only.
