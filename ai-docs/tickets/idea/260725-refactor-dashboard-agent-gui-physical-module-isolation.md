---
title: "Physically isolate/extract the dashboard agent-GUI modules (FE + BE) — Tier 2 of the 2026-07-25 agent-GUI suspension"
related:
  260713-bug-dashboard-acceptance-codex-tile-transcript-hidden: suspended agent-GUI ticket whose Tier 2 physical extraction this ticket tracks
  260713-feat-ws-dashboard-agent-chat-real-adapter-wiring: suspended agent-GUI ticket whose Tier 2 physical extraction this ticket tracks
  260713-feat-ws-dashboard-activity-session-fork-cursor: suspended agent-GUI ticket whose Tier 2 physical extraction this ticket tracks
  260713-idea-dashboard-agent-chat-bubble-visual-design: suspended agent-GUI ticket whose Tier 2 physical extraction this ticket tracks
  260711-idea-dashboard-agent-facing-mcp-control-surface: suspended agent-GUI ticket whose Tier 2 physical extraction this ticket tracks
  260620-feat-ws-dashboard-agent-client-activity-sources: suspended agent-GUI ticket whose Tier 2 physical extraction this ticket tracks
related-mental-model:
  - ws-web-dashboard
---

# Physically isolate/extract the dashboard agent-GUI modules (FE + BE)

## Context

The dashboard agent-GUI feature was suspended per user directive
(2026-07-25). Tier 1 of that suspension hid the UI and made it un-spawnable
(spawn entry points disabled behind `AGENT_GUI_SUSPENDED`) and quarantined
the associated acceptance/e2e steps, so nothing agent-GUI-facing runs or is
reachable.

This ticket covers the deferred **Tier 2**: physically extracting and
wiring-out the agent modules on both the frontend and the
ws-dashboard-daemon backend, so the agent-GUI code is no longer entangled in
the live dashboard build rather than merely hidden. Tier 2 is **not** required
to unblock `260722` — Tier 1 already removes the feature from the surface.
This is a large cross-module refactor touching the `SurfaceKind` union,
shared Activity plumbing, and a ~15k-line route test file, so it needs sage
gating before it can move to `ready/`.

The isolation map below is captured in full so future implementation work
does not have to re-derive the seams. Line numbers are as of the suspension
snapshot and should be treated as anchors, not exact addresses.

## Frontend surface

Files to move/gate; seams to sever.

### Agent-owned cluster (move/gate as a unit)

- `src/agentChatSessions.ts`
- `src/agentChatPaneBody.tsx`
- `src/agentChatBubbles.tsx`
- `src/agentChatStreamMerge.ts`
- `src/agentChatResumeFromHere.tsx`
- `src/activitySessionClient.ts`
- `src/activitySessionApi.ts`
- `src/activitySessionStub.ts`
- `src/workbench/agentChatWorkbenchPane.tsx`
- `src/workbench/agentChatPlacement.ts`

### Wiring seams to cut

- `src/workbench/surfaceRegistry.ts` — `SurfaceKind` union member
  `"agentChat"` (L4) + the registry entry (L69-76).
- `src/workbench/editorGroups.ts` — hard imports (L13-14, L28),
  params (L65-67), `agentChatWorkbenchPanesByGroup` call (L100-104),
  spread (L166).
- `src/App.tsx` — builder call (L4341-4353) + state (L3668-3691) +
  effects (L4009-4158, L5194-5228) + close case (L5927-5942).
- `src/commands.ts` — `agentChat.create` members.
- `src/hotkeys.ts` — the agentChat binding.
- `src/workbench/index.ts` — barrel exports.

### Coupling risk (cannot move out with the feature)

- `src/workRootActivity.ts` — shared Activity types used by non-agent code.
- `src/resourcePresentation.tsx` `closeContractLabel` — shared exhaustive
  map.
- Removing the `SurfaceKind` union member cascades through all
  `Record<SurfaceKind, ...>` maps, so those consumers must be handled (kept
  total) even though they are not agent-owned.

## Backend surface (ws-dashboard-daemon)

Files to remove; seams to sever.

### Agent-exclusive

- `crates/daemon/src/codex_app_server.rs`
- `crates/daemon/src/codex_routes.rs`
- `crates/daemon/src/claude_cli.rs`
- `crates/daemon/src/claude_routes.rs`
- `crates/core/src/agent_client_provider.rs`
- `crates/core/src/codex_projection.rs`
- `crates/core/src/claude_projection.rs`
- Fixtures: `core/tests/fixtures/codex-app-server-turn.ndjson`,
  `core/tests/fixtures/claude-cli-turn.ndjson`.

### Router wiring

- `crates/daemon/src/router.rs` — handler imports (L57-64),
  `AppState` fields (L86-87), local routes (L414-449), server-scoped
  routes (L196-231).
- `crates/daemon/src/server.rs` — state init (L112-113).
- `crates/daemon/src/servers.rs` — route-key arms (L702-790),
  `server_scoped_codex_*` (L1187-1281),
  `server_scoped_claude_*` (L1283-1360).

### Entanglement seams to sever (shared/non-agent code reaching INTO agent state)

- `work_root_activity.rs` — merge calls (L205-221:
  `codex_activity_items` / `claude_activity_items`) + the native
  codex-session-file scan. Severance must fall back to empty items cleanly.
- `remove_for_work_roots` cleanup calls: `resources.rs` (L43),
  `root_picker.rs` (L375-376), `git_worktree.rs` (L593-594).

### Dangling dep

- `daemon/Cargo.toml` declares `ws-dashboard-harness-core` with zero
  usages — safe to drop.

### Tests to quarantine/remove

- Inline `mod tests` in `codex_app_server.rs` (L1301) and
  `claude_cli.rs` (L1080).
- `core/src/codex_projection.rs` and `core/src/claude_projection.rs` tests.
- Interleaved agent tests in `crates/daemon/tests/routes.rs`:
  Codex (L14815-15251), Claude (L15389-15740), activity-merge
  (L15252, L15688) + native backfill (L11101-11383, L8727), cleanup
  entanglement (L7419, L7502).

## Risks / notes

- The `SurfaceKind` union edit is wide-reaching: every
  `Record<SurfaceKind, ...>` and exhaustive `switch` over the union must stay
  total after the member is removed.
- The `work_root_activity.rs` merge severance must fall back to empty items
  cleanly so non-agent Activity rendering keeps working.
- The `routes.rs` test surgery is inside a ~15k-line file with agent and
  non-agent tests interleaved — extraction must not disturb the surviving
  non-agent tests.
- Prefer feature-flag / empty-fallback over hard deletion wherever a shared
  consumer still expects the symbol, to keep the change reversible and the
  surviving surfaces total.
