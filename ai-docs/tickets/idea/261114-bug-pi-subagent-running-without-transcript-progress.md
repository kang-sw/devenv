---
title: Pi subagent remains running without flushed progress or status replies
---

# Pi subagent remains running without flushed progress or status replies

## Background

During Windows re-pass dogfooding on 2026-11-14, persistent worker
`windows696-repass` (`22cfcc17-1e7a-4c6b-92f4-6300cf6321c8`) remained
`running` in `ws-agent-list`, with last report at 11:46:51Z. At approximately
12:26Z, its session transcript still ended with four completed file-read
results at 11:47:04Z. No later tool call, progress report, terminal result,
or answer to subsequent status/throttle steers was flushed there.

Evidence: the worker registry and the last five JSONL records in its Pi
session transcript were read by lead session `boxer-nest-joyous`. The reads
concerned prior verification material and smoke/installer scripts; this is
not evidence that a native Windows command was running. The reason for the
silence is unproved: model latency, transport state, and adapter bookkeeping
remain possible explanations, not findings.

The lead stopped and resumed only this same Pi child RPC, reissuing a
status-only instruction and the existing native-execution throttle. No
Windows-host or shared-daemon restart was requested. Whether that recovery
restores responsiveness remains to be observed.

## Constraints

- Preserve the worker session, pending status instructions and ownership of
  external resources. Do not duplicate native execution during recovery.
- Distinguish a reported running state from tool execution, model waiting,
  pending delivery and a stale RPC connection before selecting a fix.
- No watchdog threshold, forced termination policy, provider diagnosis or
  harness change is approved by this observation.

## Phases

### Phase 1: Explain the silent-running interval and recovery behavior

Correlate worker transcript flushes, RPC/model request state and steer delivery
for the recorded interval. Determine whether the running state was accurate
and why no status result surfaced. Record whether same-session stop/resume
recovers it without replaying external work. Return a minimal observability or
recovery proposal only if evidence identifies a follow-up; do not infer a
product defect solely from elapsed time. Any implementation requires normal
scope and approval gates.
