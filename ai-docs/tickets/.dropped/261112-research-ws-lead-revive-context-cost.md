---
title: Cut the context a lead session spends on post-compaction revive
dropped: 2026-11-12
---

# Cut the context a lead session spends on post-compaction revive

## Background

User report (2026-11-12, dashboard session): right after a compaction, a
lead session crossed 100K context tokens within a few skill calls. The
transcript's per-call usage for that session (Claude Code, Opus) shows:

- First model call after the compaction: about 85K tokens, before any ws tool
  ran.
- `ws/workflow_manual` (the `ws:lead-revive` step): about 16K more (a 44 KB
  tool result), bringing it to about 102K.

Approximate contributors to the 85K baseline (JSON-escaped attachment bytes
from the transcript; token figures are rough estimates):

| Source | Size | Owner |
|---|---|---|
| Re-injected bodies of every skill invoked earlier in the session (lead-revive, lead-run, lead-ticket, lead-delegate, lead-discuss) | 93 KB, ~20-25K | Claude Code re-attaches; content size is ws |
| Skill listing (all plugins) | 51 KB, ~12K | harness, ws descriptions included |
| Files read shortly before compaction, re-attached (two tickets, a manual, a mockup) | 72 KB, ~15K | harness |
| AGENTS.md instructions | 33 KB, ~7K | project |
| Deferred tool and agent listings | 25 KB, ~6K | harness |
| Compaction summary | 15 KB, ~4K | harness |
| System prompt and tool schemas | not in transcript | harness |

The `workflow_manual` result's largest parts were the embedded `whiteboard`
playbook (the full command reference, on every revive) and the agenda
blobs, one of which had grown to a multi-KB history log.

## Questions

- Can `workflow_manual` stop embedding the full whiteboard playbook and point
  to `ws/playbook.read` instead (the update tool's description already says
  to read it before first use), or embed only a short digest?
- Should the agenda carry a size budget (warn at write, or summarize stale
  keys at render) so a long session's agenda does not grow into a log?
- Is the size of ws skill bodies the harness re-injects after compaction
  worth reducing (lead-ticket and lead-run are the longest), given the
  harness re-attaches every skill used earlier, not only the active one?
- Which other ws contributors (skill descriptions in the listing, manual
  list, notes) are worth trimming?

## Conclusion (2026-11-12, board 364)

Nothing on the ws side is worth cutting. A session that uses the whiteboard
reads the whiteboard playbook anyway, so moving it out of `workflow_manual`
would only shift the cost to a `ws/playbook.read` call. The agenda already
warns on write past 8192 characters (`agendaSizeNudgeThreshold`,
`internal/mcp/workflow_manual.go:78`). The rest of the baseline is harness
re-attachment. Dropped.

## Out of scope

Harness behavior ws does not control: which files are re-attached after
compaction, the tool schema size, and other plugins' skill listings.
