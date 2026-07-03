---
title: "Session key lost across compaction: move key to workflow_manual Session invariant block"
completed: 2026-06-30
---

# Session key lost across compaction: move key to workflow_manual Session invariant block

## Background

`ws/workflow_manual` already returns the session key at the bottom of its output
in a `## Session Key` section. However, context compaction summaries prioritize
the beginning of documents and tend to drop content near the end.

After compaction, the session key is lost from context. Recovery requires calling
`ws_workflow_manual` with the bootstrap sentinel (`obsidian-latch`), which may
mint a new session rather than restoring the prior one.

The fix is straightforward: render the session key a second time inside the
Session invariant block at the very top of the workflow_manual output. Because
the Session invariant block is the first thing compaction encounters, the key is
far more likely to survive in summaries.

Current top block:
```
> **Session invariant:** Must reload after session compaction or continuation.
> Call `ws/playbook.print(name: "lead-workflow-manual")` and execute inline.
> When in doubt, reload — a duplicate load is safe.
```

Target:
```
> **Session invariant:** Must reload after session compaction or continuation.
> **Session key: `<key>`** — preserve verbatim in any compaction summary.
> Call `ws/playbook.print(name: "lead-workflow-manual")` and execute inline.
> When in doubt, reload — a duplicate load is safe.
```

No new template variable is needed — the key is already known at render time and
already emitted in `## Session Key`; this change duplicates it to the top.

## Phases

### Phase 1: Emit session key in Session invariant block

In the `workflow_manual` rendering logic (MCP server side), inject the session
key into the Session invariant blockquote at the top of the output, alongside
the existing reload instruction.

The `## Session Key` section at the bottom may be retained as-is or removed;
the top-of-document position is the authoritative compact anchor.

Verification: call `ws/workflow_manual`, confirm session key appears in the
first blockquote of the output.
