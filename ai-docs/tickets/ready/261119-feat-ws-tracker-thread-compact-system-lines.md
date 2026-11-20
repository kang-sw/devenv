---
title: tracker 스레드 시스템 메시지 compact 표시와 연속 구간 접기
related:
  261119-feat-ws-tracker-org-chart-editor: sibling tracker UI work from the same owner session; independent, no ordering
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 53c5ec9fa0c9105a
sage-review-completeness-reviewed: 53c5ec9fa0c9105a
---

# Tracker thread: compact system lines and collapsible runs

## Background

In a tracker item thread, system events (assignment, CC, field changes and
similar non-comment events) use 12.5px type and 6px vertical padding, close to
the 13px comment text, so long runs of system events bury the conversation
(`agents-plugin-tool/internal/wsdashboard/assets/tracker.css#L605-L618`). The owner
asked (2026-11-19 10:13) that system messages be visually compact and distinct
from comments, and that consecutive system messages can be folded.

Today the thread renders one row per event with no grouping:
`windowHtml` maps each event to `commentHtml` or `evLine` in
`agents-plugin-tool/internal/wsdashboard/assets/tracker-window.js#L628-L632`;
`evLine` is defined in `agents-plugin-tool/internal/wsdashboard/assets/tracker.js#L648-L653`.
The fragments form one renderer (`agents-plugin-tool/internal/wsdashboard/asset_source_test.go#L22-L22`).

Approved mockup (owner check (1389)=ok, 2026-11-19 10:31):
`ai-docs/assets/mockups/261119/tracker-thread-compact/thread.html` (root class
`.tktc`).

## Decisions

1. **Compact system lines.** Non-comment events render as compact lines:
   smaller row height and font size than comments, muted colour, as in the
   mockup. Comments keep their current look. The approved mockup is the
   visual contract for the thread, including what Decisions 2-5 do not name:
   the collapsed summary line with author, per-kind counts and time range, the
   per-kind glyphs (+ CC, → assignment, ✎ edit), the name hidden when the same
   author continues (`.sl.same`), HH:MM-only times inside runs, and the
   thread-header expand-all / collapse-all toggle ("기록 모두 펼치기/접기").
2. **Run definition.** A run is 2 or more consecutive non-comment events
   between comments. A comment is exactly what `windowHtml` renders as a
   comment card (`n.type === 'comment.add' && n.thread === it.id`); every event
   that renders through `evLine` is a non-comment event and never a boundary.
   A lone non-comment event between comments renders as a single compact line,
   not a run. Rejected: grouping by time window — the owner asked for
   "consecutive" events, and time windows split or merge runs unpredictably.
3. **Default fold.** A run of 4 or more events renders collapsed to a single
   summary line ("시스템 기록 N건" in
   `ai-docs/assets/mockups/261119/tracker-thread-compact/thread.html#L91-L96`)
   that expands on click; runs of 2-3 render expanded. Every run, whatever
   its default, has a header toggle that folds and unfolds it both ways, as in
   the mockup. Rejected: always
   collapsed — short runs carry little
   noise and folding them costs a click.
4. **Runs that stay open.** A run containing an event with fold status
   `flagged` (`n.status === 'flagged'`, the flagged/ignored chip) or with
   concurrency markers (`n.markers.length > 0`, the `concm` class and the
   concurrent chip) renders expanded by default, so problems are never
   hidden. Rejected: also keeping open runs with an `item.reject` event — it is
   a normal workflow step (an assigner rejecting a completion), not a problem
   state; also keeping open runs with a `waiting` event — it resolves on its
   own.
5. **Status changes break runs.** `item.status` events are not folded: they
   end the current run, render as a visible compact line, and a new run may
   start after them. The mockup does not draw this case; render it as a compact
   line with status emphasis consistent with the mockup's palette. Other
   lifecycle events (`item.close`, `item.reopen`, `item.confirm`,
   `item.reject`, assignment events) fold inside runs like any non-comment
   event, as does a child's `item.create` shown in a parent thread although
   it carries a status chip. Rejected: breaking runs on those lifecycle events
   too — `item.status` is the thread's status-change event, and a wider break
   set keeps runs too short to fold.
6. **Fold state is in memory only.** Expanded/collapsed state per run lives in
   page memory and resets on reload. A run is identified by its first event id;
   memory holds only the user's explicit toggles, and a run with no toggle
   recomputes its default (Decisions 3-4) on each render, so a run that grows
   past 3 events or gains a flagged event while open follows the new default
   until the user toggles it. The thread-header expand-all / collapse-all
   toggle writes an explicit toggle for every run shown at that moment; a run
   that appears later follows its default. Rejected: persisting fold state — it would
   add file or store writes on an interactive path for little value
   (`ai-docs/manuals/performance.md`).
7. **Compatibility: T0.** Rendering only; no event, store, route or API shape
   changes.
8. **Item-window thread only.** Compact lines and runs apply to the item-window
   thread (`windowHtml`); the activity feed (`activityBody`) and the
   `tracker-wiki.js` activity list keep their current `evLine` rows. Rejected:
   applying them wherever `evLine` renders — the owner's request and the mockup
   cover the item thread, and the activity feed is a filtered, day-grouped log
   where folding would hide the rows its filters select.

## Constraints

- Applicable manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/performance.md`, `ai-docs/manuals/release-compatibility.md`
  (T0 statement only).
- Every new user-visible string goes into all locale tables under
  `agents-plugin-tool/internal/wsi18n/tables/`.
- Every new `js.tk.*` key also changes the generated
  `ai-docs/ref/tracker-mockup/i18n-en.js`; regenerate it with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local WS_REGEN_TRACKER_MOCKUP_I18N=1 go test -count=1 -run TestTrackerMockupI18nIsCurrent ./internal/wsdashboard`
  from `agents-plugin-tool/` (`agents-plugin-tool/internal/wsdashboard/tracker_mockup_i18n_test.go#L15-L45`).
- Test environment (amended 2026-11-20, work moved to the macOS dev
  machine): Go tests run locally and narrowly with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 -run '<pattern>' ./internal/<pkg>`
  from `agents-plugin-tool/`; never the whole `./internal/wsdashboard` or
  `./internal/mcp` package. Node tests run only for touched dashboard test
  files. No Electron.
- Do not start, stop, rebuild or swap the shared ws daemon. The LOOK capture
  uses an asset-level fixture page in headless Chromium that loads the real
  tracker assets with fixture events, not the live daemon.
- The existing fixture page is `ai-docs/ref/tracker-mockup/index.html`; serve it
  over HTTP because its fragment fetches do not work from `file://`
  (`ai-docs/ref/tracker-mockup/index.html#L81-L99`).
- Temp files only under a per-purpose directory in the system temp dir.

## Prior Decisions

- 261108-feat-ws-tracker-drawer-ui (2026-11-08, Result): "`wsi18n.SubsetScript(lang, prefix)` builds the mockup's `i18n-en.js` from the same builder as `Script`; `TestTrackerMockupI18nIsCurrent` fails on drift" — bearing: constrains
- 25f1d159 (2026-11-17, commit): "Keep embedded and development readers on the assembled single-IIFE source and preserve its asset identity." — bearing: constrains
- af93bfe5 (2026-11-15, commit): "Regeneration is the test's documented fix and is cheap." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/tracker-window.js, tracker.js, tracker.css; agents-plugin-tool/internal/wsi18n/tables/*.json; ai-docs/ref/tracker-mockup/i18n-en.js; dashboard tests |
| scope.surface | internal | assembled dashboard renderer and CSS; no exported symbol change planned |
| scope.new_public_symbol | no | none in Decisions 1-8 or Phase 1 |
| scope.new_type_contract | no | rendering only; no event, store, route, or API shape change (Decision 7) |
| scope.test_surface | new-files | Phase 1 calls for a pure-function Node grouping test; existing tracker-projection.browser.cjs does not cover run grouping |
| complexity.reuse_points | confirmed | windowHtml maps events to commentHtml/evLine (tracker-window.js#L628-L632); fixture page assembles the same renderer (ai-docs/ref/tracker-mockup/index.html#L84-L99) |
| complexity.side_effect_risk | moderate | grouping and in-memory fold state change item-thread visibility and click behavior |
| risk.correctness | high | flagged or concurrency-marked events and `item.status` changes must remain visible across run boundaries (Decisions 4-5) |
| risk.fit | moderate | compact rows, summary states, and status emphasis must match thread.html; status-change break has no mockup case |
| risk.test | moderate | grouping is unit-testable, but LOOK requires collapsed, expanded, flagged, and status-break fixture states |
| risk.security_or_contract | low | Decision 7 changes only rendering and memory state, with no protocol or persisted data shape |

## Phases

### Phase 1: Compact system lines and foldable runs

Implement Decisions 1-8 in the item-window thread built by `windowHtml`
(`tracker-window.js`), whose non-comment rows come from `evLine`
(`tracker.js`); the fragments assemble into one renderer
(`agents-plugin-tool/internal/wsdashboard/asset_source_test.go#L22-L22`).
Keep the run grouping a pure function over the thread's events so the Node
test can load it. Decision 8 means `evLine`'s output for the activity feed and
the `tracker-wiki.js` list must not change: the compact form may be a
thread-only renderer, an `evLine` variant selected by the thread caller, or
CSS scoped to the item window — the implementer picks the shape. The LOOK
states (a collapsed run, an expanded run, a run with a `flagged` event, an
`item.status` break) need fixture events in an item thread of
`ai-docs/ref/tracker-mockup/fixtures.js`; the fixture already has
`item.status` events but add whatever is missing there.

Verification:
- A pure-function test (node) for run grouping: comment boundaries, the 2+ run
  threshold, 4+ default collapse, a `flagged` or concurrency-marked event
  keeping a run open (an `item.reject` or `waiting` event does not), and
  `item.status` events breaking runs (other lifecycle events and a child's
  `item.create` with a status chip do not), and fold state keyed by the first
  event id (an untoggled run recomputes its default as it grows; an explicit
  toggle survives re-render).
- Existing dashboard Go tests covering the touched assets pass (narrow `-run`
  patterns).
- LOOK: capture the fixture thread (collapsed and expanded states, a run with
  a `flagged` event, an `item.status` break, and the state after the header
  expand-all toggle) and compare it side by side with the
  approved mockup; report differences and fix any that are not justified by
  real data.
