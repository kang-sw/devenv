---
title: Whiteboard dashboard panels - tickets, todo summary, mailbox
related:
  261106-feat-mailbox-history-ring: prerequisite; the full mailbox panel's recent-messages list reads its ring
---

# Whiteboard dashboard panels - tickets, todo summary, mailbox

## Background

Today the whiteboard renders two auxiliary panels, Todos and Agendas, through
the section providers in `internal/mcp/whiteboard_tools.go`
(`whiteboardProviders`). Each provider has a `dirtiedBy` tool list, and
`refreshWhiteboardAfter` re-renders the full page, without an embed, after
those tools write. Both pages get the same panels. The full page shows them
open in an aside; the embed shows them collapsed.

The user asked for dashboard elements on the full page. The embed carries only
summaries, because the embed host limits a page to about 512KB and the full
page is where detail belongs. The server reads the data and renders it into
the page; the page itself reads no files. Decided in the 2026-11-06
whiteboard discussion, items (156)-(158), (161), (162), (164)-(167), (183), (186).

## Decisions

**Full page**
- **Tickets.** All `ready/` tickets, plus the latest 10 `todo/` tickets, by
  title. No `idea/` tickets. Each row links to the ticket file. (156)
  Rejected: adding open review follow-ups, active worktrees, or recent commits
  in this first version (156 b/c).
- **Choosing the latest 10 todo tickets.** Sort by the stem's date prefix
  (creation date), newest first. (162)
  Rejected: last-modified time from mtime or git log, because mtime differs
  per checkout and git log would need a git call on every render.
- **Mailbox panel.** (164) b, (167) b
  - The session's own address.
  - Unread count: the named inbox when this session owns it, plus the own
    reply-id queue.
  - Whether the session is waiting for mail, from the listening marker
    (`wsmailbox.ReadListeningMarker`).
  - Correspondents with their last exchange time (reply-id entry
    `Contacts`).
  - The recent messages from the history ring of
    `261106-feat-mailbox-history-ring`, each row showing direction,
    counterparty, time, and the body's first line.

  Rejected: listing every peer inbox in the clone (164 c). That is up to 20
  entries, including sessions that are not running, and `mailbox.lookup_peers`
  already serves it on demand.

**Embed**
- **Todos.** The count plus the titles of unfinished items. (157)
  Rejected: count only; every item, as today.
- **Agendas.** Unchanged: each key with its one-line summary.
- **Tickets.** `ready/` titles only, each with the same ticket-file link as
  the full page. (161)
  Rejected: no links. A link is tens of bytes, and a host that blocks it
  merely makes the click do nothing.
- **Mailbox.** One line: the address and the unread count. (165)

**Refresh.** The ticket panel is dirtied by `tickets.move`, `tickets.close`,
and `tickets.create_empty`. The mailbox panel is dirtied by `mailbox.send`
and `mailbox.recv`. A change made outside a tool call shows on the next
render. Examples are mail arriving from another session, or a ticket file
edited by hand. (166)

**No truncation.** The embed has no truncation logic. (158)
Rejected: truncation. The fixed inline cost of an embed is about 25KB (CSS,
`form.js`, sprite), far from the limit, so truncation would only add
complexity.

**Playbook text.** The whiteboard playbook has two sentences that list the
panels and their refresh sources. One is in the intro ("with the session's
todos, agendas, and a Now slot"); the other is under Calls ("todo and agenda
writes re-render the full page"). Both are updated to name the ticket and
mailbox panels and the tools that refresh them. No other playbook text
changes. (186)

## Constraints

- **512KB limit.** The embed host limits a page to about 512KB. This figure
  is the user's report and was not measured here. A render test with many
  items must show that the embed stays well under it.
- **Link resolution.** Ticket links must resolve from wherever the page is
  written. The full page lives in `ai-docs/whiteboards/`. The embed may be
  written to `embed_dir`, which can be outside the repository.
- **Manuals.**
  - `ai-docs/manuals/ws-mcp.md`: edits under `internal/mcp/`.
  - `ai-docs/manuals/shipped-surface-boundary.md`: shipped template and
    playbook text.
  - `ai-docs/manuals/skill-authoring.md` and
    `ai-docs/manuals/wsflow-mirroring.md`: the whiteboard playbook edit,
    mirrored to wsflow and, per precedent c6339b076, to the Pi rsrc copy with
    manifests regenerated.

## Prior Art

- `todoPanel` and `agendaPanel` in `whiteboard_tools.go`.
- The `panel` template in `wswhiteboard/assets/page.tmpl`, shared by both
  layouts.
- `Panel` and `PanelRow` in `wswhiteboard/view.go`. They have no link field
  today.

## Phases

### Phase 1: dashboard panels

Add the ticket and mailbox providers and the embed summaries above. Make the
providers layout-aware where full and embed differ.

Verification:
- `internal/wswhiteboard` render tests:
  - Full ticket panel: ready rows and the latest-10 todo rows, with links.
  - Embed shows ready rows only.
  - Embed todo shows the count plus unfinished titles.
  - Mailbox full panel and embed line.
  - The many-items stress render stays under the size limit.
- `internal/mcp` tests:
  - A `tickets.move` and a `mailbox.send`/`recv` each re-render the full
    page with the panel updated.
  - The latest-10 selection follows the stem date.
- `go test ./internal/wswhiteboard -count=1`,
  `go test ./internal/mcp -run 'Whiteboard' -count=1`, the mirror and
  manifest tests named by `wsflow-mirroring.md`, then `go test ./...` in
  `agents-plugin-tool`.
- No headless browser is installed. Say whether the page was checked
  visually.
