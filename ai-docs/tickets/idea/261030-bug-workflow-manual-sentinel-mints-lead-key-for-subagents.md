---
title: workflow_manual fresh sentinel mints a lead key for any caller, including subagents
related:
  261030-feat-note-layer-discipline-and-worker-readonly: the worker note read-only gate is bypassable through this hole
---

# workflow_manual fresh sentinel mints a lead key for any caller, including subagents

## Background

Dogfood finding (2026-10-30), verified in source:

- The keyed capability gate (`agents-plugin-tool/internal/mcp/server.go`, the
  block guarded by `entry, found := s.sessions.lookup(keyStr); found &&
  entry.scope != roleLead`) blocks lead-only tools only for a key that is
  found and non-lead. The fresh-bootstrap sentinel `obsidian-latch`
  (`internal/mcp/workflow_manual.go`, `freshBootstrapKey`) is not a stored
  session, so it passes.
- `handleWorkflowManual` with the sentinel and a `root` canonicalizes the root
  and mints a lead key, returning the full manual (`workflow_manual.go`, the
  `key == freshBootstrapKey` branch).
- The sentinel is printed literally in the shipped `SKILL.md` shims of
  `lead-discuss`, `lead-run`, `lead-delegate`, `lead-ticket`, and
  `lead-use-mailbox`, so any subagent that loads one of those skills learns it.
- The claude-mod adopts a minted key only for a main-thread call
  (`agents-plugin/hooks/claude-mod/manual.ts` header), so the lead's own key is
  not hijacked; the subagent still holds a lead-scoped key that unlocks every
  lead-only tool (`git.merge`, `worktree.*`, lead-authority config) and every
  role restriction, including the worker note read-only gate planned in
  `261030-feat-note-layer-discipline-and-worker-readonly`.

For reference, the other subagent paths behave as intended: a registered
delegate/leaf key gets a JSON-RPC `tool not available` error, and an
unregistered key gets only the "no restorable state" notice with no manual
body.

## Open Questions

- The server cannot tell a fresh lead from a subagent using the sentinel. The
  candidate signals (claude-mod `agentId` on `tool.call`, a harness-side
  marker, Pi spawn role env) are uninvestigated; the fix direction is
  unsettled.
