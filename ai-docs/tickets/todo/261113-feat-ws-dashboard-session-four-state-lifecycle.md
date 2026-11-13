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
- (404) The pending-deletion state is the existing age-based registry prune window: PruneRegistrations (agents-plugin-tool/internal/wsdashboard/registry.go around lines 316-373) drops a session row whose LastSeen (max of registered, heartbeat, ended) is older than the RegistryRetention constant of 7 days; it runs once per ws-mcp process start (agents-plugin-tool/internal/mcp/dashboard_sessions.go around lines 246-252). The pending-deletion row shows that window and stays recoverable until the prune removes it. Rejected: (a) manual-delete only - it would disable the existing automatic prune.
- (405) (confirmed, 2026-11-13; technical): hidden is persisted as an optional field on the session record (T0, additive; survives daemon restarts). Rejected: a display-only derived state, because deriving it in the frontend loses it on restart.

## Constraints

- Compatibility: any store change must be T0/T1 (AGENTS.md Compatibility Tiers, "Avoid T2"; owner 2026-11-13). No minor bump.
- The prune decides on age alone, so an idle-but-alive session silent for more than 7 days may be pruned (unverified); revisit if the four-state work depends on it.

## Phases

### Phase 1: Four-state session list

Show the four states in the left session list with distinct indicators. Add hide/unhide and the transitions between states. Settle the Open Decision Queue before promoting to ready.
