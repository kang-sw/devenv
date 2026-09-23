---
title: ws named agent fails opaquely on context exhaustion
related:
  260609-refactor-ws-spawn-runtime-deletion-session-auth: surfaced during M3 Phase 2a survey delegation
---

# ws named agent fails opaquely on context exhaustion

## Background

During M3 Phase 2a a delegated `plan-populator-survey` agent (claude/sonnet,
200k window) was asked to map a large source surface. It read ~1.5M cache-read
tokens over 39 turns and hit the model context limit. Two problems surfaced:

## Opaque failure surfacing

The MCP/`agents.result` error was only:

```
backend invocation failed
raw_error: claude failed: exit status 1
```

The actual cause — `"result":"Prompt is too long","terminal_reason":"blocking_limit"`
— was buried in the agent's raw stdout and only discovered via
`agents.debug.tail`. The lead-facing failure should surface the real terminal
reason (context/prompt-too-long, rate limit, auth, etc.) instead of a generic
`exit status 1`, so a caller can react (split the task, raise the budget, switch
backend) without digging into debug streams.

## No context-budget guard / partial-result path

The agent ran to hard context exhaustion and returned nothing usable — no partial
plan, no early "approaching limit, narrow the task" escalation. A long read/survey
task that overflows the window is a total loss. Options to investigate: a
soft-budget self-check prompt convention, an early-escalation contract, or
returning the partial artifact written so far.

## Notes

- Worked around in Phase 2a by having the lead author the survey plan from
  targeted greps; not a blocker, but it defeats the named-agent-delegation-first
  goal for large survey tasks.
- Reference-discovery (smaller prompt, same backend) succeeded in the same
  session, so the backend itself was healthy — this is specifically a
  large-context-task failure mode.
- Triage: decide whether the surfacing fix and the budget-guard are one ticket or
  split; coordinate with the mercenary reshape (M3 Phase 2c) since runner error
  diagnostics live on the shared manager path.
