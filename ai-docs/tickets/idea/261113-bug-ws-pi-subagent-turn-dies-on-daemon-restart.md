---
title: "A pi.spawn subagent's running turn dies when the dashboard daemon restarts"
---

# A pi.spawn subagent's running turn dies when the dashboard daemon restarts

## Background

Dogfood 2026-11-13: the lead ran `python scripts/dev.py rerun daemon` while
four `pi.spawn` subagents were mid-turn. Every one of them got a completion
mail "turn interrupted: the dashboard daemon exited while the turn ran"; the
work in progress was lost and each had to be re-sent with `pi.send`.

Hosted leads survive the same restart (their process runs under a holder and
the new daemon rejoins it). `pi.spawn` subagents apparently do not: the daemon
owns their process directly.

## Idea

- Run a `pi.spawn` subagent's process under a holder the way hosted leads run,
  so a daemon restart rejoins its running turn instead of killing it; or, at
  least, have a restart refuse or warn while subagent turns are running
  (`dev.py rerun daemon` and the release update path alike).
- Until then, the lead's dev procedure: do not rerun the daemon while
  `pi.list` shows a running subagent.

## Open

- Whether the holder path fits the Pi RPC mode used for subagents.
- Compatibility tier at promotion (likely T0: process supervision only).
