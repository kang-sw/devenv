---
title: "Pi tool calls need bounded execution and subprocess-tree cleanup"
---

# Pi tool calls need bounded execution and subprocess-tree cleanup

## Background

During the `260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter`
dogfood run, the nested `nested-picker` agent invoked
`node --test test/audit.test.ts` through a Bash tool call. The tool call never
returned, the test process survived with PID 86310 and PPID 1, and the agent
produced no further transcript activity for approximately three hours and forty
minutes. Recovery required stopping the agent, terminating the orphan process
with SIGTERM, verifying that no matching process remained, and resuming the
agent with an externally imposed 60-second command bound.

This is separate from the later recursive terminal-message delivery stall: this
ticket concerns bounded tool execution and process cleanup, not agent-message
transport.

## Decisions

- Add a caller-selectable timeout parameter to the applicable tool-call path so
  long-running commands can be bounded explicitly instead of waiting forever.
- Timeout or cancellation must clean up the spawned subprocess tree rather than
  leave detached or PPID-1 descendants behind, and must return a distinguishable
  timeout/cancellation result to the caller.
- Do not impose a lifetime timeout on subagents. Agent lifetime remains
  independent; only the individual tool invocation is bounded.

## Open Questions

- Which Pi tool-call surface owns the timeout parameter, and what name, unit,
  maximum, and default preserve compatibility?
- What process-group or job-object strategy provides reliable descendant cleanup
  on macOS/Linux and Windows?
- Which stop, disconnect, and transport-failure paths must share the same cleanup
  primitive as explicit timeout?

## Phases

### Phase 1: Bound tool calls and reap their subprocess trees

Reproduce the orphaned command, settle the timeout API and cleanup ownership,
and implement bounded invocation without adding an agent lifetime deadline.

Verification must exercise a command that spawns a descendant and exceeds its
configured timeout, assert a distinguishable timeout result, and prove that
neither the direct process nor its descendants remain. Cover explicit agent/tool
cancellation through the same cleanup boundary and retain ordinary unbounded
behavior when the caller does not request a timeout, unless later compatibility
evidence settles a safe default.
