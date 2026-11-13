---
title: "Dashboard session list: four-state session lifecycle"
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
- (402) (owner, 2026-11-13): hidden sessions appear as a collapsed group with a count, refined by the owner: the current stopped-sessions slot splits into two halves, hidden sessions on the left and pending-deletion/stopped on the right. The two halves act as one tri-state radio: none expanded / hidden expanded / stopped expanded. Rejected: a filter toggle only.
- (403) (owner, 2026-11-13): a message or mail to a hidden session wakes it and it returns to active-on, like an asleep session. Rejected: staying hidden with the mail queued until unhide.
- (405) (confirmed, 2026-11-13; technical): hidden is persisted as an optional field on the session record (T0, additive; survives daemon restarts). Rejected: a display-only derived state, because deriving it in the frontend loses it on restart.

## Open Decision Queue

Last ID: (405)

(404) [open] [policy] When a pending-deletion session is actually removed: manual delete only (recommended; same as today's stopped state) vs auto-delete after a retention period with restore before then.
Owner question 2026-11-13: does existing stopped-session auto-deletion already cover this? Answer pending code check.

## Constraints

- Compatibility: any store change must be T0/T1 (AGENTS.md Compatibility Tiers, "Avoid T2"; owner 2026-11-13). No minor bump.

## Phases

### Phase 1: Four-state session list

Show the four states in the left session list with distinct indicators. Add hide/unhide and the transitions between states. Settle the Open Decision Queue before promoting to ready.
