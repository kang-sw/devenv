---
title: project_tree stale ticket status map
related:
  260514-epic-ws-web-dashboard-mvp: dashboard discussion exposed stale active ticket projection
---

# project_tree stale ticket status map

## Background

During a dashboard discussion on 2026-05-24, `ws/project_tree()` showed several
dashboard child tickets as active or ready even though exact
`ws/tickets.find(ticket_stem: ..., include_done: true)` lookups reported them
under `.done/`.

This makes the project map look like the current active queue when it may be
project-index-derived or otherwise stale. Future work should clarify whether
`project_tree` is expected to render raw project memory, live ticket filesystem
state, or both. If it intentionally includes project-memory inventory, it should
surface staleness clearly enough that callers do not route implementation from
completed tickets.
