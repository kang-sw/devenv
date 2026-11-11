---
title: "Tracker: a CC field so related members keep tracking an item without an assignment"
related:
  261111-feat-ws-tracker-assignee-edit-and-body-merge: "origin: its (235) a chose a CC field over relaxing assign; the CC field is edited by whoever may edit the item, which that ticket widens to the live assignee"
---

# Tracker: CC field

## Background

User, 2026-11-11 board (topic `tracker-edit`, note on (229)): should assign
relax to the assignee, or should related people get a separate CC field so
the item's tracking is visible to them, or should assignments be tiered
("assignee | CC")? Answer (235) a; placement as its own ticket (236).

Current state (read 2026-11-11, `agents-plugin-tool/internal/wstracker/`):
assign is creator-only; in "chosen" mode members below the creator are live
at once and others pending (accept or decline). Body mentions notify once
and keep nothing in the mentioned member's view. There is no watcher or CC
concept.

## Decisions

1. **A CC item field; assign stays creator-only ((235) a).** CC is an item
   field like `project`, edited by whoever may edit the item. CC'd members
   see the item in a CC list on the MY view and get change notifications.
   A CC entry has no status, accept, or completion.
   Rejected: letting the live assignee assign ((235) b), since delegation
   already works through subtasks the assignee creates and assigns, each
   with its own status; an "assignee | CC" assignment tier ((235) c), which
   would have to define accept, decline, and done for a watcher; both a
   and b ((235) d).
2. **Its own ticket ((236)).** CC touches view lists and notifications and
   shares no code with the edit-permission and body-merge work.
   Rejected: a Phase 2 of `261111-feat-ws-tracker-assignee-edit-and-body-merge`.

## Open Questions

Settle before promotion to `ready/`:

- Who may be CC'd (any member, or limited by the org tree), and whether a
  member may add or remove themselves.
- Which changes notify a CC'd member (status, comments, body edits,
  assignment changes), and whether that reuses the assignee notification
  kinds.
- Where the CC list sits in the MY view, and whether CC shows on the item
  row.
- Whether `tracker.query` gains a `cc:` filter.

## Constraints

- Mirrored in the reference engine (`ai-docs/ref/tracker-mockup/engine.js`)
  and its golden cases, and documented in the tracker playbook of all three
  packages, as the existing event types are ((210)).
- Quiet dashboard tone: `--wb-*` tokens.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/ws-mcp.md`.

## Phases

### Phase 1: CC field

Scope: the CC item field, its write rule, the MY view CC list, and its
notifications, as the open questions settle.

Verification: settled with the open questions; at least tracker tests that
a CC'd member sees the item and is notified, that CC grants no status or
completion, and golden cases agreeing with the Go domain.
