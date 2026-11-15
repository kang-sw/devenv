---
title: Dashboard model history stays stale after an entries region update
related:
  261115-refactor-ws-dashboard-wake-reads: found by its Chromium smoke; not caused by it
  261115-feat-ws-dashboard-observed-session-model: renders the model-history data this block shows
  261114-refactor-ws-dashboard-whiteboard-asset-split: source of the split dashboard-conversation.js asset
  261107-feat-ws-dashboard-conversation-history: owns mergeEntriesRegion history reload behavior
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: be692a153711367b
sage-review-completeness-reviewed: be692a153711367b
completed: 2026-11-15
---

# Dashboard model history stays stale after an entries region update

## Background

The session conversation pane renders the `r-entries` region
(`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl:256`) as
three siblings: the `.thead` title bar, an optional
`<details class="modelhistory" data-key="model-history">` block (rendered only
when `.ModelHistory` is non-empty), and the `.conv.centries` entry box.

A live update of that region goes through `mergeEntriesRegion`
(`assets/dashboard-conversation.js:71`, dispatched from
`assets/dashboard-board.js:66`). It replaces `.thead`, merges `.centries`, and
never touches `.modelhistory`. So after a region event:

- an existing model-history block keeps its old rows (stale);
- a block that first appears on the server side (model history was empty at
  page load) never appears;
- a block that disappears on the server side stays on screen.

Only a full page reload, or the fallback `el.innerHTML = html` path taken when
either entry box is missing, shows the current history. This is user-visible.
The wake-reads worker found it with a disposable Chromium smoke
(`/tmp/exec5/wakereads/README.md`, `wake-chromium.txt`). That change did not
cause it; the bug predates it.

## Decisions

- D1. `mergeEntriesRegion` syncs the model-history block from the region
  payload. If both the old and the new block exist, the new one replaces the
  old. If only the new one exists, it is inserted at the template position
  (after `.thead`, before `.centries`). If only the old one exists, it is
  removed. The entry merge, gap reset, pin eviction and history reload behave
  as they do today.
  Rejected: replacing the whole region with `innerHTML` on every event. That
  discards merged entries, scroll position, loaded older history and live
  frames, which is why the merge path exists. Also rejected: a separate
  server-side region for model history. That changes the stream region set,
  a protocol surface, for a client-only omission.
- D2. The `<details>` open state survives the sync. Use the open-state
  mechanism that already serves keyed `<details>`; the block carries
  `data-key="model-history"`, and the existing `restoreOpen`
  (`dashboard.js:229`) is the reference. An open block stays open after a
  region event, and a closed one stays closed. Times inside the new block are
  formatted with the same helper the replaced `.thead` uses (`fmtTimes`).
  Rejected: re-rendering closed, which would collapse the block on every
  update while the user is reading it.
- D3. Browser-only change; no Go render, template, or stream payload change.
  Compatibility tier T0: no cross-version surface is touched.
- D4. The `TestSelectionNoteBrowser` nit is in scope only if trivial: the
  reported hidden-textarea wait is already absent in this tree; the test waits
  for the visible `#ctext + .wsc-editor`
  (`agents-plugin-tool/internal/wsdashboard/testdata/selection-note.browser.cjs#L23-L27`). Trivial
  means a test-only fix of a few lines with no product change. If it is not
  trivial, leave it and say so in the Result.

## Constraints

- Implementation conventions: `ai-docs/manuals/shipped-surface-boundary.md`
  (path `agents-plugin-tool/`). No shipped text changes are expected.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- The dashboard bundle `dashboard.js` + `dashboard-*.js` is one IIFE
  (`pages.go` `dashboardScriptFiles`); helpers are shared by name across
  files.
- Tests: `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; never run the whole
  `./internal/wsdashboard` package, always `-run` with a focused pattern.
  Known pre-existing red: `TestTrackerMockupI18nIsCurrent`.
- Do not touch the shared dashboard daemon. Runtime checks run only in
  disposable test fixtures.

## Prior Decisions

- 261115-refactor-ws-dashboard-wake-reads (2026-11-15, Result): "entries merge ignores sibling .modelhistory, leaving its history stale after SSE" — bearing: supports
- 261115-feat-ws-dashboard-observed-session-model / 4dc1e7c3 (2026-11-15, commit): "Immutable metadata sidecars survive old typed snapshots, with original producer/time deduplication." — bearing: constrains
- 261114-refactor-ws-dashboard-whiteboard-asset-split / 961c8e3b (2026-11-15, commit): "Concatenate contiguous JavaScript source fragments without separators into the original single IIFE" — bearing: constrains
- 261107-feat-ws-dashboard-conversation-history (2026-11-07, Decisions): "an entries region the page merges by key" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard-conversation.js plus a new *_browser_test.go fixture and testdata/*.browser.cjs driver in the same package |
| scope.surface | internal | mergeEntriesRegion is private to the assembled dashboard IIFE; D3 excludes render/template/stream changes |
| scope.new_public_symbol | no | no exported symbol or browser API requested |
| scope.new_type_contract | no | D1/D2 reuse existing region HTML, model-history data-key and details state |
| scope.test_surface | new-files | Phase 1 requests a Chromium regression fixture/driver following selection_note_browser_test.go and board_units_browser_test.go |
| complexity.reuse_points | confirmed | dashboard.js#L197-L231 openKey/toggle/restoreOpen; dashboard-conversation.js#L71-L82 mergeEntriesRegion; pages.go#L37-L89 ordered single-IIFE assembly |
| complexity.side_effect_risk | moderate | replacing a sibling disclosure must preserve open state and leave keyed entries, gap reset and pinned eviction untouched |
| risk.correctness | moderate | add/replace/remove ordering and disclosure restoration must coexist with retained entry nodes and history cursors |
| risk.fit | low | dashboard.tmpl#L256 already supplies the keyed sibling; D3 confines the fix to its existing client merge owner |
| risk.test | moderate | new opt-in Chromium coverage must exercise region delivery and retained node identity; existing source only proves the omission, not the proposed fix |
| risk.security_or_contract | low | D3 changes no producer, payload, store or API; the client consumes the same server-rendered HTML through existing frag/merge helpers |

## Phases

### Phase 1: Sync the model-history block on entries region events

Change `mergeEntriesRegion` per D1/D2, then add regression coverage that fails
before the change:

- A Chromium browser test, following the pattern of the existing
  `*_browser_test.go` fixtures and their `testdata/*.browser.cjs` drivers,
  that applies entries region events to a session conversation and asserts:
  (a) changed model-history rows appear after the event;
  (b) a block absent at load appears once the payload carries one, between
  `.thead` and `.centries`;
  (c) a block removed from the payload disappears;
  (d) an open block stays open across the event;
  (e) existing entry nodes in `.centries` keep their identity (the merge
  still merges and does not re-render);
  (f) there are no page errors.
- Run the existing conversation/entries tests, including
  `conversation_history_test.go` and `reasoning_summary_test.go`, with a
  focused `-run` pattern, plus the board region/frame tests
  (`BoardKeeps|FrameReuse|BoardTopic`).

Chromium: `WS_PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright`,
`PLAYWRIGHT_BROWSERS_PATH=/Users/kang-sw/Library/Caches/ms-playwright`
(machine-local; skip with a recorded reason when unavailable).

### Result (80ca8fd13) - 2026-11-15

- Synced the optional model-history sibling on entries region updates: insert
  before `.centries`, replace existing history, or remove absent history.
  Reused `restoreOpen` and `fmtTimes`; entry merging, gap reset, pin eviction,
  history loading and retained entry nodes remain unchanged. T0 browser-only:
  no template, producer, store, protocol/API, or version changes.
- Added `TestModelHistoryEntriesBrowser` and its Chromium driver. Normal fixture
  region delivery covers changed rows, initially absent history, removal,
  open/closed disclosure state, sibling order, formatted timestamps, retained
  container/entry identity, and no page errors at 1440px and 420px.
- Verified the new test fails before the production fix (zero history blocks
  after an entries update), then passes after it, including three consecutive
  runs at both widths. The fixture initially assumed unchanged entries would
  arrive on connect; source inspection confirmed held hashes suppress them,
  so it now waits for the initial mode region before applying updates.
- Focused offline dashboard verification passed 34 tests, including every test
  in `conversation_history_test.go`, the non-capture reasoning-summary tests,
  keyed-entry merge coverage, `BoardKeeps|FrameReuse|BoardTopic`, and
  `TestSelectionNoteBrowser`. The optional `TestReasoningSummaryVisualFixture`
  skipped because no offline capture output was requested. The selection-note
  visible-editor wait was already correct; no change was needed.
- Read the lite correctness/test-integrity checklist and performed its one-pass
  review directly; no findings remain. Independent review/exploration was
  omitted because the caller prohibited subagents. Only disposable test
  fixtures ran; the shared daemon and installed binaries were untouched.
- Evidence: `/tmp/exec5/mh/baseline-red.txt`, `browser-green.txt`,
  `browser-repeat.txt`, `focused-tests.txt`, `review.md`, and the 1440px/420px
  `model-history-*.png` captures. No approved visual reference was linked.
