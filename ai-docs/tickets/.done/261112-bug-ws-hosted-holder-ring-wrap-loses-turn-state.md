---
title: "Hosted sessions: a wrapped 4 MiB holder ring loses turn state on rejoin"
related:
  261110-feat-ws-dashboard-headless-lead-sessions: hosted lead sessions, daemon rejoin
completed: 2026-11-12
---

# Hosted sessions: a wrapped 4 MiB holder ring loses turn state on rejoin

## Background

Review sweep `00fd2e5d9..62304312b` (ledger 940fc053e), finding P4, routed by
the peer session; traced in code on 2026-11-12.

- `holder.py:123` `RING_BYTES = 4 << 20`; `Ring.append` (182-205) drops the
  oldest records past it. `cmd/ws-mcp/dashboard.go:86` builds the holder
  manager without `RingBytes`, and `wsholder/manager.go:557` passes
  `--ring-bytes` only when set. The same manager serves dashboard terminals.
- On a daemon rejoin the replay starts at `first_seq`. If the current turn's
  start (`init` / `session_state_changed running`) left the ring, `c.turn`
  reads idle, `replayDone` -> `kick` submits a held message into a running
  turn (Pi's RPC may reject a prompt while streaming), records between
  `HolderSeq+1` and `first_seq-1` never reach the dashboard ring, and
  `holderChild.event` ignores the `gap` event, so a line tail can splice onto a
  stale partial line.
- Trigger: one turn producing more than 4 MiB (long agentic turns echoing big
  tool results), or more than 4 MiB of output while no daemon is attached. The
  hosted context meter's `get_context_usage` answer (~29 KB, once per model)
  is a negligible share.

## Decisions

- Lead holders get a larger ring through a per-launch ring size (a
  `RingBytes` field on the launch call, passed as the existing
  `--ring-bytes`; 32-64 MiB, filled lazily). Internal Go API only, T0.
  User board (28).
  Rejected: raising `Manager.RingBytes`; it grows every dashboard terminal's
  ring too.
- When the ring still wraps, the rejoin handles it: `holderChild.event` drops
  `partial` on `gap`, and when `FirstSeq > HolderSeq+1` or a gap arrives
  during the replay with no turn boundary in the window, the conversation
  treats the turn as running and submits nothing until the harness's next
  idle signal (Claude `session_state_changed idle` / `result`, Pi
  `agent_end`), logging the loss. User board (29).
  Rejected: trusting the replayed window; it misreads a running turn as idle.

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`.
- Heavy Go builds and tests run through `~/.local/bin/ws-rgo` on this machine.


## Resolution (2026-11-12)

Fixed in 1482a4b86 (merge 91be5e087): lead holders launch with a 32 MiB ring through wsholder LaunchSpec/LaunchWith (terminals keep the default); holderChild drops a partial line on gap; a rejoin that lost records with no turn boundary after the loss holds queued messages, mail, and steers until the harness's next idle signal, logging the loss once. Known limit: an actually idle harness whose ring kept no boundary shows running until an idle signal or a stop.
