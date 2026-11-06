---
title: sage_stamp re-stamp with the same verdicts never clears a missing-issue block
---

# sage_stamp re-stamp with the same verdicts never clears a missing-issue block

## Background

Dogfood surprise (2026-11-06, devenv lead session): after a design or
completeness block carrying `resolution: missing` issues,
`ws/tickets.sage_stamp` returns a `next_instruction` that says to fix the
autonomous issues, settle the missing ones through the Open Decision Queue,
then "call ws/tickets.sage_stamp again with the same verdicts" before
committing.

Doing exactly that (epic 261106-epic-ws-dashboard-serve-daemon, design;
261106-feat-whiteboard-rich-items, combined) recomputes the same verdict
from the same `missing` issues: the result is `verdict: block` again, the
posture stays `blocked`, the Blocked section is re-rendered, and the
`next_instruction` repeats word for word. Only fresh reviewer verdicts can
clear the block, which the same message states only in its last sentence.
A lead following the instruction loops, or commits a ticket it believes was
cleared.

Open for design: the second stamp could accept a "missing issues settled"
marker (for example the settled queue IDs) and clear to the reviewed
verdict's non-missing outcome, or the instruction could say plainly that a
`missing` block needs a delta review after settlement, with no same-verdict
re-stamp step.
