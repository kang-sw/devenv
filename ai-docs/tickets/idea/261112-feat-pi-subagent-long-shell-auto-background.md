---
title: "Pi subagents: a blocking shell command moves to a background shell after 30 seconds"
related:
  260920-bug-pi-tool-call-timeout-and-process-cleanup: neighbouring problem (bounded tool calls and subprocess cleanup); this ticket keeps a long command running instead of bounding it
---

# Pi subagents: a blocking shell command moves to a background shell after 30 seconds

## Background

User feedback (whiteboard, 2026-11-12), recorded as feedback only; nothing
here is a settled decision yet.

In the Pi adapter, a subagent running a long blocking shell command cannot
take a message the lead sends it until the command finishes, so steering a
subagent mid-task often stalls behind a long command. The user asked that
every blocking shell command in a Pi subagent switch to a background shell
after 30 seconds, so the subagent's turn frees up and the message gets
through.

## Open Questions

- Whether Pi exposes a background-shell facility the adapter can hand a
  running command to, or the adapter must own the process and return a
  handle (output file, status query) to the subagent.
- How the subagent learns the command finished (a wake, a poll tool).
- Whether the 30-second threshold is configurable, and whether it applies to
  the lead as well as subagents.

## Phases

### Phase 1: Auto-background long shell commands in Pi subagents

To be settled at capture through the Open Decision Queue.
