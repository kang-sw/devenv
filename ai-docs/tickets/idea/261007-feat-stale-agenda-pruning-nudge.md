---
title: Nudge stale agenda pruning at compaction and revive
---

# Nudge stale agenda pruning at compaction and revive

## Background

Downstream dogfood (2026-10-07): a lead kept long-running work state in agenda
blobs and, by its own account, never cleared the stale ones. Today the only
pruning signal is size-based: `agendaSizeNote`
(`agents-plugin-tool/internal/mcp/workflow_manual.go`) tells the lead to
clear stale blobs once the agenda totals 8192 bytes or more, both at
`workflow_manual` render and on `agenda.set`. Below that threshold a stale
blob is reminded at every revive with nothing prompting its removal.

## Direction

Tie pruning to the natural boundaries where a session already re-reads its
state, rather than raising the size threshold:

- **Pi compaction.** The `ws-compact` preparation message
  (`agents-plugin-pi/src/lead-compaction.ts`) asks the lead to clear agenda
  blobs that no longer hold before it writes the summary.
- **Revive.** The `workflow_manual` session-state render asks the lead to
  clear blobs that no longer hold whenever the agenda is non-empty, not only
  above the size threshold. This boundary covers hosts whose compaction ws
  does not own (Claude Code, Codex).

Rejected: raising the threshold to 16 KiB. Every revive re-renders the whole
agenda, so a larger allowance costs context at every revive, and the observed
problem is unpruned blobs, not size.

## Open

- The exact wording at each boundary, and whether the revive nudge
  replaces or joins the size note.
