---
title: "Mailbox wait without launching a new process on every wake"
related:
  261111-bug-ws-dashboard-daemon-idle-cpu-windows: "sibling from the same CPU probe (board (288) b)"
---

# Mailbox wait without a process relaunch per wake

## Background

The 2026-11-11 Windows CPU probe (board topic `cpu-windows`) found that each
armed `mailbox wait` starts a new launcher process on every wake and on
every 10-minute timeout. Each start costs 0.3-0.9 s of CPU and about
30 MB; 13 such waits were live during the probe. Unlike the daemon's own
idle CPU (`261111-bug-ws-dashboard-daemon-idle-cpu-windows`), the cause is
known.

User decision (board (288) b): reduce this cost in its own ticket.

## Open Questions

Settle before promotion to `ready/`:

- The mechanism: wait on the session's already-running MCP connection,
  keep one long-lived waiter process, lengthen the timeout, or a mix.
- Which harnesses the change covers (the wait is armed differently per
  host), and whether the current re-arm contract the lead playbooks
  describe changes.

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`, and `ai-docs/manuals/skill-authoring.md` /
  `ai-docs/manuals/wsflow-mirroring.md` if the mailbox playbook text
  changes.

## Phases

### Phase 1: mailbox wait without a per-wake relaunch

Replace the per-wake process start with the mechanism settled above, and
measure the per-wake CPU and memory before and after on Windows.
