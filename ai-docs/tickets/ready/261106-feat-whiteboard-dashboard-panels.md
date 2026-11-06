---
title: Whiteboard dashboard panels - tickets, todo summary, mailbox
related:
  261106-feat-mailbox-history-ring: prerequisite; the full mailbox panel's recent-messages list reads its ring
  261106-epic-ws-dashboard-serve-daemon: related; the daemon reuses these panel providers (171)
blocked-by: 261106-feat-mailbox-history-ring
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 45b9fb6f4c567495
sage-review-completeness-reviewed: 45b9fb6f4c567495
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
  (creation date), newest first; stems with the same date prefix order by
  stem name, descending. (162)
  Rejected: last-modified time from mtime or git log, because mtime differs
  per checkout and git log would need a git call on every render.
- **Mailbox panel.** (164) b, (167) b
  - The session's own address.
  - Unread count: the named inbox when this session owns it, plus the own
    reply-id queue.
  - Whether the session is waiting for mail, from the listening marker
    (`wsmailbox.ReadListeningMarker`).
  - Correspondents with their last exchange time (reply-id entry
    `Contacts`), all stored ones; the store already trims them to 20.
  - The recent messages from the history ring of
    `261106-feat-mailbox-history-ring`, every stored entry (at most 10),
    each row showing direction, counterparty, time, and the ring's title.
    The panel renders the ring's title field and derives no title of its
    own, so the ring's rule (which skips the peer-lead note line) is the
    only one.

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

**Provider signature.** A provider stays one `panel` function per panel,
and it takes an input struct instead of the bare session record: the
record, the layout being rendered (full or embed), the session root (for
the ticket directories), the session key (the history ring is keyed by it),
the session's reply-id (for mailbox state), and the process mailbox
identity the own address and the named-inbox unread count depend on (what
`mailboxOwnerCheck` reads in `mailbox_runtime.go`), resolved once by the
renderer.
The provider gathers its data once and shapes rows by layout. `PanelRow`
gains a link field for ticket rows.
Rejected: separate full and embed panel functions per provider, which
gather the same data twice and let the two layouts drift.

**Refresh.** The ticket panel is dirtied by `tickets.move`, `tickets.close`,
and `tickets.create_empty`. The mailbox panel is dirtied by `mailbox.send`
and `mailbox.recv`. A change made outside a tool call shows on the next
render. Examples are mail arriving from another session, or a ticket file
edited by hand. (166)

**Empty panels are omitted.** A ticket panel with no ready or todo tickets,
and a mailbox panel for a session with no mailbox address and no history,
are left out, as empty todo and agenda panels already are (f98d8e83).

**Ticket links.** On the full page a ticket link is a path relative to
`ai-docs/whiteboards/`. In an embed written to an `embed_dir` outside the
repository, it is an absolute `file:` URL; `html/template` rewrites an
untrusted `file:` URL to `#ZgotmplZ`, so the renderer marks the link it
built itself as `template.URL`. An embed inside the repository uses a
relative path like the full page.

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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Art

- `todoPanel` and `agendaPanel` in `whiteboard_tools.go`.
- The `panel` template in `wswhiteboard/assets/page.tmpl`, shared by both
  layouts.
- `Panel` and `PanelRow` in `wswhiteboard/view.go`. They have no link field
  today.

## Prior Decisions

- 3b89c6a5 (2026-11-06, commit): "The work is split in two (item 183). The ring and tool are mailbox capability ... The panels touch wswhiteboard and the whiteboard providers. The dashboard ticket records the mailbox ticket as a related prerequisite." — bearing: supports
- 261030-feat-session-whiteboard-mode (2026-10-30, Decisions): "The renderer is a list of section providers plus a per-view layout. The approved mockup shows mailbox, playbook, and ticket-board providers as empty placeholder panels so the layout can be judged with them." — bearing: supports
- 261030-feat-session-whiteboard-mode (2026-10-30, Decisions): "Collapsible panels. Todos, agendas, and later auxiliary sections sit in <details> panels. In the embed they start collapsed; it has no storage. On the full page their open state survives reloads through sessionStorage." — bearing: constrains
- f98d8e83 (2026-11-04, commit): "Implicit re-render is gated on board existence, not the knob, and also fires after route.resolve_* (which write todo/agenda); empty todo/agenda panels are omitted." — bearing: constrains
- c6339b07 (2026-11-06, commit): "Staleness is judged against the latest update, not every render: ... the implicit todo/agenda refresh (BeginRender) no longer turns a fresh Now stale." — bearing: constrains
- c6339b07 (2026-11-06, commit): "Playbook Now sentence rewritten in agents-plugin/rsrc/whiteboard; the rsrc manifest regenerated, the wsflow rsrc mirror regenerated with its documented command, and the Pi rsrc mirror resynced (TestPiMirrorUpToDate failed otherwise)" — bearing: constrains
- ebf89cb0 (2026-11-06, commit): "Pi manifest has no regen entrypoint; copied from canonical (byte-identical tree), following precedent c6339b076." — bearing: constrains
- 261106-epic-ws-dashboard-serve-daemon (2026-11-06, Cross-Child Decisions): "The static dashboard ticket goes first. The daemon reuses the panel providers of 261106-feat-whiteboard-dashboard-panels rather than defining its own. (171)" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/whiteboard_tools.go, agents-plugin-tool/internal/wswhiteboard/view.go, agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl, agents-plugin/rsrc/whiteboard/whiteboard.md with agents-plugin-wsflow and agents-plugin-pi rsrc mirrors and manifests |
| scope.surface | cross-module | whiteboard providers in internal/mcp, View and templates in internal/wswhiteboard, mailbox reads from internal/wsmailbox, shipped playbook text |
| scope.new_public_symbol | yes | a link field on exported wswhiteboard.PanelRow and new ticket and mailbox providers; internal Go packages, no new MCP tool |
| scope.new_type_contract | yes | whiteboardSectionProvider.panel is func(rec sessionRecord) today at agents-plugin-tool/internal/mcp/whiteboard_tools.go#L75-L78 and must become layout-aware; PanelRow gains a link |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/render_test.go, agents-plugin-tool/internal/mcp/whiteboard_tools_test.go, Pi and wsflow mirror tests |
| complexity.reuse_points | confirmed | whiteboardProviders and dirtiedBy at whiteboard_tools.go#L80-L91, panel template page.tmpl#L51, wsmailbox.ReadListeningMarker listening.go#L135, ReplyIDEntry Contacts and SortedContacts replyid.go#L45-L70 |
| complexity.side_effect_risk | moderate | adds re-render hooks to tickets.move/close/create_empty and mailbox.send/recv, and each render reads ticket dirs and mailbox files |
| risk.correctness | moderate | ticket links must resolve from ai-docs/whiteboards and from an embed_dir outside the repo; unread count spans named inbox plus reply-id queue |
| risk.fit | moderate | recent-messages list depends on unlanded 261106-feat-mailbox-history-ring; providers are reused by the dashboard daemon epic |
| risk.test | moderate | no headless browser for visual check; link resolution and size-limit stress need new render tests |
| risk.security_or_contract | moderate | shipped playbook text changes across three package mirrors; ring titles render into HTML, escaped by html/template in render.go |

## Phases

### Phase 1: dashboard panels

Add the ticket and mailbox providers and the embed summaries above. Make the
providers layout-aware where full and embed differ. Update the two playbook
sentences under **Playbook text**, mirrored to wsflow and the Pi rsrc copy
with manifests regenerated.

Verification:
- `internal/wswhiteboard` render tests:
  - Full ticket panel: ready rows and the latest-10 todo rows, with links.
  - The rendered ticket `href` resolves from `ai-docs/whiteboards/` and,
    for an embed written to an `embed_dir` outside the repository, is a
    `file:` URL that is not rewritten to `#ZgotmplZ`.
  - Empty ticket and mailbox panels are omitted.
  - Recent-message rows show the ring's title.
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
