---
title: "Board search: index topic log entries and panels, including older history pages"
related:
  261115-feat-ws-dashboard-search-drawer-answer-agent: "sequenced before this ticket (MAIN, 2026-11-15): both edit paletteRows/palettePick in dashboard.js and the drawer moves the palette; not a technical prerequisite"
  261114-feat-ws-dashboard-unified-search-palette: "predecessor: built the board slot (paletteRows, qBoard) this ticket extends"
---

# Board search: index topic log entries and panels, including older history pages

## Background

The owner asked that the search palette's board results include log
records ("로그 기록도 들어가야"; relayed by MAIN, 2026-11-15 at 01:50Z).

Evidence was taken read-only on 2026-11-15 (search-drawer ticket worktree
at `9ea13ec19`); paths are relative to `agents-plugin-tool/internal/`.

### Current state

- **Rows are built client-side.** `paletteRows()` in
  `wsdashboard/assets/dashboard.js:5580-5621` emits group 0 sessions,
  1 topic labels, 2 topic body (`.doc-body`, or the inert `data-srcdoc` of a
  framed body) plus the `.rtext` recap, 3 item titles, 4 answers/drafts.
  `paletteClosed()` (5622-5630) loads older closed topics through the
  existing show-more path. The host contract is the `searchRows` comment
  in `wsdashboard/assets/tracker.js:153-157` ("group (0..4)"). `qBoard()`
  (`tracker.js:6151-6169`) scores every row with `group >= 1`, with loose
  excerpt matching for groups 2 and 4, lists only groups 1 and 3 on an
  empty query, and tie-breaks by group; the board row renderer shows an
  excerpt for groups 2 and 4 (`tracker.js:6390`).
- **Not indexed.** A topic's stream (`wswhiteboard/assets/page.tmpl`,
  `define "stream"`/`"entrypage"`/`"logrow"`/`"entry"`): log rows
  `article.wb-entry` with `.entry-text`; system rows folded in
  `details.system-run`; panel cards `details.entry-panel` with
  `.entry-recap` and `.doc-body.entry-body`; earlier panel versions in
  `details.panel-versions`; and older stream pages held as inert
  `<template data-history-page>` inside `.wb-history`. All of it is in the
  board DOM the dashboard already renders (`wswhiteboard/render.go:268,299`
  set `LayoutFull`), so no Go or format change is needed.
- **History navigation exists.** `window.wbHistory.loadTarget(root, id,
  beforeGeometry)` (`wswhiteboard/assets/stream.js:345-361`, exported at
  727) materializes an entry by id from older history pages and from
  deferred topics (`template[data-history-topic]`), opens every enclosing
  `<details>` (system run, panel, versions), opens a panel card, flashes the
  target and navigates the history scroller; its `beforeGeometry` callback
  receives the owning topic or a live descendant, possibly more than once.
  The dashboard page loads this script (`dashboard.js:606-611,783,799-805`
  use `window.wbHistory`).
- **Body-row bug.** A topic without a body has no topic-owned `.doc-body`,
  although `.tdoc` can still exist for a closed topic with a recap
  (`agents-plugin-tool/internal/wswhiteboard/view.go#L327-L333`,
  `agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L40-L40`), so
  `topic.querySelector(".doc-body")` (`dashboard.js:5594`) matches the
  first panel's `.doc-body.entry-body` in the stream and lists that panel's
  text as the topic's "body" row; `palettePick` then scrolls to the topic,
  not the panel.

## Decisions

Owner decision (relayed by MAIN, 2026-11-15 at 01:50Z):

- **Board search includes log records.** Rejected: leaving the stream out
  of search.

Scope announced by MAIN (no owner question needed):

- Index log lines and panel recap/body, including entries on older history
  pages that are not yet loaded.
- `palettePick` opens whatever hides the target (a folded panel, a system
  run, an older history page) before scrolling to it.
- Fix the body-row bug above.
- Tests extend `wsdashboard/testdata/search-palette.test.cjs`; add an i18n
  string only if a new visible label is needed.
- Compatibility: no tier. Only embedded browser assets change; no MCP tool,
  hello, store, tracker event or manifest field.

Technical calls (EXEC4):

- **One row per stream entry, new group 5.** Every `article.wb-entry` in a
  live topic's stream (log line, system row, latest panel card and each
  earlier panel version), whether live or inside a
  `template[data-history-page]`, yields one row `{group: 5, kind:
  "board-entry", topic, entry: <entry id>, title: <text>, hint: <topic
  label> · <entry clock>}`. The contract comment in `tracker.js:153-157`
  documents group 5 and the `entry` field; `qBoard` treats group 5 like
  groups 2 and 4 (loose excerpt matching, not on the empty-query list) and
  the row renderer shows its excerpt. Group 5 sorts last on equal score.
  Rejected: folding entries into group 2, where a long stream would crowd
  out topic bodies at equal score.
- **Entry text.** A log or system row uses its `.entry-text` without the
  time element; a panel uses its `.entry-recap` plus its own `.entry-body`
  text, reading a framed body's inert `data-srcdoc` or `srcdoc` through a
  `<template>` exactly as topic bodies do (`dashboard.js:5596-5602`); a
  nested version's text is its own, not its newer card's. Template content
  is parsed and read, never adopted into the live DOM during indexing.
- **Pick through `wbHistory.loadTarget`.** A `board-entry` row is opened by
  `window.wbHistory.loadTarget(<the live owning topic>, row.entry,
  beforeGeometry)`; the topic is looked up again by `row.topic` through
  `boardQ` as item picks do, so a topic in the pinned column works too.
  The callback runs the existing topic preparation (`WsOverview.pick`,
  `WsCards.unfoldTopic`, leaving a tracker page, showing the board tab)
  once per pick. `loadTarget` itself opens the enclosing `<details>`,
  flashes the target and navigates the history scroller with the sticky-head
  offset (`agents-plugin-tool/internal/wswhiteboard/assets/stream.js#L264-L277`,
  `#L345-L360`), so for an entry the pick adds only focus on the returned
  article (`preventScroll`, skipped under Cmd/Ctrl+Enter) and calls neither
  `scrollIntoView` nor `palette-flash`: the clipped history box is never
  scrolled by `scrollIntoView`
  (`261113-feat-ws-whiteboard-topic-history-load-more` Decisions: "The
  clipped box is never scrolled"). An entry `loadTarget` cannot find (the
  board was swapped and the entry is gone) falls back to the topic pick;
  existing item picks instead return when the target is missing
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L5637-L5640`).
  Rejected: the generic focus/`scrollIntoView`/flash path for entries
  (breaks the clip's bottom anchoring); a second loader in `dashboard.js`
  that duplicates the page-materialization and system-run merge logic.
- **Body-row fix.** The topic body row reads only the topic's own fold:
  the `.doc-body` inside the topic's `.tdoc`, never one inside
  `.topic-stream`. A topic without a body yields no body row.
- **Deferred closed topics stay as they are.** Topics held in
  `template[data-history-topic]` are not indexed today (neither their
  labels nor their items); this ticket does not change that.
- **No new visible label.** Entry rows render in the board slot like body
  rows; no i18n string is added.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- `tracker.js` stays host-agnostic: it learns only the generic group 5 and
  `entry` field; stream parsing and navigation live in `dashboard.js`
  (`261114-feat-ws-dashboard-unified-search-palette`).
- Indexing must not execute scripts, fetch images or mutate the board:
  read template contents inertly, as the topic-body path does.
- Never touch the shared daemon or port 29417; browser checks use an
  isolated daemon fixture with its own `WS_CACHE_HOME`
  (`scripts/palette-headless.mjs` pattern). Never run the whole
  `./internal/wsdashboard` Go test package; use `go test -run`. Offline env
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. End every run with a scan for
  processes it started and stop only those.
- Before editing, read the manuals the AGENTS.md Implementation Conventions
  table binds to the touched paths:
  `ai-docs/manuals/shipped-surface-boundary.md` (all of
  `agents-plugin-tool/`) and `ai-docs/manuals/release-compatibility.md`
  (`agents-plugin-tool/internal/`).
- Out of scope: server-side search, deferred closed topics, and the
  search-drawer ticket's keyboard and drawer changes.

## Prior Decisions

- 261115-feat-ws-dashboard-search-drawer-answer-agent (2026-11-15, Decisions): "No row is highlighted until the user moves with the arrow keys. Plain Enter asks the answer agent; Enter after an arrow selection opens that result" — bearing: constrains
- 261114-research-ws-review-sweep-794-findings (2026-11-15, commit): "Search calls the owner only after Overview declines its existing handoff, before tab/page landing, disclosures, focus or geometry." — bearing: constrains
- 261114-feat-ws-dashboard-unified-search-palette (2026-11-14, commit): "Rows keep their selection by key across late batches and re-ranks; stale rows stay dimmed until the new generation's first batch" — bearing: constrains
- 261113-feat-ws-whiteboard-board-frontend-performance (2026-11-14, commit): "Decision 2 revision (stop (c) settled): an <iframe> with only data-srcdoc still committed about:blank synchronously while the page parsed (65 CommitLoad, a 507 ms parse task at 4x); a div is inert." — bearing: constrains
- 261113-feat-ws-whiteboard-topic-history-load-more (2026-11-13, Decisions): "The clipped box is never scrolled" — bearing: constrains (entry picks never call scrollIntoView; see Decisions)
- 3a13a369 (2026-11-14, commit): "Hold board swaps until exit and route palette Enter/Ctrl+Enter through the overview picker" — bearing: constrains
- 261113-feat-ws-whiteboard-topic-panels-and-log (2026-11-13, commit): "Reuse board SSE regions and trusted frame metadata; materialize inert older/closed history only for known targets and preserve viewport anchors." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.js, agents-plugin-tool/internal/wsdashboard/assets/tracker.js, agents-plugin-tool/internal/wsdashboard/search_palette_test.go, agents-plugin-tool/internal/wsdashboard/testdata/search-palette.test.cjs, scripts/palette-headless.mjs |
| scope.surface | cross-module | Extends the host searchRows/searchPick row contract in agents-plugin-tool/internal/wsdashboard/assets/tracker.js#L153-L159 and consumes wswhiteboard's window.wbHistory.loadTarget |
| scope.new_public_symbol | no | No new exported function, MCP tool or HTTP route; board-entry is a new internal host-row kind |
| scope.new_type_contract | yes | searchRows gains group 5, kind board-entry and entry; the existing contract lists groups 0..4 (agents-plugin-tool/internal/wsdashboard/assets/tracker.js#L153-L157) |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/search_palette_test.go; agents-plugin-tool/internal/wsdashboard/testdata/search-palette.test.cjs; scripts/palette-headless.mjs; an additional node file is optional |
| complexity.reuse_points | confirmed | Read paletteRows/palettePick, inert frame parsing, qBoard/qRowHtml, wbHistory.materialize/loadTarget and existing hostCases fixtures |
| complexity.side_effect_risk | moderate | Picks materialize older pages and open disclosures, change topic folds, focus and scroll; indexing must leave templates and the live board unchanged |
| risk.correctness | high | Entry/version ownership, paging and pre-geometry topic revelation must agree; qKeyOf currently ignores entry and would give same-topic entry rows identical selection keys (agents-plugin-tool/internal/wsdashboard/assets/tracker.js#L6205-L6226) |
| risk.fit | moderate | Entry picks must respect the history clip (no scrollIntoView, settled in Decisions) and boardQ spans the board region and the pinned column (agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L131-L140) |
| risk.test | high | Existing hostCases use DOM stubs, not rendered templates or real history geometry; the new older-page/system-run acceptance also depends on the unlanded drawer keyboard contract |
| risk.security_or_contract | moderate | Panel payloads can contain active HTML; indexing must use inert template parsing and navigation must preserve loadTarget's existing ID allowlist (agents-plugin-tool/internal/wswhiteboard/assets/stream.js#L20-L21, agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L5596-L5602) |

## Phases

### Phase 1: Index stream entries and open them from search

Add the group-5 entry rows, the entry-text extraction, the
`wbHistory.loadTarget` pick path and the body-row fix, following the
decisions above.

Verification:
- `wsdashboard/testdata/search-palette.test.cjs`: `qBoard` scores group 5
  with excerpt matching, omits it on an empty query, and sorts it after
  groups 1-4 at equal score; the board row renders a group-5 excerpt.
- A DOM-level test of `paletteRows`/`palettePick` (extend
  `wsdashboard/search_palette_test.go` or add a node test beside it) over a
  board fixture rendered from the real templates: rows for a live log line,
  a system row inside a folded run, a panel recap, a panel body, an earlier
  panel version, and a log line on an older `template[data-history-page]`;
  no body row for a topic without a body, and no panel text in any topic's
  body row; indexing leaves the template count and the live DOM unchanged.
- `scripts/palette-headless.mjs` (isolated daemon, a seeded board with
  enough entries to page): search a phrase that exists only on an older
  page and only inside a folded system run; opening the row (arrow keys
  then Enter under the search-drawer keyboard contract, which lands first
  (pending 261115-feat-ws-dashboard-search-drawer-answer-agent))
  loads the page,
  the run and panel open, the target is in view inside the history clip and
  flashed, and the clip's bottom anchoring is not disturbed (no
  `scrollIntoView` on the entry);
  Cmd/Ctrl+Enter keeps focus in the palette.
- Focused Go tests with `-run` for the touched tests, then an owned
  process scan.
