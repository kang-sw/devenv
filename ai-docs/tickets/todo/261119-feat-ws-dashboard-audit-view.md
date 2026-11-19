---
title: "대시보드 대화 감사 뷰: 호출 행, 숨은 이벤트, 감사 창, 대화 전체 검색"
parent: 261119-epic-ws-conversation-audit
blocked-by:
  - 261119-feat-ws-audit-query-api
  - 261119-feat-ws-audit-capture-harness-adapters#phase1
related:
  261119-feat-ws-audit-query-api: "prerequisite: every byte this view shows comes from that API (599)"
  261119-feat-ws-audit-capture-harness-adapters: "prerequisite #phase1: the ring call ID (601), the join key from a history row to an audit event"
  261119-feat-ws-dashboard-shell-topbar-rail-redesign: "landed shell redesign; the mockup's surroundings realign to it"
  261114-feat-ws-dashboard-board-toolbar-overview: "sibling darkroom precedent: the board overview dims the shell through layout.dim"
  261113-feat-ws-dashboard-iframe-shell: "session pages are shell frames; layout.dim and the shell's Esc and Ctrl+K forwarding shape the key rules"
  261114-feat-ws-dashboard-conversation-audit-view: "superseded source of (550), (602)-(610), (681) and the approved mockup v2 (asset folder kept in place)"
---

# Dashboard conversation audit view

## Background

Fifth child of `261119-epic-ws-conversation-audit` ((1331)=a, 2026-11-19).
It carries the UI ticket `261114-feat-ws-dashboard-conversation-audit-view`,
dropped as superseded, with its approved interaction unchanged.

The owner approved mockup v2 with "550=ok" and added one rule: in the wide
view, a click dims everything except the conversation pane and the floating
window. The approved mockup stays at its tracked path:
`ai-docs/tickets/assets/261114-feat-ws-dashboard-conversation-audit-view/conversation-audit-mockup-v2.html`.
Its notes panel lists the behaviour this ticket builds:

- **Conversation list.** A folded tool batch becomes a call list. Each call is
  one row: status, tool, argument preview, time, and token growth. Three rows
  show and the rest fold under "+n".
  - Every call row and every hidden-event row is clickable and opens the audit
    window. J/K move between rows; Enter opens.
  - A hidden event folds to one dotted line: a kind-coloured dot and a
    preview. Chips at the top show or hide each kind. Hooks and reminders
    attached to a call are indented under that call.
  - Conversation-wide search covers the stored outputs too. A matching row
    gets a badge such as "출력 3", and the audit window opens on that match.
- **Audit window.**
  - Tabs: Input, Output, Meta, Raw, Linked events. Raw is the harness's
    recorded JSON after the store's pre-write redaction (596), never the
    original secret.
  - Long outputs get line numbers, an in-window search (Ctrl+F, with Enter and
    Shift+Enter for next and previous), a wrap toggle, and copy.
  - What the model saw vs what was stored: the line where the harness
    truncated what it delivered is marked, and everything below it is the
    part the model did not see.
  - A redaction is a hatched chip showing 🔒, the kind and the length. The
    original is never stored; only the rule name shows.
  - ←/→ replace the window's content with the previous or next call, and Esc
    closes it. 📌 pins the window, so the next open makes a new one, for
    comparing several.
- **By width.**
  - Wide: a floating window, dragged by its header and resized from its
    corner. It opens to the right of the conversation without covering the
    source row, and the source row keeps its selected mark.
  - Narrow: a bottom sheet at 72% height, with tabs that scroll sideways and a
    one-column Meta.
- **Harness-neutral ((545)=a, (546)=a).** A harness badge shows on the
  conversation header and the window header. Every harness maps onto the same
  event kinds, so it gets the same list and the same window. The system
  prompt shows on the session-start row only when the stream carries it. An
  unknown kind becomes an "other" row with its Raw tab.

Stale since the superseded ticket (codex.large read-only research,
2026-11-19):

- The dashboard script and styles are no longer one `dashboard.js`. They are
  ordered fragment files that the daemon assembles into one script and one
  stylesheet (`dashboardScriptFiles` in `wsdashboard/pages.go`), so the
  standalone `assets/audit.js` / `audit.css` assumption of (603) needs
  realigning.
- The mockup's surroundings (rail, header, session chrome) predate the
  landed shell topbar/rail redesign
  (`261119-feat-ws-dashboard-shell-topbar-rail-redesign`, approved mockup
  `ai-docs/assets/mockups/261119/shell-redesign/shell-redesign-v4.html`).
- The superseded ticket's "what exists today" code coordinates (handlers,
  breakpoints, reuse candidates) are not carried. Re-populate them at ready
  promotion.

## Decisions

1. **Mockup v2 is the approved visual reference for the audit interaction
   ((550)=ok).** Verification compares result captures side by side with it,
   in the wide and the narrow view.
2. **Realigned surroundings, unchanged interaction ((1331)=a realignment).**
   Before ready promotion, produce a realigned copy of mockup v2 at
   `ai-docs/assets/mockups/<YYMMDD>/conversation-audit/`. It replaces only the
   surroundings with the current shell (the redesign's rail, topbar handles
   and existing `.hb` harness badge) and leaves the audit list, window,
   sheet, chips and darkroom exactly as v2 shows them. v2 stays linked at its
   tracked path as the approved interaction. The owner confirms the realigned
   copy as the visual-reference update. Captures are then compared side by
   side with both: the realigned copy for placement, v2 for the interaction.
3. **Wide-view darkroom ((550) note).** In the wide view, opening the audit
   window from a click dims everything except the conversation pane and the
   floating window. In the shell, the rail and header dim through the shell's
   existing dim mechanism (`layout.dim`, as the board overview does; confirm it
   survives the redesign at ready).
   - Rejected: dimming nothing (the owner asked for it), and dimming the
     conversation pane too (the source row must stay readable beside the
     window).
4. **Every byte comes from the query API ((599)).** That covers events, the
   full envelope, search, and the `?call=` lookup. The UI parses no harness
   transcript file and adds no route of its own. Fields used:
   - On a summary: `duration_ms`, `ctx_after`, `attached`, `model_seen`.
   - In the full envelope: `seen`, `redactions`, `truncated` with the `range`
     follow-up, and `meta.signature_only`.
   - On a search hit: `call_id`, `parent_id`.
5. **The history-to-audit join key is the ring's call ID ((601)).** The
   capture child gives `ToolCallArgs` an optional `id` equal to the store's
   `call_id`. Calls past `BatchCallCap` in a huge batch get no ID. This ticket's
   template emits the ID as `data-call` on each call row; a call without one
   follows (602).
   - Rejected: joining by time window between ring stamps (ambiguous with
     parallel calls and subagents).
6. **Compatibility tier T0.** Dashboard asset content, a template change that
   adds attributes, and new i18n keys. No route, MCP tool, store or tracker
   event changes here.

### Approved technical contracts (602–610)

Authority: owner `594..609,611=ok`, recorded 2026-11-14; (610) was separately
answered `ok` the same day. (611) is the UI group acknowledgment, not a
separate implementation choice.

- **(602) Fallback without a call ID.** Ring entries without call IDs keep
  today's folded pill instead of turning into rows: entries written before
  the capture change, or by a harness the adapters do not cover. In a batch
  where only the calls past `BatchCallCap` lack IDs, those calls fold into the
  "+n" row and are not clickable. Only rows with a `data-call` are clickable,
  and hidden-event rows still interleave by `at`. Rejected: a time-window
  guess (Decision 5).
- **(603) New files over growing existing ones (realigned).** The audit view's
  script and styles live in new files of their own, with classes prefixed
  `au-` so the CSS collision guard
  (`TestCSSClassesDoNotCollideAcrossModules`) passes. The original form, a
  standalone `assets/audit.js` `WsAudit` module mounted like `WsFiles.mount`
  plus `assets/audit.css`, predates the fragment assembly. Choose at ready
  between a new assembled fragment pair and a separately loaded module,
  following how comparable surfaces load today.
- **(604) Window and sheet are built new.**
  - The wide window is a non-modal floating window with header drag and
    corner resize, clamped to the viewport. Its first placement is right of
    the conversation pane, not covering the source row.
  - The narrow sheet copies the drawer's sheet look but is its own element.
  - Rejected: reusing the card `maximize()` (full-screen, and its scrim covers
    the conversation), the commit window (modal), or the drawer itself (a nav
    surface with its own open state).
- **(605) Key scope.**
  - J/K/Enter act only while focus is inside the conversation pane and not in
    an editable element.
  - ←/→, Esc and Ctrl+F act only while focus is inside an audit window. Ctrl+F
    overrides the browser's find only there.
  - Esc closes the focused window first, before any other Esc consumer. In
    the shell, the frame-local handler runs before the shell's Esc forwarding.
  - Rejected: page-global keys, which collide with the composer, the link
    menu and the Files pane.
- **(606) Darkroom mechanics.** The darkroom starts with the first window
  opened by a click in the wide view and ends when the last unpinned window
  closes. A click on the dimmed area ends it and closes unpinned windows, like
  Esc; pinned windows stay open and stay lit. The narrow sheet has no
  darkroom, since the sheet already covers the page. Rejected: keeping the
  darkroom while any window is open, which makes the board unreachable while a
  pinned comparison is up.
- **(607) Fetching.**
  - Opening a call row calls `GET .../audit/events?call=<id>`. Opening any tab
    calls `GET .../audit/event/<id>` once per event and caches it for the
    window's life.
  - A body with `truncated: true` shows a "load more" row that pages through
    the `range` follow-up.
  - Hidden-event rows page with history, using (681)'s UTC filters. After a
    ring history page renders, fetch `audit/events` with `since` equal to that
    page's oldest stamp and `until` equal to the oldest stamp of the
    immediately newer loaded page; omit `until` for the latest/live page.
    These half-open intervals include gaps between ring rows without
    double-owning a boundary. Follow event-ID cursors within the same interval
    until `end`, deduplicate by event ID, and order equal-nanosecond events by
    sequence. Refresh the unbounded latest interval as new events arrive. UTC
    selects hidden rows; it never guesses a call join.
  - Rejected: loading the whole audit log up front.
- **(608) Pin limit.** At most 4 pinned windows. A fifth pin replaces the
  oldest pinned window and shows a toast. Each new pinned window is offset
  24px from the last. Unbounded windows can bury the page.
- **(609) Kind chips** default to all on, as the mockup shows. Their state
  persists per browser in localStorage (`ws.dashboard.auditkinds`), not per
  session, matching the panel-fold precedent (`ws.dashboard.panelshut`).
- **(610) Token growth** shows as the delta of `ctx_after` against the
  previous request's `ctx_after` in the same conversation. Parallel calls
  answered in one request show the delta on the first call and "·" on the
  others, so a shared growth is not counted several times. Rejected: the same
  delta on each, which overstates the total.

### Reconciliation

- **(681), 2026-11-14, T0:** the API's optional `since`/`until` UTC filters,
  `[since, until)`, keep event-ID pagination and every response field. (607)
  uses them only to select hidden events; (601)'s ID join is unchanged.
- **(682)** is superseded by this epic's dependency chain: this view waits for
  `261119-feat-ws-audit-query-api` and the capture child's Phase 1 instead of
  the superseded backend's Phase 2. The foundation interface Phase 2 it also
  named is done.

## Constraints

- Implementation conventions (AGENTS.md):
  - `agents-plugin-tool/` -> `ai-docs/manuals/shipped-surface-boundary.md`
  - `agents-plugin-tool/internal/` -> `ai-docs/manuals/release-compatibility.md`
- Ready promotion requires the realigned mockup of Decision 2, confirmed by
  the owner.
- The ring, `/history` and its rendered HTML stay valid for entries without
  call IDs. Existing pill behaviour does not regress for them.
- The audit view must not slow history paging. Audit fetches run after the
  history page has rendered and never block it. Preserve event-ID cursors
  within (681)'s UTC bounds and the ID-only call join (601).
- Every user-visible string is a `js.*` key in every `wsi18n/tables/<lang>.json`
  table.
- Responsive behaviour follows "By width" above: floating window in the wide
  view, 72% bottom sheet in the narrow view, at the current shell's
  breakpoints.
- QA uses an isolated daemon (`WS_CACHE_HOME=<tmp> WS_CONFIG_HOME=<tmp>
  HOME=<tmp> <bin> dashboard --port 0`) with a fixture audit store. Never
  contact 127.0.0.1:29417, never run `scripts/dev.py`, and kill
  nothing you did not start.

## Phases

### Phase 1: Call rows and the audit window

- The template emits `data-call` per call.
- A folded tools entry with IDs renders as the mockup's call list: three rows,
  then "+n".
- A click on a row opens the audit window, wide or narrow, with all five tabs
  and the long-output tools: line numbers, in-window search, wrap and copy.
- Also in this phase:
  - the seen-cut line
  - redaction chips
  - ←/→ through calls
  - Esc
  - pinning
  - the wide-view darkroom (Decision 3)
  - the harness badge on the window header

Verification:

- Node harness checks for row building, the key scopes and the darkroom state
  machine.
- A headless run on an isolated daemon with a fixture audit store, covering
  open, tabs, truncated "load more", the seen cut, a redaction, pin and
  darkroom.
- LOOK captures at one narrow and one wide width chosen from the current
  breakpoints, light and dark, side by side with the realigned mockup
  (placement) and mockup v2 (interaction). The superseded ticket used 420 and
  1200, which predate the redesign.

### Phase 2: Hidden-event rows, kind chips and keys

Depends on this ticket's Phase 1.

- Dotted one-line rows for hidden events interleave by `at`, paged with
  history using (607)/(681)'s UTC selection and event-ID continuation.
  Hooks and reminders attached to a call are indented under it.
- Kind chips sit at the top of the conversation.
- J/K/Enter move between rows and open them.
- The harness badge shows on the conversation header.
- The session-start row shows the system prompt when the stream carried one.
- "Other" rows open on their Raw tab.

Verification: interleaving order tests against ring stamps (equal-ns ties),
adjacent ring-page intervals and timestamp gaps, more than one API page in an
interval, event-ID deduplication and latest-interval live refresh after
history rendering. Cover chip persistence, paging both ways, J/K skipping
hidden kinds, and LOOK side by side with both mockups.

### Phase 3: Conversation-wide search

Depends on this ticket's Phase 2.

- A search box on the conversation pane runs `audit/search`, with text, regex
  and case options and a kind filter.
- Matching rows get an "<field> <count>" badge (e.g. "출력 3") through the
  hit's `call_id`.
- Opening a badged row opens the window on the first matching line, with the
  in-window search pre-filled.

Verification: headless search over the fixture store, covering a hit inside a
truncated body (it pages to the line), a regex hit, a hit on a subagent event
(it routes through `parent_id`), the empty state, and LOOK side by side with
both mockups.
