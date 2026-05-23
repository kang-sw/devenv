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
