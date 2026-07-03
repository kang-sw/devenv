---
name: lead-drain-ready-queue
description: Pick the next ready/ ticket and hand it to lead-proceed. Stop if ready/ is empty.
---

# Drain Ready Queue

Pick the next ticket in `ai-docs/tickets/ready/`: prefer one named as a
prerequisite via another ready ticket's `related:`/`parent:` frontmatter,
otherwise take the oldest (FIFO). If `ready/` is empty, report that and stop.
Otherwise hand off to `lead-proceed` with the selected ticket's path as an
explicit target — never call it bare.

Conserve lead context for the long-running goal this serves: never read or
write files or run commands yourself. Delegate everything — including simple
tasks like commits — to an appropriately tiered subagent or forked subagent,
following `lead-prefer-subagent`.
