---
title: ws dashboard command surface (quick-open, custom commands, shortcuts)
related:
  260710-epic-ws-dashboard-terminal-ux-polishing: sibling board; that epic's Non-Scope excludes new product surfaces, which is why this direction was split out instead of folded in
  260622-epic-ws-dashboard-session-key-realignment: sibling board; owns the agent/MCP-facing half of custom commands (execution/registration approval, harness/session authority) so this epic stays UI-only
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard command surface (quick-open, custom commands, shortcuts)

## Scope

A new epic for the dashboard's human-facing command/interaction surface,
split out on 2026-07-11 explicitly because bundling it with
`260622-epic-ws-dashboard-session-key-realignment` (agent-harness/
session-key authority) was judged too broad a stretch — the two boards
have different risk shapes and different "who decides" boundaries.

This epic owns:

- A VSCode-style quick-open command bar (prefix-triggered actions: go to
  file, go to line/symbol, full-text search, run a command in the current
  work root).
- Custom command buttons registrable in the sidebar/topbar per work root,
  and their human-facing registration UI.
- Keyboard shortcut registration for dashboard-level actions (e.g.
  `Ctrl+`` `` to open/focus a terminal).
- The shared dispatch mechanism these three front-ends resolve into,
  building on the existing `DashboardCommand`/`executeCommand` bus in
  `commands.ts`.

All work here is a human clicking, typing, or pressing a key inside the
browser dashboard. Nothing in this epic exposes these commands to an
agent.

## Non-Scope

- Any agent-facing/MCP execution or registration path for custom
  commands — owned by
  `260711-idea-dashboard-agent-facing-mcp-control-surface`, a child of
  `260622`, precisely because that direction requires an approval/
  authority model this epic should not decide on its own.
- Terminal control-key fidelity, visual/design-system polish, and other
  items already tracked under `260710`.
- ws session-key, ferrule, or harness-integration concerns — those stay
  with `260622`.

## Child Tickets

- `260711-idea-dashboard-command-bus-quick-open-shortcuts` - todo;
  background research (VSCode Quick Open prefix reference, existing
  `DashboardCommand` bus, existing shortcut-guard precedent) plus a
  3-phase implementation plan (frontend shell → backend `%`/`!` endpoints
  → frontend wiring) approved 2026-07-11.

## Cross-Child Decisions

- Any agent-initiated registration of a new persistent custom command,
  and any agent-initiated execution of a custom command (whether
  human- or agent-registered) through the sibling MCP control surface,
  always requires explicit human approval before taking effect. This is
  a hard policy floor set by the owner (2026-07-11), not a per-ticket
  judgment call — implementation tickets on either side of the
  UI/MCP split must not relax it.

## Completion Criteria

- Done: quick-open palette, custom command buttons, and keyboard
  shortcuts are usable end-to-end for human-driven dashboard interaction,
  dispatching through the existing command bus.
- Dropped: a different interaction model (e.g. adopting an existing
  third-party command-palette library wholesale) supersedes this
  direction before implementation starts.
- Deferred: the agent/MCP-facing half of custom commands remains
  deferred to `260622`'s child ticket regardless of how far this epic
  progresses.
