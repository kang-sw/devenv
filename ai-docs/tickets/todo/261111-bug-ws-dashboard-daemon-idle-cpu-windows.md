---
title: "Dashboard daemon holds about 15% of a core while idle on Windows"
related:
  261111-feat-ws-mailbox-wait-without-process-relaunch: "sibling from the same CPU probe (board (288) b); that ticket covers the per-wake process cost, this one the daemon's own CPU"
---

# Dashboard daemon idle CPU on Windows

## Background

User, 2026-11-11: the Windows machine showed sustained CPU use and asked
whether ws was the cause. A delegated probe (board topic `cpu-windows`)
measured, on native Windows:

- `ws-mcp-dashboard-*.exe`: about 15% of one core in a 10 s sample, 10.6%
  averaged over its 7 h lifetime. It hosts every MCP session and mailbox
  wait, so the CPU that per-session executables used to spend is now
  concentrated here; whether 15% idle is that sum or a hot loop is unknown.
- The ws Python launchers (35 processes): about 0% CPU, about 30 MB each;
  no busy loop in their code (blocking readline / WaitForSingleObject).
- `D:/sys_lock_remote.py` at about 20% is the user's own tool, outside ws.

Candidate periodic work in the daemon, unmeasured: the mailbox wait
backstop (10 s) and nudge (1 s), the session index (30 s), the tracker
watch (10 s), and SSE keepalives (15 s).

User decision (board (288) b): profile the daemon and fix the hot path, as
its own ticket beside the mailbox-wait relaunch ticket.

## Open Questions

Settle before promotion to `ready/`:

- How the profile is taken on Windows (a pprof endpoint behind a flag, an
  env-gated CPU profile file, or another route), and whether that hook
  ships or stays a dev build.
- The acceptance bound for idle daemon CPU, and with how many hosted
  sessions it is measured.

## Constraints

- Heavy Go builds run through `~/.local/bin/ws-rgo`; the profile itself is
  taken on the user's Windows machine
  (`ai-docs/manuals/windows-dogfood.md`).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`.

## Phases

### Phase 1: profile the idle daemon and fix the hot path

Take a CPU profile of an idle daemon on Windows with the current session
count, name the hot paths, and fix those that are waste (polling that an
event could replace, a timer that runs with nothing to do). Report the
before and after idle CPU.
