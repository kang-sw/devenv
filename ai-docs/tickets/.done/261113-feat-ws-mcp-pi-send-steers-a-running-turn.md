---
title: "pi.send can steer a running Pi subagent turn instead of waiting for it to end"
related:
  261112-feat-pi-subagent-long-shell-auto-background: complementary - a steer still waits behind one long blocking tool call; that ticket frees the turn from it
completed: 2026-11-13
---

# pi.send can steer a running Pi subagent turn instead of waiting for it to end

## Background

Owner feedback 2026-11-13. `ws/pi.send` to a daemon-run Pi subagent
(`pi.spawn`) that is mid-turn queues the message until the turn ends and
delivers it as the next turn (tool description: "during a turn it waits for
that turn to end and then goes as the next turn"). A ticket worker runs its
whole ticket - implement, verify, review, close - in one turn, so the lead
cannot steer it at all until it is done: e.g. the lead could not ask the
single-exe worker to stop after Phase 1 so its V3-warning fix could merge
early, nor redirect a worker that stopped on a wrong session key until it had
ended its turn.

The hosted Pi harness already delivers mid-turn messages as steers at tool
boundaries (done: 260923-feat-ws-pi-agent-send-steer-default,
260914-feat-ws-pi-mailbox-native-steer-push), so the capability exists on the
Pi side; the daemon `pi.send` path does not use it.

## Idea

- `pi.send` gains a delivery mode: `steer` (inject at the next tool boundary
  of the running turn) vs the current follow-up (next turn). An additive
  optional argument is T0; making steer the default changes observable
  behaviour (ask first).
- The completion mail and `pi.list` show whether a held message was steered
  or is queued.

## Open

- Whether the daemon's Pi RPC channel exposes a steer command for a running
  turn or the runner must add one.
- Default mode, and whether the lead's playbooks should steer by default.


## Resolution (2026-11-13)

Delivered as an owner-approved delegated hotfix: 7723acc96, merged into develop at 6ccbbbbf1. A pi.send during a running daemon Pi turn now steers that turn at the next tool boundary (steer is the behaviour, not an opt-in); idle delivery is unchanged and no extra turn follows. Live installed-Pi delivery awaits the next daemon rerun.
