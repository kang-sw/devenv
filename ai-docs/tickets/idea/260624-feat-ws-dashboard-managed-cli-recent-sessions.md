---
title: ws dashboard managed CLI recent sessions
parent: 260622-epic-ws-dashboard-session-key-realignment
related:
  260624-feat-ws-dashboard-managed-cli-terminal: future follow-up after the first managed CLI surface is dogfoodable
  260620-feat-ws-dashboard-agent-client-activity-sources: adjacent but separate structured provider/activity adapter track
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard managed CLI recent sessions

## Background

The first managed CLI terminal should close by terminating the vendor CLI
process and should not try to preserve daemon-owned PTYs across daemon restarts.
Longer term, the dashboard can provide a separate "recent sessions" surface by
parsing vendor-owned history/transcript stores for Codex, Claude, OpenCode, or
similar CLIs.

This is idea-level follow-up, not scope for
`260624-feat-ws-dashboard-managed-cli-terminal`. The first managed CLI
implementation may show a restore prompt for a remembered pane after daemon
restart, but it should not implement vendor history parsing or a flat recent
session browser.

## Direction

Recent sessions should appear as a flat list that distinguishes:

- which workspace/workRoot or Git root the conversation came from;
- which worktree held the vendor history record;
- when the conversation happened;
- which vendor/profile produced it;
- a vendor-derived conversation title or bounded fallback label.

Vendor history remains vendor-owned. The dashboard may parse known local history
formats for display and reopen affordances, but it must keep provider-native
session ids, cache paths, transcript paths, and ws session keys out of browser
route identity and ordinary command logs.

The list should be visually available from one place even when underlying vendor
history is stored separately by Git root and worktree. Any future implementation
needs explicit parser fixtures per vendor before promising restore or reopen
behavior.
