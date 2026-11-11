---
title: "A whiteboard patch that rewrites an item's title re-asks it"
---

# Title patch re-asks a whiteboard item

## Background

User, 2026-11-11: an agent sometimes rewrites a question it already put on
the whiteboard and posts it again. When the user had already sent an answer
for that item from the dashboard, the item keeps its sent mark, so the
rewritten question looks answered and the user does not see that it waits
on them again.

Today `applyPatch` (`agents-plugin-tool/internal/wswhiteboard/store.go`)
stamps `ReaskedAt` only when a patch reopens a settled item or really
changes its class or options; the dashboard's `sentMarks`
(`internal/wsdashboard/pageview_inbox.go`) drops sent marks older than that
stamp. A patch that rewrites only the title is not a re-ask, and the
whiteboard playbook says so.

## Decisions

1. A patch that really changes an item's title is a re-ask, the same as a
   class or options change: it stamps `ReaskedAt`, so the dashboard drops a
   sent mark older than the patch and the item shows as waiting on the user
   (user acknowledgement, board (294)). The title and the options carry the
   question's meaning.
   - Rejected: re-asking on every field change. A patch that edits only
     `wb:why`, `wb:recommend`, or links leaves the question itself as it was,
     so it keeps the sent mark.
   - Resending the current title unchanged is not a re-ask, matching the
     existing rule for class and options.

## Constraints

- An answer sent before the patch and not yet delivered still reaches the
  lead, as today; only the sent mark changes.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`
  (the whiteboard playbook's `Re-ask` paragraph has a wsflow copy at
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`), and
  `ai-docs/manuals/ws-mcp.md`.

## Phases

### Phase 1: title change counts as a re-ask

- `applyPatch` stamps `ReaskedAt` when the patched title differs from the
  stored one; update its doc comment and the `ReaskedAt` field comment.
- The whiteboard playbook's `Re-ask` paragraph (both packages) names the
  title among the re-asking changes and drops it from the not-a-re-ask list.

Verification:

- A store test: a title-only patch on an open item stamps `ReaskedAt`; a
  patch resending the same title, or editing only `wb:why`, `wb:recommend`,
  or links, does not.
- The existing re-ask and sent-mark tests stay green; run the wswhiteboard,
  wsdashboard, and mcp packages plus any playbook or wsflow mirror drift
  tests through `~/.local/bin/ws-rgo`.
