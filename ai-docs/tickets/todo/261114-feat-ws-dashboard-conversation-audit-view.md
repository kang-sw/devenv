---
title: "Dashboard: a conversation audit view - per-call rows, hidden harness events, an audit window, and conversation-wide search"
blocked-by: 261114-feat-ws-conversation-audit-event-store#phase2
related:
  261114-feat-ws-conversation-audit-event-store: "prerequisite (blocked-by #phase2): the daemon's audit store and its read API, queue item (f) = board (599). Every byte this view shows comes from that API. Its ring BatchCall call_id (requested 2026-11-14, see Decision 4) is the join key from a history row to an audit event"
  261114-feat-ws-dashboard-board-toolbar-overview: "sibling darkroom: overview dims the page through the iframe shell's layout.dim field; this ticket's wide-view darkroom uses the same field for the shell's rail and header"
  261113-feat-ws-dashboard-iframe-shell: "landed (0586a4289): session pages are shell frames; layout.dim and the shell's Esc and Ctrl+K forwarding shape the key rules here"
  261114-refactor-ws-split-oversized-source-files: "dashboard.js is on its list; the audit view goes to new assets/audit.js and audit.css (Open Decision Queue item (b))"
---

# Dashboard: a conversation audit view - per-call rows, hidden harness events, an audit window, and conversation-wide search

## Background

The owner wants to audit what a hosted conversation actually did and saw: every
tool call with its full input and output, and the events a harness streams but
the dashboard hides today (hooks, system reminders, thinking, the compaction
summary, system events, subagent traffic).

The owner approved mockup v2 with "550=ok" and added one rule: in the wide
view, a click dims everything except the conversation pane and the floating
window. The mockup is stored at
`ai-docs/tickets/assets/261114-feat-ws-dashboard-conversation-audit-view/conversation-audit-mockup-v2.html`
(a copy of the board topic `conversation-audit`'s `shared/docs/conversation-audit-mockup.html`).
Its notes panel lists the behaviour this ticket builds:

- **Conversation list.** A folded tool batch becomes a call list. Each call is
  one row: status, tool, argument preview, time, and token growth. Three rows
  show and the rest fold under "+n".
  - Every call row and every hidden-event row is clickable and opens the audit
    window. J/K move between rows; Enter opens.
  - A hidden event folds to one dotted line: a kind-coloured dot and a preview.
    Chips at the top show or hide each kind. Hooks and reminders attached to a
    call are indented under that call.
  - Conversation-wide search covers the stored outputs too. A matching row gets
    a badge such as "출력 3", and the audit window opens on that match.
- **Audit window.**
  - Tabs: Input, Output, Meta, Raw, Linked events. Raw is the harness's
    recorded JSON as is.
  - Long outputs get line numbers, an in-window search (Ctrl+F, with Enter and
    Shift+Enter for next and previous), a wrap toggle, and copy.
  - What the model saw vs what was stored: the line where the harness truncated
    what it delivered is marked, and everything below it is the part the model
    did not see.
  - A redaction is a hatched chip showing 🔒, the kind and the length. The
    original is never stored; only the rule name shows.
  - ←/→ replace the window's content with the previous or next call, and Esc
    closes it. 📌 pins the window, so the next open makes a new one, for
    comparing several.
- **By width.**
  - Wide: a floating window, dragged by its header and resized from its corner.
    It opens to the right of the conversation without covering the source row,
    and the source row keeps its selected mark.
  - Narrow: a bottom sheet at 72% height, with tabs that scroll sideways and a
    one-column Meta.
- **Harness-neutral ((545)=a, (546)=a).** A harness badge (e.g.
  `claude · stream-json`) shows on the conversation header and the window
  header. Every harness maps onto the same event kinds, so it gets the same
  list and the same window. The system prompt shows on the session-start row
  only when the stream carries it. An unknown kind becomes an "other" row with
  its Raw tab.

What exists today (Pi exploration, 2026-11-14, paths under
`agents-plugin-tool/internal/`):

- **History rendering.** `renderEntries` executes template `entry`, and the
  region template `r-entries` iterates `.Conversation.Entries`
  (`wsdashboard/pages.go:627-670`, `dashboard.tmpl:242,287`). Paging is
  `GET /s/<reply-id>/history?before=<entry>&n=<count>`, handled by
  `handleHistory` (`pages.go:1261-1302`). On the client, `nearTop` calls
  `loadHistory`, which calls `mergeEntries`; older HTML is inserted at its
  sorted position (`dashboard.js:1142-1152,1209-1245`).
- **Tool batches.** Consecutive finished calls fold into one `RingTools` /
  `ToolSummary` entry (`wslive/ring.go:35-38`).
  - Its markup is `<details class="toolsline" data-entry="<ring file name>" data-key="tools-<ring file name>">`.
    The live batch adds `class="live"` and `data-key="tools-live"`
    (`dashboard.tmpl:244,295-298`). The pill is the `<summary>`, and the body
    holds `.brows`, `.brow`, `.tool`, `.ba` and `.bm` (`dashboard.css:336-351`).
  - **No tool call id exists in the ring or the DOM.** `ToolSummary`,
    `ToolBatch`, `ToolCallArgs` and `BatchCall` carry names, counts and
    argument previews only (`wslive/toolsummary.go:42-62`, `ring.go:62-84`,
    `snapshot.go:367-372`).
- **Ring stamps** are UTC with nanosecond precision, strictly advancing within
  a process. File names are `20060102T150405.000000000Z-<random hex>.json`
  (`ring.go:59-60,128-146,190`). The audit API's `at` uses the same clock and
  precision ((599)).
- **Keys.**
  - The conversation pane has no J/K handler. The composer handles Esc, Tab and
    Enter (`dashboard.js:2507-2528`).
  - These handlers also use keys that matter here: the link menu
    (Esc/Up/Down/Enter/Space), the splitter (Up/Down) and the Files pane
    (arrows/Enter) (`dashboard.js:2888-2899,350-354`, `files.js:746-769`).
  - Cards, the drawer, the git window, rail popovers and the shell consume or
    forward Esc (`cards.js:652-655`, `dashboard.js:3321-3325,4280-4283,7739-7741`).
  - The shell forwards Ctrl+K and Ctrl+Space to its search, except from editors
    (`dashboard.js:7640-7647`).
- **Patterns to reuse.**
  - `cards.js` `maximize(c)` builds a `.bc-scrim` (`cards.js:529-571`).
  - The bottom-sheet drawer: `drSetOpen`, `drApply`, `drDragStart`,
    `drDragMove`, `drEnd`, `.dr-sheet` and `.dr-handle`
    (`dashboard.js:3133-3296`, `dashboard.css:928-1016`).
  - The commit window: `gwBuild` / `gwClose` with `.gwin-back` and `.gwin`
    (`dashboard.js:4185-4283`).
  - The Files pane: a module mounted by `WsFiles.mount` (`files.js:815-832`).
- **Layout.**
  - The conversation pane is `#convo.convo`, holding `entries` and `convlive`.
    The board column is `#bcol.bcol` (`dashboard.tmpl:21-23`).
  - The base layout is one column of tabs. An explicit split starts at
    `min-width: 760px` and the automatic wide layout at `min-width: 1100px`
    (`dashboard.css:769-814`).
- **i18n and the CSS guard.** Browser strings are `js.*` keys in the 11
  `wsi18n/tables/<lang>.json` tables (`wsi18n/i18n.go:260-274`). The CSS guard
  is `TestCSSClassesDoNotCollideAcrossModules` (`css_collision_test.go:178-201`).

## Decisions

1. **Mockup v2 is the approved visual reference ((550)=ok).** Verification
   compares result captures side by side with it, in the wide and the narrow
   view.
2. **Wide-view darkroom ((550) note).** In the wide view, opening the audit
   window from a click dims everything except the conversation pane and the
   floating window. In the iframe shell, the shell's rail and header dim
   through its existing `layout.dim` field, as the board overview does.
   - Rejected: dimming nothing (the owner asked for it), and dimming the
     conversation pane too (the source row must stay readable beside the
     window).
3. **Every byte comes from the backend's read API ((599)).** That covers
   events, the full envelope, search, and the `?call=` lookup. The UI parses no
   harness transcript file and adds no route of its own. This is a consequence
   of the backend/UI split agreed with EXEC (2026-11-14), and it keeps the view
   harness-neutral ((546)=a). Fields used:
   - On a summary: `duration_ms`, `ctx_after`, `attached`, `model_seen`.
   - In the full envelope: `seen`, `redactions`, `truncated` with the `range`
     follow-up, and `meta.signature_only`.
   - On a search hit: `call_id`.
4. **The history-to-audit join key is the call id carried by the ring.** The
   backend adds an optional `call_id` to each call of a folded tools entry, an
   additive ring field (T0); EXEC was asked on 2026-11-14 and this is pending
   their ticket edit. This ticket's template emits it as `data-call` on each
   call row.
   - Rejected: joining by time window between ring stamps (ambiguous with
     parallel calls and subagents).
5. **Compatibility tier: T0.** Dashboard asset content, a template change that
   adds attributes, and new i18n keys. No route, MCP tool, store or tracker
   event changes here.

## Open Decision Queue

Last ID: lead-local letters. MAIN mints board IDs, so these get numbered when
they go to the owner.

- (a) [open] [technical] Ring entries without call ids keep today's folded pill
  over turning into rows. Those are entries written before the backend change,
  or by a harness the mapper does not cover. Only rows with a `data-call` are
  clickable, and hidden-event rows still interleave by `at`. This is chosen
  over a time-window guess, which Decision 4 rejects.
- (b) [open] [technical] New assets over growing dashboard.js. The new files
  are `assets/audit.js` (a `WsAudit` module mounted from the session page, as
  `WsFiles.mount` is) and `assets/audit.css`, with classes prefixed `au-` so
  the collision guard passes. dashboard.js is on the split-oversized list.
- (c) [open] [technical] Window and sheet are built new.
  - The wide window is a non-modal floating window with header drag and corner
    resize, clamped to the viewport. Its first placement is right of
    `#convo`, not covering the source row.
  - The narrow sheet copies the drawer's sheet look (`.dr-sheet` /
    `.dr-handle`) but is its own element.
  - This is chosen over reusing `maximize()` (full-screen, and its scrim covers
    the conversation) or `gwBuild` (modal), and over reusing the drawer (a nav
    surface with its own open state).
- (d) [open] [technical] Key scope.
  - J/K/Enter act only while focus is inside `#convo` and not in an editable
    element.
  - ←/→, Esc and Ctrl+F act only while focus is inside an audit window. Ctrl+F
    overrides the browser's find only there.
  - Esc closes the focused window first, before any other Esc consumer. In
    the iframe shell the frame-local handler runs before the shell's Esc
    forwarding.
  - This is chosen over page-global keys, which collide with the composer,
    the link menu and the Files pane.
- (e) [open] [technical] Darkroom mechanics. The darkroom starts with the first
  window opened by a click in the wide view and ends when the last unpinned
  window closes. A click on the dimmed area ends it and closes unpinned
  windows, like Esc; pinned windows stay open and stay lit. The narrow sheet
  has no darkroom, since the sheet already covers the page. The alternative,
  keeping the darkroom while any window is open, makes the board unreachable
  while a pinned comparison is up.
- (f) [open] [technical] Fetching.
  - Opening a call row calls `GET .../audit/events?call=<id>`. Opening any tab
    calls `GET .../audit/event/<id>` once per event and caches it for the
    window's life.
  - A body with `truncated: true` shows a "load more" row that pages through
    the `range` follow-up.
  - Hidden-event rows page with the history: each `loadHistory` page also
    fetches `audit/events` bounded by that page's oldest and newest ring
    stamps.
  - This is chosen over loading the whole audit log up front, which can reach
    the 256 MiB per-session cap.
- (g) [open] [technical] Pin limit: at most 4 pinned windows. A fifth pin
  replaces the oldest pinned window and shows a toast. Each new pinned window
  is offset 24px from the last. Unbounded windows can bury the page.
- (h) [open] [technical] Kind chips default to all on, as the mockup shows.
  Their state persists per browser in localStorage
  (`ws.dashboard.auditkinds`), not per session. This matches the panel-fold
  precedent (`ws.dashboard.panelshut`).
- (i) [open] [technical] Token growth shows as the delta of `ctx_after`
  against the previous request's `ctx_after` in the same conversation.
  Parallel calls answered in one request show the delta on the first call and
  "·" on the others, so a shared growth is not counted several times. The
  alternative, the same delta on each, overstates the total.

## Constraints

- Implementation conventions (AGENTS.md):
  - `agents-plugin-tool/` -> `ai-docs/manuals/shipped-surface-boundary.md`
  - `agents-plugin-tool/internal/` -> `ai-docs/manuals/release-compatibility.md`
- The ring, `/history` and its rendered HTML stay valid for entries without
  call ids. Existing pill behaviour does not regress for them.
- The audit view must not slow history paging. Audit fetches run after the
  history page has rendered, and never block it.
- Every user-visible string is a `js.*` key in all 11 i18n tables.
- QA uses an isolated daemon (`WS_CACHE_HOME=<tmp> WS_CONFIG_HOME=<tmp>
  HOME=<tmp> <bin> dashboard --port 0`) with a fixture audit store. Never
  contact 127.0.0.1:29417, never run `scripts/dev.py`, and kill nothing you did
  not start.

## Phases

### Phase 1: Call rows and the audit window

- The template emits `data-call` per call.
- A folded tools entry with ids renders as the mockup's call list: three rows,
  then "+n".
- A click on a row opens the audit window, wide or narrow, with all five tabs
  and the long-output tools: line numbers, in-window search, wrap and copy.
- Also in this phase:
  - the seen-cut line
  - redaction chips
  - ←/→ through calls
  - Esc
  - pinning
  - the wide-view darkroom (Decision 2)
  - the harness badge on the window header

Verification:

- Node harness checks for row building, the key scopes and the darkroom state
  machine.
- A headless run on an isolated daemon with a fixture audit store, covering
  open, tabs, truncated "load more", the seen cut, a redaction, pin and
  darkroom.
- LOOK captures at 420 and 1200, light and dark, side by side with mockup v2.

### Phase 2: Hidden-event rows, kind chips and keys

- Dotted one-line rows for hidden events interleave by `at`, paged with
  history. Hooks and reminders attached to a call are indented under it.
- Kind chips sit at the top of the conversation.
- J/K/Enter move between rows and open them.
- The harness badge shows on the conversation header.
- The session-start row shows the system prompt when the stream carried one.
- "Other" rows open on their Raw tab.

Verification: interleaving order tests against ring stamps (equal-ns ties),
chip persistence, paging both ways, J/K skipping hidden kinds, and LOOK against
the mockup.

### Phase 3: Conversation-wide search

- A search box on the conversation pane runs `audit/search`, with text, regex
  and case options and a kind filter.
- Matching rows get an "<field> <count>" badge (e.g. "출력 3") through the
  hit's `call_id`.
- Opening a badged row opens the window on the first matching line, with the
  in-window search pre-filled.

Verification: headless search over the fixture store, covering a hit inside a
truncated body (it pages to the line), a regex hit, a hit on a subagent event
(it routes through `parent_id`), and the empty state.
