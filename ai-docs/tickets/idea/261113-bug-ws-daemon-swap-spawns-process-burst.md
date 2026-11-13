---
title: "Daemon swap spawns a burst of short-lived dashboard processes"
---

# Daemon swap spawns a burst of short-lived dashboard processes

Related: `261113-bug-ws-launcher-dev-rebuild-replaces-daemon-mid-work` (the
rebuild-replaces-daemon trigger; this ticket is the process burst that
replacement produces).

## Background

Owner report, Windows, 2026-11-13: when the dev dashboard daemon is replaced
after a rebuild, Task Manager shows close to a few hundred processes named
`ws-mcp-dashboard-1498562acc85ccdc.exe` appearing and disappearing until one
remains. Question: what mechanism causes the burst?

## Mechanism

The hashed `ws-mcp-dashboard-<hash16>.exe` is the daemon's staged copy of
`ws-mcp` (`stage_daemon_binary`, `agents-plugin/bin/ws-mcp-launcher.py:937`;
Go twin `wsdashboard.StageExecutable`, `internal/wsdashboard/spawn.go:51`). The
launcher launches exactly two kinds of process from it, both short-lived
except the winner:

1. `daemon-start --build-time N` (`start_daemon`, launcher ~1370-1418;
   `daemonStart`, `cmd/ws-mcp/proxy.go:357`). It runs `wsdashboard.Ensure`.
2. Up to `maxSpawns = 2` detached `dashboard --port ...` daemon candidates per
   `Ensure` (`internal/wsdashboard/ensure.go:38,155`). A candidate waits up to
   `DefaultLockWait = 5s` for the single-instance flock
   (`internal/wsdashboard/daemon.go:33,293-302`) and exits with `ErrLocked`
   when it loses. Exactly one candidate keeps the lock and becomes the daemon.

Fan-out: every running MCP session (each Claude/Codex session, each Pi
subagent's ws-mcp) has its own launcher process holding a stdio proxy. When the
old daemon shuts down (replacement via `RequestShutdown`, `ensure.go:140`),
every proxy sees its stream close and runs `recover()` (launcher ~1627), which
calls `link.connect(hello, grace=1.5)`. That waits only 1.5s for a healthy
daemon (`wait_healthy`, ~1331); the handoff (3s shutdown deadline, lock wait,
bind, endpoint write) usually takes longer. With `health is None` and
`allow_start` true, `connect` (~1420-1440) calls `start_daemon()` for each
proxy independently. Nothing coordinates the launchers: each does
repair/stage (the binary path is shared, so all stage the same new hash, which
is why one name is seen), then runs its own `daemon-start`, which spawns its own
candidate(s). Hook/CLI invocations (`proxy_cli`) use the same `connect` and can
join the herd.

Result: roughly N sessions x (1 `daemon-start` + 1-2 candidates), each living
from sub-second to ~5-15s (lock wait + `waitForSuccessor`, `ensure.go:188`),
until one daemon wins and the rest see a healthy successor (`OutcomeRunning`)
or lose the lock. That matches "a few hundred" at tens of live sessions.

The `Ensure` spawn cap and the lock keep it bounded and keep one daemon: it is
a thundering herd, not an unbounded respawn loop.

## Verified vs inferred

Verified (source read): the staged-name origin; the two process kinds and what
each does; the `maxSpawns`/lock-wait bounds; `recover()` -> `connect(grace=1.5)`
-> `start_daemon()` per launcher with no cross-launcher guard; the wsflow
launcher mirror is byte-identical to the ws one; session holders are Python
(`holder.py`), not this exe, so they are not part of the burst. Steady state
observed once (pid 26744, parent 35412): a single
`ws-mcp-dashboard-24c4db9f87966444.exe dashboard --port 29417 --build-time ...`
and nothing else, so hashed-name processes are the daemon plus transient
starters only.

Inferred, not observed: the exact timing (that 1.5s grace routinely expires
before the successor is healthy), the exact per-session count, and that no
other starter (hooks) materially adds. A swap was not triggered; process
creation/exit was not sampled. Verify by sampling `Win32_Process` (name, pid,
ppid, CommandLine, CreationDate) at 50ms during a swap and grouping by
command (`daemon-start` vs `dashboard`) and parent.

## Impact

CPU and disk churn at swap time (each start hashes the ~tens-of-MB exe, and
staging may copy it), AV on-access scans of every new process (the likely
cause of Task Manager lag/flash), and wider windows of "daemon not reachable"
for tool calls (compounds the mid-work replacement ticket). Race risk: the
flock keeps a single daemon, and `Ensure` leaves a daemon that refuses
shutdown alone, so no duplicate-daemon state is expected; losers can still
briefly contend for the port/endpoint file during the handoff (inferred).

## Idea (non-binding notes, no decisions)

- Single-flight the start across launchers: a start lock (flock in the
  daemon dir) taken before `start_daemon`; losers skip to `wait_healthy`.
- Lengthen/jitter the reconnect grace in `recover()` so a proxy waits for the
  successor (bounded to the daemon start budget) instead of starting at 1.5s;
  only the launcher that triggered the replacement starts one.
- Have `Ensure`/`daemon-start` bail early when an endpoint with a newer token
  appears, before spawning a candidate.

Compatibility if pursued: launcher-local, T0.
