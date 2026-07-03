---
name: lead-drain-ready-queue
description: Pick the next ready/ ticket and hand it to lead-proceed. Stop if ready/ is empty.
---

# Drain Ready Queue

Spawn a light-tier Explore-style subagent to pick the next ticket: list
`ai-docs/tickets/ready/`, prefer one named as a prerequisite via another
ready ticket's `related:`/`parent:` frontmatter, otherwise the oldest
(FIFO); have it return exactly one ticket path, or report that `ready/` is
empty. Do not list `ready/` or read ticket files yourself — the subagent
does that pinpoint read, not you.

If the subagent reports `ready/` is empty, stop — do not hand off.
Otherwise hand off to `lead-proceed` with the returned path as an explicit
target; never call it bare.

Conserve lead context for the long-running goal this serves: beyond
selection, delegate everything else too — including simple tasks like
commits — to an appropriately tiered subagent or forked subagent, following
`lead-prefer-subagent`.
