---
title: "Pi nested subagent terminal delivery can leave the parent waiting indefinitely"
---

# Pi nested subagent terminal delivery can leave the parent waiting indefinitely

## Background

During the `260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter`
dogfood run, the nested `nested-picker` agent completed Phase 3, committed
`33630931`, reported a clean worktree, and emitted its final response at
2026-09-20T12:05:22Z. Its direct parent `subtree-gutter` nevertheless remained
in `waiting-on-children` and did not consume the terminal delivery until the
lead inspected the descendant transcript and prompted the parent explicitly.

This is distinct from an earlier hung test process in the same run: that process
was terminated and the child was successfully resumed before it later completed.
The delivery stall occurred after the recovered child had produced its terminal
result.

Expected behavior: when a nested child settles, its direct parent receives the
terminal delivery without external prompting and can leave
`waiting-on-children` once its subtree is quiescent.

## Decisions

- Recursive subagent terminal messages must not be lost. Delivery must reach the
  direct parent and be consumed without an unrelated external prompt, including
  after a child stop/resume recovery.
- Do not solve delivery reliability by imposing a lifetime timeout on
  subagents. Agents remain unbounded; this ticket restores lossless delivery and
  parent wake-up behavior.

## Open Questions

- Did the terminal result reach the parent's delivery queue but fail to wake it,
  or was the result never admitted to that queue?
- Does the failure require a child that was stopped and resumed, or can it occur
  on an uninterrupted nested run?
- Which lifecycle state, persisted channel, and wake-up edge should be observed
  to distinguish delivery admission from parent resumption?

## Phases

### Phase 1: Reproduce and restore nested terminal delivery

Establish a deterministic nested-agent reproduction, identify the lost or
unobserved delivery/wake-up transition, and restore automatic parent progress
without weakening subtree-settle safeguards.

Verification must cover a grandchild settling under a waiting parent, including
stop/resume recovery if that is part of the trigger, and assert that the parent
receives the terminal result and leaves `waiting-on-children` without an
external message.
