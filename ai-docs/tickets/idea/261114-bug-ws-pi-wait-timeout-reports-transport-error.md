---
title: "Pi wait: bounded timeout sometimes returns a transport-style context-canceled error"
---

# Pi wait: bounded timeout sometimes returns a transport-style context-canceled error

## Observed

VERIFIED during a ticket-worker run on 2026-11-14: `pi.wait` with
`timeout_s: 300` returned:

```text
pi.wait: the dashboard daemon, which runs this subagent's Pi subagents, did not answer at http://127.0.0.1:29417: Post "http://127.0.0.1:29417/agents/by-token/wait": context canceled
```

A subsequent `pi.list` succeeded and showed the same reviewer still running.
A later `pi.wait(timeout_s: 120)` reported that its completed report had already
arrived, and the normal completion report did arrive. No daemon lifecycle
operation was performed in response.

INFERRED: the wait deadline may be reaching the transport boundary instead of
returning the documented bounded `running` result. The timing/cancellation
source was not instrumented; this observation does not establish a daemon crash.

## Expected boundary

A normal bounded timeout should return the documented running status, not
suggest that the daemon is unavailable. Actual transport failure should remain
distinct. Diagnose the cancellation source before changing error handling; do
not hide genuine connectivity failures.

## Scope / verification

Investigate the wait endpoint/client deadline interaction and add a focused
regression distinguishing timeout, caller cancellation and unreachable daemon.
This idea grants no authority to restart or replace a shared daemon.
