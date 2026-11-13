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

Peer report 2026-11-13 (InspectTGV, wsflow 0.49.0):

- Another lead's `python scripts/dev.py rerun daemon` exited the machine-wide
  daemon twice (02:11:28Z and 02:14:41Z), interrupting a Pi worker and its
  nested leaf in this project. Two ws-mcp-0.49.0 builds and two dashboard
  processes were seen at once; the MCP server reconnected several times.
- After exit, `pi.list` still showed the dead worker as running, `pi.wait`
  returned "turn interrupted", and `pi.send` queued forever waiting for the
  running turn to end. `pi.stop` and respawn were required.

## Idea

- Run a `pi.spawn` subagent's process under a holder the way hosted leads run,
  so a daemon restart rejoins its running turn instead of killing it; or, at
  least, have a restart refuse or warn while subagent turns are running
  (`dev.py rerun daemon` and the release update path alike). Check running
  turns across all sessions: the daemon is machine-wide, so a rerun can kill
  other projects' workers too.
- After daemon restart, mark interrupted Pi records interrupted/idle so
  `pi.send` can start a new turn instead of queuing indefinitely.
- Until then, avoid rerunning the daemon while any session has a running
  subagent; stale `pi.list` state may require stopping and respawning it.

## Open

- Whether the holder path fits the Pi RPC mode used for subagents, and how
  to reliably detect running turns across sessions before a restart.
- Compatibility tier at promotion (including stale-state recovery and the
  cross-session restart guard; process supervision alone may be T0).
