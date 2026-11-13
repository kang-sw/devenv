---
title: ""
related:
  261113-bug-ws-dashboard-sidebar-shows-processless-session-as-live: the "active, off (asleep)" state is what that bug's sessions should display
---

# Dashboard session list: four-state session lifecycle

## Background

Owner request (2026-11-13). The dashboard's left session list currently cannot tell a session with a running process from a session with none, and it has no way to put a session out of sight without stopping it for deletion. The owner wants each session in the list to be in one of four states:

1. Active, on - the session's process is running.
2. Active, off (asleep) - no process; still listed normally; starts on the next message or mail (today's behavior when Stop is disabled).
3. Inactive, off (hidden) - no process; hidden from the default list.
4. Pending deletion - today's "stopped" state.

Recorded as todo for later; the UI track (dashboard web frontend owner) is the expected implementer.

## Decisions

- The four states above, as the owner stated them (2026-11-13).

## Open Decision Queue

Last ID: (405)

(402) [open] [policy] How hidden sessions appear in the list: a collapsed "Hidden" group at the bottom with a count (recommended) vs only behind a filter toggle.
(403) [open] [policy] A message or mail to a hidden session: it stays hidden and the mail queues until the user unhides it (recommended) vs it wakes and returns to active-on like an asleep session.
(404) [open] [policy] When a pending-deletion session is actually removed: manual delete only (recommended; same as today's stopped state) vs auto-delete after a retention period with restore before then.
(405) [open] [technical] Hidden is persisted as an optional field on the session record (T0, additive; survives daemon restarts) over a display-only derived state - Rejected: deriving it in the frontend loses it on restart.

## Constraints

- Compatibility: any store change must be T0/T1 (AGENTS.md Compatibility Tiers, "Avoid T2"; owner 2026-11-13). No minor bump.

## Phases

### Phase 1: Four-state session list

Show the four states in the left session list with distinct indicators. Add hide/unhide and the transitions between states. Settle the Open Decision Queue before promoting to ready.
