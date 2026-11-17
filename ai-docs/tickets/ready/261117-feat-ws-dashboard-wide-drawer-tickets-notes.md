---
title: "Dashboard wide view: session drawer with 2-column ticket and notes panels"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 7dfc3d227d301a01
sage-review-design-reviewed: 7dfc3d227d301a01
---

# Dashboard wide view: session drawer with 2-column ticket and notes panels

## Background

The session drawer (top-right hamburger) lays every present panel (Todo,
Agenda, Tickets, Mailbox, Worktrees, Session holders; see
`agents-plugin-tool/internal/wsdashboard/assets/dashboard-drawer.js#L49-L105`)
in an auto-fit grid of >=300px columns inside a
right-anchored sheet capped at 960px at >=1600px viewport (1280px for the Git
sheet); it stacks vertically at <=759px and opens 320px tall by default
(search `dr-clip`, `drbody` and `grid-auto-rows` in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard-drawer.css`). The
ticket panel shows only `ready` and `todo` from the session root, todo capped
at 10 while its count covers the whole backlog (search the ticket panel
builder in `agents-plugin-tool/internal/mcp/whiteboard_panels.go`). The
dashboard exposes no ws notes today.

The owner wants the wide-view drawer to give tickets and notes more room and
to show ws notes by layer (2026-11-17).

Facts from investigation (2026-11-17, read-only):

- Note layers: machine `<config-home>/notes.json`, worktree
  `<cache>/proj/<worktree-key>/notes.json`, clone
  `<cache>/proj/<project-key>/shared/notes.json`, repo `ai-docs/ws-notes/`
  (one file per key) under the session root (`internal/wsnote/store.go`,
  `repo_store.go`). Records carry key, value, priority, written_at and
  visible (false = muted from ambient injection, still listed); `note.query`
  sorts priority desc, time desc, key asc (`internal/wsnote/search.go`).
- Computing the worktree/clone store path goes through
  `wsstate.Manager.Ensure`, which can create directories and upsert metadata,
  so the existing path helper is not a side-effect-free read.
- In LAN mode a token-authenticated remote viewer passes the same outer guard
  as local viewers for every route, so a new notes route is LAN-visible
  unless it gates itself (`internal/wsdashboard/access.go`).
- Agenda and tickets reach the page as live panel-region updates; worktrees
  and session holders are fetched when the drawer opens.

Approved visual reference: `ai-docs/assets/mockups/261117/wide-drawer/drawer-wide.html`,
states `#c` (default), `#c-idea`, `#c-lan` and `#c-tall` — approved (26)=ok.
States `#a` and `#b` are rejected alternatives kept for history.

## Decisions

1. **Layout C — (10)=d.** In the wide drawer the Tickets panel and the Notes
   panel each span 2 grid columns; Agenda, Session holders and Worktrees take
   1 column each. The grid packs densely in the order Agenda, Tickets,
   Session holders, Worktrees, Notes (at 1280px: row 1 Agenda | Tickets x2 |
   Holders, row 2 Worktrees | Notes x2 | one empty cell). A span is one wider
   card whose rows run across both columns, not two side-by-side lanes.
   Rejected: (A) tickets as two side-by-side lanes with notes in a side
   column behind layer tabs; (B) a full-width notes row with the four layers
   as columns; a user-arrangeable m x n grid (owner judged it out of scope).
2. **Ticket panel stacks ready -> todo -> idea — (10)=d.** Sections run top
   to bottom: 준비됨, 할 일, 아이디어. Idea is collapsed behind a `더보기` row
   and fetched on demand. A ticket row is one line where it fits: category
   chip, title, stem right-aligned and ellipsized.
3. **Todo is no longer capped — (12)=ok.** All todo tickets are listed; the
   card scrolls internally.
   Rejected: keeping the 10-item cap with its own `더보기`.
4. **Wide sheet cap 1280px — (13)=ok.** The 960px cap at >=1600px becomes
   1280px (the existing Git sheet cap), giving four >=300px columns.
5. **Taller default height in the wide view — (25)=ok.** The wide drawer
   opens tall enough to show both grid rows (about 60vh); a height the user
   dragged is kept as today.
   Rejected: lowering the 280px row minimum, which makes the card lists too
   short.
6. **Notes panel stacks machine -> clone -> repo -> worktree — (10)=d.** Each
   layer section has a small header with its count; a note row shows a
   priority badge (0 greyed), the key in monospace, a relative time and a
   one-line value preview that expands on click, in `note.query` order.
7. **LAN viewers see only the repo layer — (08)=b.** A non-local viewer's
   notes panel shows the repo layer and a one-line hint that the other
   layers are local-only (mockup `#c-lan`). Owner's reason: notes such as
   `handoff` are needed on LAN viewers; repo notes are already in git.
   Rejected: hiding the notes panel from LAN viewers entirely; showing all
   layers to LAN viewers (machine/clone notes hold local paths and
   confidential locations).
8. **Notes cache with an explicit refresh — (07)=b plus owner amendment.**
   The daemon keeps the session's notes in memory and serves the cache when
   the drawer opens; note tools running inside the daemon update it directly;
   the repo layer is checked by a directory fingerprint when the drawer
   opens. The Notes card header carries a refresh button that rereads all
   layer files for that session on click. Store paths resolve through a
   read-only path that creates no directory or metadata. Owner's reason:
   standalone `ws-mcp serve --stdio` and direct `ws-mcp call`, which can
   write notes outside the daemon (`agents-plugin-tool/cmd/ws-mcp/main.go#L18-L76`,
   `agents-plugin-tool/cmd/ws-mcp/hosted_cli.go#L521-L535`,
   `agents-plugin-tool/internal/mcp/note_tools.go#L27-L51`), are rarely used,
   so an explicit refresh is the compromise. The refresh button is not in the
   mockup; style it like the existing panel header actions.
   Rejected: background stats of every layer file on each drawer open;
   rereading note files per request or per event (violates the daemon
   file-touch rule).
9. **Wide rules reuse existing breakpoints — (27)=ok.** The 2-column spans
   apply whenever the drawer grid has at least two columns (>=760px); the
   1280px cap applies at >=1600px. Rejected: a new width breakpoint.
10. **Todo and Mailbox panels — (36), auto-confirmed.** When present they take
    one column each and pack after Worktrees and before Notes.
11. **Idea fetched on demand — (29)=ok.** A new route reads `idea/` only when
    the user presses `더보기`. Rejected: adding idea to the live panel
    region.
12. **LAN gate on the server — (30)=ok.** The notes route sends a non-local
    viewer only the repo layer plus a flag that the other layers are
    local-only. Rejected: hiding layers in the browser.
13. **Muted and empty states — (33)=ok.** Muted notes are dimmed with a
    struck-through key and a `muted` tag; an empty layer shows an empty-state
    line (mockup `#c`).
14. **No compatibility surface — (31)=ok.** The new routes serve the daemon's
    own embedded page; no MCP tool, `/mcp` hello, store shape, tracker event
    or manifest change, so no tier applies.
15. **Two phases and verification — (28), (32)=ok.** Phase 1 layout and
    tickets, Phase 2 notes; each closes on one passing check (Node contract
    tests locally, Go tests on w5090, LOOK captures side by side with the
    mockup states). Rejected: one phase; new browser fixtures.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Daemon file-touch rule (owner, 2026-11-17, release gate): minimize file
  touches at the root; no per-event O(N) rereads, no periodic rewrites, no
  rewriting unchanged content; keep live state in memory; interactive paths
  must not wait on file I/O.
- The narrow drawer (<=759px stack) behaves exactly as today.
- Session holders, worktrees and agenda panels keep their content and data
  paths; only their grid placement changes.
- Go tests never run locally on this Windows PC; run them on remote w5090
  (see the `remote-build-w5090` clone note recipe).

## Prior Decisions

- Prior-decision search unavailable: wsflow/rationale.query was not exposed in this session; review the relevant history before implementation.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard-drawer.css, agents-plugin-tool/internal/mcp/whiteboard_panels.go and dashboard routes |
| scope.surface | cross-module | dashboard browser, daemon routes, MCP panel and wsnote storage integration |
| scope.new_public_symbol | unknown | cache and route integration interfaces are not specified |
| scope.new_type_contract | unknown | cache and route response types are not specified |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/drawer_height_test.go, frontend-geometry.test.js and access_test.go; Phase 2 requires additional Go assertions |
| complexity.reuse_points | confirmed | dashboard-drawer.js panelTab and cardTab, whiteboard_panels.go ticketPanel, wsnote/search.go CompareRecords, wsdashboard/access.go LocalViewer |
| complexity.side_effect_risk | high | cache coherence across independent writers, store-path resolution and LAN filtering affect note visibility and confidentiality |
| risk.correctness | high | cross-process note mutations and external repo changes can leave the cache stale |
| risk.fit | high | interactive no-I/O constraint must coexist with directory fingerprinting and existing drawer panels |
| risk.test | high | new routes, cross-process invalidation and responsive layouts need coverage beyond existing contracts |
| risk.security_or_contract | high | remote route must not expose local note layers and changes cross package boundaries |

## Phases

### Phase 1: Wide drawer layout and ticket panel

Implement Decisions 1-5, 9-11: the wide grid with 2-column spans and dense order,
the 1280px cap, the taller wide default height, and the ticket panel's
ready -> todo -> idea stack with uncapped todo and the on-demand idea fetch.
The Notes panel is not part of this phase.

Verification: Node contract tests pass locally; Go tests for touched packages
pass on w5090; LOOK captures at 1920x1080 compared side by side with mockup
states `#c` (without the Notes card) and `#c-idea`; a <=759px capture shows
the narrow stack unchanged.

#### Verification checkpoint - 2026-11-17

Phase 1 code is in `6b2963ee5`; it is not phase-complete. Fixed wrapped
missing-ticket-directory detection in both the live panel and idea demand
source with `errors.Is(err, os.ErrNotExist)`, and added an absent-board demand
regression test. Full remote MCP package passed (463.724s); after R1's
Important finding in the idea source, the targeted
`^TestDashboard(TicketIdeas|ReadPageServesTheBoard)` check passed (0.257s).
The earlier remote full wsdashboard and wsi18n checks passed (273.208s,
1.878s). R1's Important is fixed; fixes-only R2 is clean.

LOOK is blocked: the first capture attempt failed on a Windows Playwright
import URL; the one environmental retry launched Chrome but timed out waiting
30 seconds for visible `#drhandle`. No PNGs or geometry measurements were
produced, so no visual comparison is claimed. The isolated daemon/browser
were stopped and temporary artifacts removed from `D:/Temp/wide-drawer/`.
The owner must authorize further capture work before the screenshot gate can
close. Phase 2 has not started.

Forward: eight Node failures reproduced at base `c79cdf133` (lifecycle
`headDirty`, history `kit.seen`, six usage-chart assertions); the exact-name
exclusion check passed 48/48. Repair those baseline contracts separately.

### Result (77fd0f74e) - 2026-11-17

Implemented the dense wide grid, 2-column Tickets, 1280px sheet cap, 60vh
uncustomized wide default, uncapped todo and on-demand idea stack. Stored
heights and the narrow stack retain their previous behavior. Missing ticket
boards now show an empty demand entry instead of returning an error.

Verification: full remote MCP passed (463.724s), wsdashboard passed
(273.208s), wsi18n passed (1.878s); targeted post-review regressions passed
(0.257s). Local Node passed 48/48 with the checkpoint's base-proven exclusions.
R1's Important is fixed; fixes-only R2 clean. No local Go tests ran.

Captures pending by lead authorization (2026-11-17 18:55): combine both
phases' LOOK at the end of Phase 2 instead of blocking this Result. Imported
only the sibling's harness regression/fix commits as `d3ae80b0a` and
`bae7372c3` (Windows .exe name and authenticated readiness); their four Node
regressions pass. No sibling dashboard changes were imported.

### Phase 2: Notes panel

Depends on Phase 1's grid. Implement Decisions 6-8, 12-13: the daemon notes cache
with its refresh button and a notes route for the session, the server-side LAN gate, and the Notes
card in the 2-column slot.

Verification: Node contract tests pass locally; Go tests for touched packages
(including a test that a non-local viewer receives only the repo layer, and
that an in-daemon note write refreshes the cache without a file reread on
the next drawer open, and that the refresh button rereads the layer files) pass on w5090; LOOK captures at 1920x1080 compared side by side
with mockup states `#c` and `#c-lan`.
