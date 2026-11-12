---
title: Auto-collapse the dashboard agents panel to its header
related:
  261112-feat-ws-dashboard-lead-wake-events: its background/monitor badges are what a collapsed header would show
---

# Auto-collapse the dashboard agents panel to its header

## Background

User request (2026-11-12, board topic `conv-events` item 4, deferred by
board 349 a as a long-term item): the always-visible agents panel at the top
left of the session view (`#r-tree`, region `tree`, `dashboard.tmpl:124`)
should rest collapsed to its header. Clicking the header, or the subagent /
background badges, expands it; moving focus to another card collapses it
again.

Today the panel has no whole-panel collapse: only per-agent `<details>` rows
(`openState`/`restoreOpen`, `dashboard.js:100-131`) and the splitter
(`dashboard.js:239-297`, `--sa`, 56px floor) exist.

## Open Questions

- What counts as "focus moved to another card" (pointer down elsewhere,
  keyboard focus, or the composer gaining focus)?
- Does the collapsed state persist across reloads, and how does it interact
  with a user-dragged splitter position?
- In the tabs layout (narrow widths), does the panel collapse the same way
  or stay a tab?
