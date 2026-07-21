---
title: Represent main-session work in WorkRoot Activity
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-activity-console-tail-ribbon-polish: recent Activity Console polish that exposed stale-looking latest activity
spec:
  - 260521-ws-dashboard-activity-console-read-model
  - 260521-ws-dashboard-activity-console-ui-shell
related-mental-model:
  - ws-web-dashboard
---

# Represent main-session work in WorkRoot Activity

## Background

Dogfood after Activity Console polish showed a misleading freshness gap. The
WorkRoot Activity pane still showed `mental-model-updater` from
`2026-05-23T02:22:16Z` as the latest activity even though the current Codex
session had continued implementing and committed dashboard work around
`2026-05-23T04:29Z`.

The current projection is accurate for ws named-agent cache state: `implementer`,
`reviewer-*`, and `mental-model-updater` are named-agent rows, and
`mental-model-updater` is the latest completed named-agent call. However,
main-session actions such as direct edits, tests, git commits, MCP doc updates,
and local shell investigation are invisible because they are not recorded as a
source-neutral Activity source.

## Follow-Up Questions

- Should the dashboard expose a `session.codex` or `cmd.main` source for the
  current host session's command-routed work?
- Should git commits, ticket moves, and test/build commands appear as activity
  items even when no ws named-agent was spawned?
- Where should the durable event log live so it remains host-neutral and still
  supports future tmux-like command binding?
- How should the UI distinguish named-agent rows from main-session command
  history so the latest row does not look stale during direct implementation?

## Staleness audit (2026-06-19)

The Background frames the Activity source as a named-agent cache projection. That
model changed under the epic 260605 ephemeral session-auth reshape (Option B): the
SQLite actor/named-agent registry was removed and the mercenary lifecycle is now
the activity source. The underlying question (should main-session work appear in
Activity) is still open, but it is now a **port-vs-remove deferred product
decision** under epic 260605, not a straightforward forward-design item.
Re-ground the follow-up questions on the mercenary lifecycle before promoting.

## Ticket closure

Superseded by `260620-feat-ws-dashboard-agent-client-activity-sources`
(2026-07-21 housekeeping sweep). That ticket's own `related` frontmatter
explicitly names this ticket as "prior main-session freshness gap that must
be re-grounded on host-owned agent/client activity" and its Background makes
the same re-grounding this ticket's staleness audit called for: after the
260605 pivot removed the SQLite named-agent registry, native Codex/OpenCode/
Claude work is treated as host-owned agent-client data and projected through
the dashboard's source-neutral Activity model (`AgentClientProvider`
contract, `items`-based Activity source split) rather than the old
named-agent cache assumption this ticket's Background was written against.
This ticket's parent epic `260514-epic-ws-web-dashboard-mvp` is already
closed (`.done/`). Dropped rather than promoted; no distinct need remains
that `260620` does not already own.
