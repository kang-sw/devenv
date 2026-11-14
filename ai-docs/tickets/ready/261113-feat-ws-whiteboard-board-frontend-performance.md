---
title: "Whiteboard board frontend performance: lazy frames, cheaper swaps, measurement harness"
related:
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "this ticket absorbs its board scope ((417) note typing, long whiteboard) and the (473) Mac/Windows measurement"
  261113-feat-ws-dashboard-iframe-shell: "independent; Phase 1-2 land without it; Phase 3 waits for its Phase 2 (fan-out cache, sse.go)"
  261113-feat-ws-dashboard-board-reorder-hold: "prerequisite for Phase 2 only: its Phase 1 builds the shared visibility observer in wswhiteboard/assets/stream.js"
  261113-feat-ws-whiteboard-topic-history-load-more: "shares the visibility observer with Phase 2; its worker decides template- vs fetch-based older pages"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 8016b4b29deea59c
sage-review-completeness-reviewed: 8016b4b29deea59c
---

# Whiteboard board frontend performance: lazy frames, cheaper swaps, measurement harness

## Background

Owner answer (532)=a on MAIN's board, 2026-11-13: build board frontend
performance work now, landing before `261113-feat-ws-dashboard-iframe-shell`
where independent. Source: a read-only static analysis by MAIN, copied to
`ai-docs/tickets/assets/261113-feat-ws-whiteboard-board-frontend-performance/static-analysis.md`
(section numbers below refer to it). Owner note: memory is not a big concern;
prioritise paint, layout and CPU.

Main findings (static, unmeasured in a browser):

- Every panel record, every panel version, and each topic body on the
  dashboard board becomes a sandboxed `srcdoc` iframe (`render.go:458-487`,
  `framedDoc` `render.go:547-571`), each inlining ~85 KB of CSS and JS
  (`page.css` + `svg-view.js`, `chg.js`, `code.js`, `doc-frame.js`). Frames
  under a closed `<details>` are still created; only ~4 per open topic are
  ever visible (section 1.1).
- Each frame height message costs a full-document frame lookup
  (`dashboard.js:3242-3244`) and a forced host layout
  (`getBoundingClientRect`, `:3345`): O(F^2) query work plus F layouts
  (section 1.4 item 7).
- Every swap re-serializes each incoming topic with `outerHTML`
  (`dashboard.js:694`, compared at `:701`), and a changed topic rebuilds all
  its frames except the first (`frameGraftable`, `:790-800`).
- Typing in a board note runs document-wide queries in `form.js` `build()`
  (`form.js:151-181`) and `pill.js` `update()` rect reads on `input`
  (`pill.js:125-141, 162-163`), with two forced layouts per keystroke
  (section 1.7, the (417) symptom); `tickTimes` queries the whole document
  every second (`dashboard.js:91-97`).

## Decisions

1. **Scope and order** ((532)=a, owner 2026-11-13): Phase 1 = the S fixes
   plus a measurement harness; Phase 2 = viewport mount/unmount of frames and
   `content-visibility` on topics; Phase 3 = incremental board events after
   iframe-shell Phase 2. Rejected: waiting for the iframe shell (the fixes are
   board-internal, and landing them first shrinks the shell's 5-frame LRU
   footprint, section 4); true list virtualization (section 3, last row:
   fights rail, pill, boardJump, pins, frameGraft and FLIP).
2. **Lazy frame mount** ((532)=a): the server emits `data-srcdoc` instead of
   `srcdoc` for frames; the client sets `srcdoc` from it (keeping
   `data-srcdoc` on the frame for Decision 6's compare) when the frame's whole
   `<details>` chain is open, at each of these points:
   - Initial page load: the server-rendered board (`SessionView.ThemedBoard`,
     `pageview.go:148`) gets one mount pass after `WsCards.mount`
     (`dashboard.js:624`) has moved pinned topics into the pin list, so a
     pinned topic's frames are not loaded and then moved.
   - `toggle`: through a document-level capture listener, since `toggle` does
     not bubble (the open-state listener at `dashboard.js:125-131` is already
     capture; the mount hook joins it or registers its own with `true`).
     Programmatic opens fire `toggle` as well - `restoreOpen`
     (`dashboard.js:132-140`) after a swap and `boardJump`'s `p.open = true`
     (`dashboard.js:3253`) - so they are covered without a separate call.
   - Insert in `morphBoard`, `adopted` (`dashboard.js:639-645`) and
     `wbhistoryload` (`dashboard.js:3269`).

   Heights stay keyed by `data-doc` (`frameHeights` / `setFrameHeights`). The
   static full page (`framed=false`) is unchanged.

   **Revision (Phase 1 stop, 2026-11-14, EXEC lead, technical): an unmounted
   frame is an inert placeholder, not an `<iframe>`.** Measured at 4x, an
   `<iframe>` carrying only `data-srcdoc` still commits an `about:blank`
   document synchronously while the page parses (a devtools trace of one cold
   load: 65 `CommitLoad about:blank`, a 507 ms parse task; scenario (a) long
   tasks 3 -> 5 and long-task time 435 -> 1201 ms, all 12 samples 1071-1368
   ms against 389-561 ms), while a `srcdoc` frame commits asynchronously. So:
   - The server never mounts, so it emits every board frame as a non-frame
     placeholder `div` carrying `data-srcdoc`, `data-doc` (and
     `data-doc-key` where the frame has one), the frame's class, the
     sandbox token set as a data attribute rendered from Go's `frameSandbox`
     (so mount copies it and the one-constant design holds), and any other
     element attribute the frame carries today, sized as the frame would be
     (a restored `style.height` applies to it as to the frame). A `div` needs
     no display rule; a `<template>` holder is rejected because it is
     `display: none`. `data-theme` is not on the list: `ThemeFrames` writes it
     inside the `data-srcdoc` document's `<html>`, not on the element.
   - Mount replaces the placeholder with an `<iframe>` built from those
     attributes, setting `sandbox` and `srcdoc` BEFORE the iframe is
     connected (a sandbox set after navigation applies only to the next
     navigation, and an iframe connected without `srcdoc` commits
     `about:blank` synchronously, the cost this revision removes), and keeps
     `data-srcdoc` on the frame for Decision 6's compare. The mount points
     above are unchanged.
   - Element identity: expandos (`wbGrow`, `wbNext`, `wbOld`, `wbOut`,
     `wbShowing`, `wbRelayAsked`), `frameGraft`'s `load` listener and the
     graft chain's `g.nc[0]` refer to the element, so a placeholder is
     replaced before any of them is set: an incoming placeholder is mounted
     before `frameGraftable` builds the chain, and keyed reuse runs before the
     swap's `mountFrames(n)` (`dashboard.js:522`), so a placeholder about to
     be replaced by a kept old frame is never mounted first.
   - Every reader that finds a board frame by element (all use
     `iframe.doc-frame` today, so an untouched reader silently skips
     placeholders) takes one of three modes:
     - Reads the placeholder's attributes: the palette body-text read
       (`dashboard.js:4796`) reads `data-srcdoc` (as ready ticket
       `261114-feat-ws-dashboard-unified-search-palette` plans); keyed reuse
       matches an incoming placeholder by `data-doc-key` plus `data-srcdoc`
       and moves the old frame in; `frameHeights` / `setFrameHeights`
       (`:3466`, `:3471`) read and write the placeholder's `style.height`.
     - Matches then mounts: `frameGraftable` (`dashboard.js:820`) matches
       the incoming placeholder, and `frameGraft` mounts it first only when
       the old frame is mounted, so a changed open topic keeps its
       double-buffer.
     - Skips (needs a live window): `docFrame` / `frameWins`, `postTheme`
       (`:404`), the initial size request (`:7154`), the `viewer.js` message
       lookup (`:262`), `wbGrow` marking (`:548`), and `mpend.frame`.
   - wiki-html (merged c49ae5a11): `SandboxedPage` builds its own eager
     `srcdoc` iframe outside the board mount pass; it is NOT a placeholder
     and stays an iframe. The interaction is `framedDoc`'s return line, where
     wiki-html introduced `sandbox="` + frameSandbox + `"`: the placeholder
     carries that same constant, as above.
   - Phase 2's unmount turns a mounted frame back into this placeholder,
     keeping the last `style.height`.
   - Verification adds: a JS asset test that a mounted frame carries the
     exact sandbox token set and `srcdoc` before it is connected; a Go test
     that the served board has no board `<iframe` (placeholders only);
     `regions_test.go:299-300` ("body frame missing or its sandbox changed")
     pins the placeholder's sandbox data attribute instead.
   - Measurement baseline: fixes 2-4 (e79e174de, 75d9e51a8, 27d8535a9) sit
     on top of the iframe form and are unmeasured. Measure in Phase 1 order
     on a stack where the revised lazy mount is fix 1 (each fix's A side is
     the stack below it); the Result records the method.
   - Rejected: (b) keeping the `<iframe>` element and accepting the long-task
     regression (total task time and layout fall, but the cold-load long
     tasks are what the owner feels as jank, and Decision 12 puts paint and
     CPU first); (c) reverting lazy mount (Phase 2 builds on it, Decision 8's
     exception).
3. **Batched frame heights** ((532)=a): height messages are queued in a Map
   and applied in one `requestAnimationFrame`; the "from" height is read from
   `style.height`, not `getBoundingClientRect` (`dashboard.js:3345`);
   `docFrame` (`dashboard.js:3242-3244`) resolves through a
   `Map(contentWindow -> iframe)` refreshed on insert. When `style.height` is
   unset (a frame's first report with no restored height), the "from" height
   is the CSS default, 0 (`iframe.doc-frame { height: 0 }`, `page.css:560`),
   which is what `getBoundingClientRect` returns for such a frame today, so
   the `wbGrow` first-report ease (`dashboard.js:3349-3353`) looks the same.
   Rejected: skipping the `wbGrow` ease when no height is set (it would drop
   the ease a new frame of a moving swap shows today, a visual change).
4. **Typing and idle paths** ((532)=a):
   - `pill.js` defers `input`-driven `update` to rAF / a short debounce (or
     re-classifies only the edited item).
   - `form.js` `build()`, when driven by `input` (`form.js:99`), coalesces to
     one rAF; its click and change callers (`form.js:40, 66, 72, 123, 138`)
     may stay synchronous. `build()` still reads every item and topic note
     for the reply text (`form.js:154-181`: the reply is the whole answer
     block); only the `staged` class toggles (`form.js:166`) scope to the
     edited item's topic. A pending build is flushed synchronously before
     anything reads the reply: the copy button (`form.js:201-203`) and the
     dashboard's `boardAnswers` (`dashboard.js:1324-1328`, reads
     `#reply-text`). form.js exposes the flush on `window`, as it exposes
     `wbBindBoard` (`form.js:270`), and any other reader of `#reply-text` or
     of form.js's `dirty` / `copied` state that the change finds calls it
     too.
   - `tickTimes` (`dashboard.js:91-97`) keeps cached arrays of its
     `time[data-ms][data-f=since|ago|rel]` and `em.u-pace[data-reset]` sets
     and re-collects both only when a dirty flag is set. One
     `MutationObserver` on the document (`childList` + `subtree`, plus
     `attributes` filtered to `data-ms`, `data-f`, `data-reset`) sets the
     flag, for insertions and removals alike. This covers every insertion
     path at once: board swaps, conversation-box inserts
     (`dashboard.js:1020`), history loads (`:3273`), `:4995`, `:6796`, cards
     and adopted pages, and nodes the script builds itself (the approval
     card's `time`, `:6684-6686`). Rejected: invalidating the cache at each
     `fmtTimes` call site (a path that builds a time without `fmtTimes`
     relies on today's per-second document query as its backstop, and a new
     call site could forget the invalidation); collecting once per board swap
     (misses every insertion outside the board).
5. **Per-topic hash** ((532)=a): the server emits a per-topic content hash
   attribute (`data-th`) on each topic section; `morphBoard` compares the
   incoming `data-th` with the previous incoming one (`boardSeen`,
   `dashboard.js:694-701`) instead of `outerHTML`, keeping d67107d5's
   compare-incoming-with-previous-incoming semantics. The hash covers
   everything `outerHTML` covered: the rendered section bytes with the
   `data-th` attribute itself excluded - the section's attributes and classes
   (`quiet`, `closed`, `has-doc`, `live`; `page.tmpl:42`), nested frames'
   `data-srcdoc` text, and the page language (it changes the rendered bytes).
   `ThemeFrames` runs per request after render, so `data-th` is theme-free; a
   theme switch reaches kept frames through the page script (`postTheme`,
   doc-frame.js), the path it already takes between swaps. T0 attribute.
   Rejected: hashing selected `TopicView` fields (any rendered byte left out
   of the field list - a class, a language string, a template change - would
   leave a stale topic on the page).
6. **Keyed frame reuse** ((532)=a): inside a changed topic, an incoming entry
   frame (`data-doc-key` `entry:eN`, immutable per entry) takes a matching old
   frame via `Element.moveBefore`, generalizing `frameGraftable`'s first-frame
   graft (`dashboard.js:790-800`). A match needs both an equal
   `data-doc-key` and equal `data-srcdoc` text (a plain string compare: the
   same key need not mean the same bytes, since `ImageSrc` inlining and relay
   lists change the frame document), and each old frame matches at most one
   incoming frame, so duplicate keys pair off once each. The move happens only
   after the new section is connected (`moveBefore` keeps state only for a
   move between connected nodes of one document). The topic-body frame
   (`topic:<name>`) keeps a stable key while its content changes, so it is not
   reused by key and keeps today's double-buffered `frameGraft`. Without
   `moveBefore`, today's behaviour stays.
7. **Hashed shared frame assets** ((532)=a): frame CSS and JS are served as
   public, immutable, content-hashed assets (`<link>` / `<script src>`)
   instead of inline, following the mermaid route precedent
   (`access.go:129-138`, `pages.go:1048, 1249-1254`). T0 additive route. The
   session-page CSP has no `style-src` / `script-src` today
   (`pages.go:1276`): if iframe-shell or browser-pane adds one when this
   lands, the hashed asset path is allowed in it. The theme pin stays
   working only if the frame document still opens with `frameDocStart`
   (`render.go:582-584`) as its first bytes after the inline `<style>` /
   `<script>` become `<link>` / `<script src>`: `ThemeFrames`
   (`render.go:594-600`) pins the theme by replacing
   `srcdoc="<!DOCTYPE html><html>` as a plain substring, which also matches
   inside `data-srcdoc="` (Decision 2), and silently stops pinning if
   anything is written before it. A test asserts that a themed board's
   `data-srcdoc` frames carry `data-theme`. A frame mounted after a later
   theme switch carries the theme pinned at render and takes the current one
   on its first height report (`postTheme`, `dashboard.js:3354-3356`).
8. **Measurement harness** ((532)=a; absorbs (473)): a synthetic board plus
   Playwright on Chromium with CDP `Emulation.setCPUThrottlingRate` at 4x and
   6x, recording `Performance.getMetrics` (Nodes, Frames, LayoutCount,
   LayoutDuration, RecalcStyleDuration, ScriptDuration), long tasks, and
   Event Timing for keystrokes, over scenarios (a) cold load, (b) 60 s idle,
   (c) 20 scripted one-line updates into one topic, (d) typing 50 characters
   into a topic note, (e) scrolling the board end to end (section 5). The
   harness runs before and after each Phase 1 fix (A/B). The harness is a
   repo-local developer command beside `scripts/dev.py`, not shipped (EXEC
   lead, technical: it needs Playwright and a dev daemon, which no plugin
   package carries). It runs on macOS now and on Windows (Chrome/Edge) when
   run there. A Windows run is outside Phase 1's boundary: Phase 1 is
   verified by the macOS run, and the (473) Windows numbers are a follow-up
   recorded whenever the harness is run there, not a Phase 1 verification
   item. Rejected: committing Playwright as a repo dependency (the cached
   Playwright under `~/.npm/_npx` is used, and the harness says how to get
   it).

   **A/B keep rule** (design review 2026-11-13, technical): each Phase 1 fix
   names its target scenarios and metrics before it is measured (lazy mount:
   Frames and LayoutDuration in (a) and (e); batched heights: LayoutCount and
   LayoutDuration in (a) and (c); typing paths: keystroke Event Timing and
   LayoutCount in (d), and ScriptDuration in (b) for `tickTimes`; per-topic
   hash and keyed reuse: ScriptDuration and Frames rebuilt in (c); hashed
   assets: ScriptDuration and Nodes in (a) and (c)). Each scenario runs at
   least 3 times per side and the medians are compared at 4x (6x is recorded,
   not judged). A fix is kept when a target metric improves by at least 10%
   and no recorded metric in any scenario regresses by more than 10%;
   otherwise it is reverted, and the Result records its numbers and the
   revert. Exception: lazy frame mount (Decision 2) is the mechanism Phase 2
   builds on, so if it misses the rule the worker stops and reports instead
   of reverting.

   **Noise amendment (Phase 1 stop, 2026-11-14, EXEC lead, technical):** the
   host runs parallel workers, and scenario (c)'s within-side spread reached
   10-15%. When a metric's regression, or a target metric's improvement, is
   within that metric's within-side spread (max minus min over a side's runs
   as a share of its median, taking the larger of the two sides), both sides
   are re-run with at least 5 runs each, alternating A and B run by run so
   host drift lands on both sides, and the rule is applied to the medians of
   those new runs only. The Result records the spread next to each judged
   metric. Rejected: raising the threshold (it would also
   hide real regressions on a quiet host).
9. **Viewport mount/unmount** ((532)=a, Phase 2): open frames mount within
   about 1-2 viewports and unmount beyond about 4-5, keeping the last
   `style.height` as the placeholder, through the visibility observer that
   `261113-feat-ws-dashboard-board-reorder-hold` Phase 1 builds in
   `wswhiteboard/assets/stream.js` and
   `261113-feat-ws-whiteboard-topic-history-load-more` reuses: one observer
   helper, no second copy (EXEC lead, technical, as for topic-history's
   decision 3). Must handle the `frameGraft` path, `boardJump`, mention
   selection state (`mpend.frame`), and a mermaid redraw on remount.
10. **`content-visibility: auto`** ((532)=a, Phase 2) on `section.topic` with
    `contain-intrinsic-size: auto 600px`; rect readers inside skipped topics
    (`pill.js:131`, `stream.js:32-43` `position()`, `captureBoardHistory`)
    switch to observer results or the topic's own rect so they do not force
    skipped subtrees to lay out.
11. **Incremental board events** ((532)=a, Phase 3, after iframe-shell
    Phase 2): the server sends only changed topics (keyed by `data-topic` +
    hash) plus order, as a new additive event type the page opts into (T0).
    Shape (design review 2026-11-13, technical):
    - Event: `board-topics`, beside today's `region` event (`regionEvent`,
      `sse.go:213-217`; `sendRegions`, `sse.go:583-600`), on both the SSE and
      WebSocket sinks. Payload: the new board region hash, the ordered topic
      keys (`data-topic`), and for each topic whose `data-th` differs from
      what this connection last sent, its `data-th` and section HTML.
    - Opt-in: the page asks for it with a stream query parameter, `topics=1`,
      beside `regions` and `h` (`sse.go:249, 425`); a page without it gets
      today's full `region` event unchanged.
    - Fallback is the full board region swap: on a connection's first board
      send and after a reconnect (the server does not know which topics the
      page holds), when any non-topic part of the board region changed (reply
      bar, closed-section controls), and when the client cannot apply an
      event (an order key it neither holds nor received) - it then reconnects
      without its held board hash in `h`, which yields a full region.
    - The per-topic HTML and hashes come from the per-session render that
      iframe-shell Phase 2's fan-out-by-hash cache shares across connections;
      only the diff against each connection's held topic hashes is per
      connection.
    - The client feeds the result through the same `morphBoard` path, so
      pins, keyed frame reuse and FLIP behave as for a full swap.
12. **Priorities** (owner note 2026-11-13): paint, layout and CPU over
    memory; a fix that saves only memory is not pursued here.
13. **Compatibility tier: T0**: asset and template changes, data attributes,
    one additive immutable asset route, and (Phase 3) one additive opt-in
    event type.

## Constraints

- No intended visual change; LOOK before/after captures of a long board on
  the dashboard confirm it.
- Out of scope: server-side windowing of older history pages and deferred
  closed topics (section 3 row 3b) - topic-history's worker decides template-
  vs fetch-based older pages; the server per-entry render cache (2d), the
  static page `live.js` hidden-tab reload (6), and the `plan` meta write
  (7) - each can become its own ticket from the measurements.
- Overlap: ready `261113-feat-ws-dashboard-iframe-shell` (in flight) nests
  the board one frame deeper and composes the `?frame=1` CSP; whichever lands
  second rebases. `IntersectionObserver` with the implicit root works inside
  the nested frame.
- Daemon code and assets under `agents-plugin-tool/internal/` follow
  `ai-docs/manuals/release-compatibility.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 515af13f (2026-11-13, commit): "Measure-first decision; speculative rewrite or framework migration rejected." — bearing: supports
- b990bc04 (2026-11-13, commit): "Dashboard is server-rendered HTML plus vanilla JS with no build step, not React; techniques are CSS/DOM level." — bearing: supports
- e95725c2 (2026-11-13, commit): "Technical: harness is a repo-local developer command beside scripts/dev.py, not shipped and not adding Playwright as a dependency." — bearing: supports
- 0cd850f5 (2026-11-12, commit): "Ruled out on the way: deferring the folded topics' iframe srcdoc loads (bars still went missing), and sticky-bar compositing" — bearing: constrains
- d67107d5 (2026-11-10, commit): "Board morph compares incoming HTML with the previous incoming HTML, never the live DOM, which the page mutates after a swap" — bearing: constrains
- ddd374ee (2026-11-10, commit): "Heights are read before morphBoard (topicHeights) ... setFrameHeights restores the body frame heights" — bearing: constrains
- 445e0769 (2026-11-12, commit): "frame-src 'self' verified headless: ... topic-body srcdoc frames still load and run." — bearing: constrains
- 93346ad1 (2026-11-10, commit): "Lead profiling found the event path (whole-page re-render per live-snapshot nudge, base64-inlined board images) dominates; polls are under 5%." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Remaining Phase 3 verification spans agents-plugin-tool/internal/wsdashboard/board_topics_stream_test.go, board_topics_client_test.go, wswhiteboard/assets/stream.js and scripts/board-perf.mjs; this accommodation edits only this ticket |
| scope.surface | cross-module | Existing dashboard/whiteboard stream and frame behavior; board-topics wire shape at agents-plugin-tool/internal/wsdashboard/board_topics.go#L20-L32 |
| scope.new_public_symbol | no | Resumption adds none; topics=1 and board-topics already exist (agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L6264-L6273) |
| scope.new_type_contract | no | Resumption adds none; existing boardTopicsEvent is a wire contract, not an absence of one (agents-plugin-tool/internal/wsdashboard/board_topics.go#L26-L32) |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/board_topics_stream_test.go#L26-L133 and board_topics_client_test.go#L38-L98 cover existing Phase 3 contracts; phase3/README.md records the failed LOOK comparison |
| complexity.reuse_points | confirmed | Existing per-connection diff/fallback (agents-plugin-tool/internal/wsdashboard/board_topics.go#L102-L119), shared decomposition test (board_topics_stream_test.go#L109-L126), and retained phase3/README.md reproduction procedure; independently reviewed history-fit producer 8f24cec5b9ee9e09d7c2d58e5002ddab03842152 landed in develop via 28d8576226cc62312bf2d85d9c4cb306882f3df0 |
| complexity.side_effect_risk | high | Sequential frame delivery changes history clipping in the unchanged source (agents-plugin-tool/internal/wswhiteboard/assets/stream.js#L53-L73); no integration/runtime action is authorized by this accommodation |
| risk.correctness | high | Original 219px mismatch remains unresolved; current snap measures the current clip but adds to the original base (agents-plugin-tool/internal/wswhiteboard/assets/stream.js#L55-L67) |
| risk.fit | moderate | iframe-shell Phase 2 is landed (3310356af is an ancestor of HEAD); root verified history-fit integration 28d8576226cc62312bf2d85d9c4cb306882f3df0, but this retained worktree still has the old source and the common-history A/B stack must be prepared and verified |
| risk.test | high | phase3/README.md records an Important LOOK gap; matched boundary IDs, settled geometry and the sequential-height replay still need independent verification on the same landed history revision |
| risk.security_or_contract | moderate | Existing additive T0 event/fallback contract must remain unchanged (agents-plugin-tool/internal/wsdashboard/board_topics.go#L26-L32, board_topics_stream_test.go#L87-L133); this resumption approves no new API or history behavior |

## Phases

### Phase 1: Measurement harness and S fixes

- The harness (Decision 8), with a baseline run recorded before any fix.
- Lazy frame mount (Decision 2), batched heights (Decision 3), typing and
  idle paths (Decision 4), per-topic hash (Decision 5), keyed frame reuse
  (Decision 6), hashed shared frame assets (Decision 7), in that order, each
  measured A/B with the harness and kept or reverted under Decision 8's A/B
  keep rule.

Verification: the Result records the harness numbers per scenario at 4x and
6x before and after each fix (Frames, LayoutCount, LayoutDuration,
ScriptDuration, long tasks, keystroke Event Timing) and each fix's kept or
reverted outcome under the A/B keep rule; asset tests that frames under a
closed `<details>` carry `data-srcdoc` and mount on open (including a
programmatic open through `restoreOpen` and `boardJump`), on initial page
load (including a pinned topic moved into the pin list), on insert and after
`wbhistoryload`; a test that N height messages cause one host layout batch,
and that a frame with no `style.height` eases from 0 as today; a test that an
unchanged topic (same `data-th`) is not replaced, and that `data-th` changes
when only a section class (`closed`) or only a frame's `data-srcdoc`
changes; a test that inside a changed topic an entry frame (`entry:eN`) with
an unchanged key and unchanged `data-srcdoc` is kept as the same element, an
entry frame with the same key but changed `data-srcdoc` is replaced, and the
topic-body frame (`topic:<name>`) still takes the `frameGraft` path; a test
that `tickTimes` updates a relative time inserted outside a board swap on the
next tick; a test that the copy button and `boardAnswers` see a keystroke
typed just before them (the pending `build()` is flushed); a Go test that a
themed board (`ThemeFrames`) carries `data-theme` on every `data-srcdoc`
frame; a Go test for the hashed asset route (immutable cache headers, hash in
the path, served without a session); LOOK captures of a long board before and
after with no visual difference; the dashboard and whiteboard Go and JS test
suites pass. A Windows harness run is not part of Phase 1 verification
(Decision 8).

### Result (e04c7ea1b) - 2026-11-14

Fixes 1-5 kept, fix 1 in a paced form. Fix 6 reverted under the A/B keep
rule; see its block, and the report proposes the owner re-judge it.

Commits:

- harness: 90bc7c5b1, adbb8639a (measures inside the iframe shell's
  session frame), 4b9bac4f2 (several trees alternate run by run; spread),
  0cd759136 (`--gc`), c48a0fa69 (LOOK diff box)
- fix 1 lazy mount: bcb2f06c3, 589e65e1f (placeholder per the Decision 2
  Revision), e55ae0796 (paced first pass)
- fix 2 heights: e79e174de
- fix 3 typing and idle: 75d9e51a8
- fix 4 per-topic hash: 27d8535a9
- fix 5 keyed frame reuse: 277615827
- fix 6 hashed frame assets: 143eedccb, reverted by e04c7ea1b

Method.

- **Harness.** `scripts/board-perf.mjs run` builds the `internal/mcp` test
  binary from each tree and starts a fresh in-process daemon per run
  (TestBoardPerfServe). It never touches the shared daemon.
  - It runs headless Chromium 148 with CDP CPU throttling at 4x and 6x, on
    the synthetic board shape 8,6,4,2,60.
  - It measures inside the iframe shell's session frame (`?frame=1`).
  - Performance.getMetrics covers the whole renderer, shell included.
- **Stack.** Each fix was judged on a stack built on the merged base
  (develop 3ee1d448b plus the harness). Revert commits peel the fixes off
  one at a time:
  - S4 = 4b9bac4f2, fixes 1-4
  - S3 = without fix 4
  - S2 = without fixes 3-4
  - S1 = fix 1 only
  - S0 = no fix

  Fix 5 and fix 6 were each judged against the commit below them
  (S4 -> 277615827 -> 143eedccb).
- **Runs.** Every A/B pair alternates A and B run by run, so host drift
  lands on both sides. The host ran parallel workers (load 6-30).
- **Keep rule.**
  - Medians at 4x; 6x is recorded only.
  - A regression counts past 10% and past a one-unit absolute floor.
  - Each cell below is `before -> after (spread)`. Spread is max minus min
    over a side's runs as a share of its median, the larger side.
- **Noise amendment.** Where a judged change fell within its spread, the
  metric's scenario was re-run with at least 5 alternating runs per side,
  and only the new medians were judged.
- **`--gc` and scenario (c).** Scenario (c) Nodes and Frames turned out
  bimodal per run, depending on whether a garbage collection ran before the
  closing read (19.7k / 35.6k / 51.4k nodes). So:
  - every re-run, and the fix 5 and fix 6 runs, read those counts after a
    forced collection (`--gc`, durations read before it);
  - the fix 1-4 first runs did not.
- **Scenario (a).** Every b-e run starts from a cold load, so (a) gets 4x
  the run count in samples, except in a-only runs.

**Fix 1, lazy mount.** S0 -> S1, 3 runs per side.

- Targets: a@4 Frames -67.3% and LayoutDuration -22.7%, e@4 Frames -67.3%.
- Regression: a@4 longTaskMs +17.7%, within its 21% spread.
- The noise re-run (a only, 6 alternating runs) still showed longTaskMs
  +17.1% at a 17% spread, with TaskDuration -39%: FAIL. Mounting every open
  frame in one synchronous pass lengthened the load's long task.
- The paced first pass (`mountFramesPaced`, 8 ms slices) was then judged
  against S0 (a only, 6 alternating runs): KEEP.
  - Targets: a@4 Frames -67.3%, LayoutDuration -17.8% (13% spread).
  - longTaskMs -12.3%; no regression.
- The swap, adopted-page and history-load mounts stay synchronous.

First run, unpaced:

| run | Frames | LayoutCount | LayoutDur ms | ScriptDur ms | long tasks | long ms | key med ms | key p95 ms | key max ms | built | Nodes |
|---|---|---|---|---|---|---|---|---|---|---|---|
| a@4 | 104 → 34.0 (0%) | 379 → 190 (3%) | 238 → 184 (18%) | 475 → 420 (15%) | 5.0 → 4.5 (22%) | 479 → 563 (21%) | - | - | - | 102 → 32.0 (0%) | 25803 → 20156 (1%) |
| a@6 | 104 → 34.0 (0%) | 381 → 192 (1%) | 363 → 295 (20%) | 751 → 650 (19%) | 8.0 → 7.5 (27%) | 852 → 996 (19%) | - | - | - | 102 → 32.0 (0%) | 25720 → 20156 (0%) |
| b@4 | 104 → 34.0 (0%) | 1.0 → 1.0 (0%) | 1.5 → 1.9 (27%) | 67.4 → 56.9 (6%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 25656 → 19453 (0%) |
| b@6 | 104 → 34.0 (0%) | 1.0 → 1.0 (0%) | 2.5 → 1.9 (89%) | 96.2 → 83.4 (11%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 25656 → 19453 (0%) |
| c@4 | 113 → 38.0 (11%) | 806 → 406 (0%) | 529 → 340 (10%) | 11891 → 11587 (12%) | 20.0 → 20.0 (5%) | 20761 → 20979 (8%) | - | - | - | 180 → 80.0 (0%) | 296449 → 35426 (45%) |
| c@6 | 122 → 38.0 (65%) | 810 → 406 (0%) | 815 → 508 (5%) | 18171 → 17359 (4%) | 20.0 → 20.0 (0%) | 32365 → 31883 (4%) | - | - | - | 180 → 80.0 (0%) | 297150 → 35426 (168%) |
| d@4 | 104 → 34.0 (0%) | 321 → 255 (0%) | 167 → 82.2 (6%) | 338 → 271 (7%) | 1.0 → 0.0 (0%) | 138 → 0.0 (12%) | 19.0 → 14.2 (6%) | 22.7 → 16.4 (6%) | 32.0 → 24.0 (0%) | 0.0 → 0.0 (0%) | 27245 → 21583 (1%) |
| d@6 | 104 → 34.0 (0%) | 321 → 255 (0%) | 272 → 147 (18%) | 517 → 446 (16%) | 1.0 → 0.0 (100%) | 209 → 0.0 (50%) | 29.8 → 23.0 (13%) | 34.4 → 27.0 (15%) | 40.0 → 32.0 (20%) | 0.0 → 0.0 (0%) | 27162 → 21583 (0%) |
| e@4 | 104 → 34.0 (0%) | 154 → 154 (0%) | 49.3 → 48.7 (36%) | 256 → 204 (13%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 26623 → 20986 (0%) |
| e@6 | 104 → 34.0 (0%) | 151 → 153 (2%) | 90.9 → 83.7 (23%) | 475 → 329 (16%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 26503 → 20986 (0%) |

Re-run, unpaced:

| run | Frames | LayoutCount | LayoutDur ms | ScriptDur ms | long tasks | long ms | built | Nodes |
|---|---|---|---|---|---|---|---|---|
| a@4 | 104 → 34.0 (0%) | 392 → 190 (4%) | 208 → 157 (18%) | 410 → 378 (13%) | 3.5 → 3.0 (33%) | 392 → 459 (17%) | 102 → 32.0 (0%) | 25634 → 19425 (0%) |
| a@6 | 104 → 34.0 (0%) | 383 → 192 (1%) | 319 → 262 (15%) | 616 → 568 (9%) | 6.0 → 5.0 (50%) | 704 → 745 (33%) | 102 → 32.0 (0%) | 25628 → 19425 (0%) |

Paced (kept form):

| run | Frames | LayoutCount | LayoutDur ms | ScriptDur ms | long tasks | long ms | built | Nodes |
|---|---|---|---|---|---|---|---|---|
| a@4 | 104 → 34.0 (0%) | 379 → 196 (4%) | 212 → 175 (13%) | 408 → 372 (10%) | 4.0 → 3.5 (50%) | 414 → 363 (31%) | 102 → 32.0 (0%) | 25628 → 19425 (0%) |
| a@6 | 104 → 34.0 (0%) | 381 → 201 (1%) | 322 → 288 (15%) | 621 → 614 (13%) | 6.5 → 4.0 (46%) | 733 → 607 (28%) | 102 → 32.0 (0%) | 25628 → 19425 (0%) |

**Fix 2, batched heights.** S1 -> S2, 5 alternating runs per side: KEEP.

- Target: a@4 LayoutCount -16.3% (1% spread).
- The c@4 Nodes (+79.9%) and Frames (+11.4%) rises were within their
  spreads. The c-only re-run with `--gc` (5 alternating) showed Nodes
  +0.4% and Frames +2.9% (34 -> 35), so there was no regression.
- mountedFrames and framesBuilt are identical on both sides throughout.

| run | Frames | LayoutCount | LayoutDur ms | ScriptDur ms | long tasks | long ms | key med ms | key p95 ms | key max ms | built | Nodes |
|---|---|---|---|---|---|---|---|---|---|---|---|
| a@4 | 34.0 → 34.0 (0%) | 190 → 159 (1%) | 172 → 171 (25%) | 397 → 385 (19%) | 4.0 → 4.0 (50%) | 513 → 513 (32%) | - | - | - | 32.0 → 32.0 (0%) | 20156 → 20156 (0%) |
| a@6 | 34.0 → 34.0 (0%) | 192 → 161 (1%) | 280 → 267 (25%) | 608 → 599 (18%) | 7.0 → 7.0 (57%) | 944 → 940 (36%) | - | - | - | 32.0 → 32.0 (0%) | 20156 → 20156 (0%) |
| b@4 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 1.2 → 1.6 (105%) | 55.4 → 54.7 (10%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19453 → 19453 (0%) |
| b@6 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 2.6 → 2.4 (54%) | 77.1 → 78.9 (20%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19453 → 19453 (0%) |
| c@4 | 35.0 → 39.0 (21%) | 406 → 406 (0%) | 320 → 325 (13%) | 11177 → 11347 (8%) | 20.0 → 20.0 (0%) | 20504 → 20549 (5%) | - | - | - | 80.0 → 80.0 (0%) | 19742 → 35512 (89%) |
| c@6 | 38.0 → 39.0 (11%) | 406 → 406 (0%) | 492 → 518 (11%) | 17023 → 17341 (7%) | 20.0 → 20.0 (0%) | 31484 → 31809 (5%) | - | - | - | 80.0 → 80.0 (0%) | 35427 → 35511 (82%) |
| d@4 | 34.0 → 34.0 (0%) | 255 → 255 (0%) | 83.1 → 75.3 (30%) | 250 → 260 (20%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 12.4 → 12.8 (19%) | 16.1 → 15.5 (13%) | 24.0 → 16.0 (50%) | 0.0 → 0.0 (0%) | 21583 → 21583 (0%) |
| d@6 | 34.0 → 34.0 (0%) | 255 → 255 (0%) | 119 → 110 (38%) | 377 → 376 (17%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 19.0 → 18.3 (19%) | 23.5 → 25.2 (31%) | 24.0 → 24.0 (33%) | 0.0 → 0.0 (0%) | 21583 → 21584 (0%) |
| e@4 | 34.0 → 34.0 (0%) | 154 → 154 (0%) | 47.4 → 44.7 (30%) | 177 → 182 (20%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 20986 → 20986 (0%) |
| e@6 | 34.0 → 34.0 (0%) | 154 → 154 (2%) | 72.0 → 69.8 (42%) | 301 → 319 (28%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 20986 → 20986 (0%) |

c re-run with `--gc`:

| run | Frames | LayoutCount | LayoutDur ms | ScriptDur ms | long tasks | long ms | built | Nodes |
|---|---|---|---|---|---|---|---|---|
| c@4 | 34.0 → 35.0 (0%) | 406 → 406 (0%) | 296 → 302 (2%) | 10859 → 10974 (3%) | 20.0 → 20.0 (0%) | 20182 → 20563 (3%) | 80.0 → 80.0 (0%) | 19638 → 19723 (0%) |
| c@6 | 34.0 → 35.0 (0%) | 406 → 406 (0%) | 458 → 461 (9%) | 15885 → 16021 (4%) | 20.0 → 20.0 (0%) | 29587 → 29822 (3%) | 80.0 → 80.0 (0%) | 19638 → 19723 (0%) |

**Fix 3, typing and idle paths.** S2 -> S3, 3 runs per side: KEEP.

- Target: d@4 LayoutCount -19.6% (0% spread).
- b@4 ScriptDuration -94.3% was within its 147% spread, and c@4 Nodes
  +37.7% was within spread too. The re-run (b and c, 5 alternating, `--gc`)
  showed:
  - b@4 ScriptDuration -93.6% (43% spread);
  - c with no change past 1%.
- d@4 keystroke Event Timing is unchanged at about 11.5 ms median and
  16 ms max. The typing win is the layouts.

| run | Frames | LayoutCount | LayoutDur ms | ScriptDur ms | long tasks | long ms | key med ms | key p95 ms | key max ms | built | Nodes |
|---|---|---|---|---|---|---|---|---|---|---|---|
| a@4 | 34.0 → 34.0 (0%) | 159 → 159 (1%) | 158 → 163 (32%) | 371 → 368 (19%) | 3.5 → 3.5 (86%) | 486 → 474 (45%) | - | - | - | 32.0 → 32.0 (0%) | 20156 → 20156 (0%) |
| a@6 | 34.0 → 34.0 (0%) | 160 → 161 (1%) | 254 → 247 (17%) | 564 → 557 (17%) | 6.0 → 6.0 (67%) | 841 → 860 (35%) | - | - | - | 32.0 → 32.0 (0%) | 20156 → 20156 (0%) |
| b@4 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 1.7 → 1.6 (34%) | 50.7 → 2.9 (147%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19453 → 19453 (0%) |
| b@6 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 2.2 → 2.4 (58%) | 71.7 → 4.1 (47%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19453 → 19453 (0%) |
| c@4 | 39.0 → 39.0 (31%) | 406 → 406 (0%) | 304 → 289 (3%) | 10762 → 10939 (2%) | 20.0 → 20.0 (0%) | 20033 → 20057 (2%) | - | - | - | 80.0 → 80.0 (0%) | 35617 → 49043 (129%) |
| c@6 | 39.0 → 39.0 (21%) | 406 → 406 (0%) | 457 → 451 (5%) | 16086 → 16090 (2%) | 20.0 → 20.0 (0%) | 29965 → 29880 (2%) | - | - | - | 80.0 → 80.0 (0%) | 35617 → 35511 (89%) |
| d@4 | 34.0 → 34.0 (0%) | 255 → 205 (0%) | 70.9 → 64.4 (8%) | 224 → 240 (6%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 11.5 → 11.7 (7%) | 14.6 → 15.1 (15%) | 16.0 → 16.0 (50%) | 0.0 → 0.0 (0%) | 21583 → 21583 (0%) |
| d@6 | 34.0 → 34.0 (0%) | 255 → 205 (0%) | 113 → 104 (37%) | 420 → 364 (21%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 19.8 → 17.3 (28%) | 24.8 → 25.6 (8%) | 32.0 → 32.0 (0%) | 0.0 → 0.0 (0%) | 21583 → 21583 (0%) |
| e@4 | 34.0 → 34.0 (0%) | 154 → 154 (0%) | 33.5 → 34.7 (51%) | 180 → 146 (19%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 20986 → 20986 (0%) |
| e@6 | 34.0 → 34.0 (0%) | 153 → 154 (1%) | 81.2 → 66.7 (29%) | 310 → 249 (32%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 20986 → 20986 (0%) |

b/c re-run with `--gc`:

| run | Frames | LayoutCount | LayoutDur ms | ScriptDur ms | long tasks | long ms | built | Nodes |
|---|---|---|---|---|---|---|---|---|
| b@4 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 1.5 → 1.3 (67%) | 51.7 → 3.3 (43%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 19425 → 19425 (0%) |
| b@6 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 2.2 → 2.7 (50%) | 76.9 → 4.2 (42%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 19425 → 19425 (0%) |
| c@4 | 35.0 → 35.0 (0%) | 406 → 406 (0%) | 300 → 300 (6%) | 10802 → 10759 (6%) | 20.0 → 20.0 (0%) | 20224 → 20071 (4%) | 80.0 → 80.0 (0%) | 19723 → 19723 (0%) |
| c@6 | 35.0 → 35.0 (0%) | 406 → 406 (0%) | 474 → 449 (10%) | 16016 → 16049 (5%) | 20.0 → 20.0 (0%) | 29646 → 29727 (4%) | 80.0 → 80.0 (0%) | 19723 → 19723 (0%) |

**Fix 4, per-topic hash.** S3 -> S4, 3 runs per side: KEEP.

- Target: c@4 ScriptDuration -10.4% (6% spread).
- No regression at 4x. The c@6 Nodes and Frames jump is the same pre-`--gc`
  collection artifact; 6x is recorded only.

| run | Frames | LayoutCount | LayoutDur ms | ScriptDur ms | long tasks | long ms | key med ms | key p95 ms | key max ms | built | Nodes |
|---|---|---|---|---|---|---|---|---|---|---|---|
| a@4 | 34.0 → 34.0 (0%) | 159 → 159 (1%) | 163 → 164 (16%) | 376 → 294 (14%) | 4.0 → 3.0 (67%) | 512 → 388 (37%) | - | - | - | 32.0 → 32.0 (0%) | 20156 → 20156 (0%) |
| a@6 | 34.0 → 34.0 (0%) | 161 → 160 (1%) | 253 → 262 (22%) | 569 → 471 (19%) | 6.0 → 6.0 (50%) | 856 → 782 (31%) | - | - | - | 32.0 → 32.0 (0%) | 20156 → 20156 (0%) |
| b@4 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 1.8 → 1.6 (30%) | 3.2 → 3.7 (7%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19453 → 19453 (0%) |
| b@6 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 1.9 → 1.9 (60%) | 4.6 → 4.1 (50%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19453 → 19453 (0%) |
| c@4 | 39.0 → 39.0 (10%) | 406 → 406 (0%) | 297 → 295 (8%) | 10639 → 9532 (6%) | 20.0 → 20.0 (0%) | 19839 → 19017 (5%) | - | - | - | 80.0 → 80.0 (0%) | 35511 → 35512 (45%) |
| c@6 | 35.0 → 43.0 (11%) | 406 → 406 (0%) | 477 → 478 (11%) | 16622 → 14204 (5%) | 20.0 → 20.0 (0%) | 30645 → 28283 (4%) | - | - | - | 80.0 → 80.0 (0%) | 19742 → 51371 (83%) |
| d@4 | 34.0 → 34.0 (0%) | 205 → 205 (0%) | 61.8 → 64.3 (39%) | 241 → 233 (13%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 11.6 → 11.5 (14%) | 15.3 → 15.2 (12%) | 16.0 → 16.0 (50%) | 0.0 → 0.0 (0%) | 21583 → 21583 (0%) |
| d@6 | 34.0 → 34.0 (0%) | 205 → 205 (0%) | 116 → 127 (27%) | 409 → 444 (21%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 21.2 → 22.5 (21%) | 26.3 → 26.7 (7%) | 32.0 → 32.0 (0%) | 0.0 → 0.0 (0%) | 21583 → 21583 (0%) |
| e@4 | 34.0 → 34.0 (0%) | 154 → 154 (0%) | 39.7 → 36.8 (23%) | 157 → 161 (40%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 20987 → 20986 (0%) |
| e@6 | 34.0 → 34.0 (0%) | 153 → 154 (1%) | 73.4 → 62.6 (27%) | 296 → 249 (18%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 20986 → 20986 (0%) |

**Fix 5, keyed frame reuse.** S4 -> 277615827, 3 runs per side, `--gc`:
KEEP.

- Target: c@4 framesBuilt -75.0% (80 -> 20: only the topic-body frames
  rebuild).
- Also: c@4 LayoutCount -24.6% and LayoutDuration -32.9%.
- No regression.

| run | Frames | LayoutCount | LayoutDur ms | ScriptDur ms | long tasks | long ms | key med ms | key p95 ms | key max ms | built | Nodes |
|---|---|---|---|---|---|---|---|---|---|---|---|
| a@4 | 34.0 → 34.0 (0%) | 159 → 159 (1%) | 158 → 162 (25%) | 292 → 288 (23%) | 4.0 → 3.0 (67%) | 416 → 384 (37%) | - | - | - | 32.0 → 32.0 (0%) | 19425 → 19425 (0%) |
| a@6 | 34.0 → 34.0 (0%) | 160 → 160 (1%) | 255 → 254 (22%) | 447 → 443 (14%) | 6.0 → 6.0 (33%) | 726 → 718 (22%) | - | - | - | 32.0 → 32.0 (0%) | 19425 → 19425 (0%) |
| b@4 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 1.6 → 1.7 (52%) | 2.8 → 3.1 (61%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19425 → 19425 (0%) |
| b@6 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 2.0 → 2.6 (43%) | 4.0 → 3.3 (46%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19425 → 19425 (0%) |
| c@4 | 35.0 → 35.0 (0%) | 406 → 306 (0%) | 303 → 203 (8%) | 9514 → 8887 (9%) | 20.0 → 20.0 (0%) | 18738 → 18156 (4%) | - | - | - | 80.0 → 20.0 (0%) | 19724 → 19724 (0%) |
| c@6 | 35.0 → 35.0 (0%) | 406 → 306 (0%) | 474 → 325 (10%) | 14243 → 13449 (4%) | 20.0 → 20.0 (0%) | 28560 → 27808 (4%) | - | - | - | 80.0 → 20.0 (0%) | 19724 → 19724 (0%) |
| d@4 | 34.0 → 34.0 (0%) | 205 → 205 (0%) | 58.3 → 56.3 (39%) | 235 → 242 (7%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 11.8 → 11.9 (6%) | 15.0 → 14.8 (5%) | 24.0 → 16.0 (50%) | 0.0 → 0.0 (0%) | 19443 → 19443 (0%) |
| d@6 | 34.0 → 34.0 (0%) | 205 → 205 (0%) | 106 → 96.5 (25%) | 366 → 374 (7%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 17.6 → 17.6 (3%) | 23.8 → 25.0 (9%) | 24.0 → 32.0 (33%) | 0.0 → 0.0 (0%) | 19443 → 19443 (0%) |
| e@4 | 34.0 → 34.0 (0%) | 154 → 154 (0%) | 43.8 → 36.2 (24%) | 168 → 167 (33%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19427 → 19427 (0%) |
| e@6 | 34.0 → 34.0 (0%) | 154 → 154 (1%) | 57.9 → 68.0 (47%) | 270 → 318 (20%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19427 → 19427 (0%) |

**Fix 6, hashed shared frame assets.** 277615827 -> 143eedccb, 3 runs per
side, `--gc`: FAIL, reverted.

- Target: c@4 ScriptDuration -77.7% (8987 -> 2007 ms over 20 updates, 5%
  spread), and longTaskMs -84%.
- e@4 LayoutDuration, RecalcStyleDuration, ScriptDuration and TaskDuration
  rose 11-16%, each within its spread.
- The e re-run (5 alternating) still showed LayoutDuration +26.3%
  (38.2 -> 48.2 ms, 48% spread) and TaskDuration +12.5% (32% spread). At
  6x the same metrics fell about 9%.
- The rule applies to the re-run medians, so the fix was reverted. No
  mechanism links stylesheet and script loading to scroll-time layout after
  load.

| run | Frames | LayoutCount | LayoutDur ms | ScriptDur ms | long tasks | long ms | key med ms | key p95 ms | key max ms | built | Nodes |
|---|---|---|---|---|---|---|---|---|---|---|---|
| a@4 | 34.0 → 34.0 (0%) | 159 → 156 (2%) | 157 → 155 (12%) | 281 → 294 (12%) | 3.5 → 4.0 (29%) | 402 → 413 (25%) | - | - | - | 32.0 → 32.0 (0%) | 19425 → 19425 (0%) |
| a@6 | 34.0 → 34.0 (0%) | 160 → 158 (1%) | 255 → 248 (19%) | 441 → 439 (16%) | 6.0 → 6.0 (33%) | 740 → 705 (25%) | - | - | - | 32.0 → 32.0 (0%) | 19425 → 19425 (0%) |
| b@4 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 1.5 → 1.6 (24%) | 4.4 → 3.6 (54%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19425 → 19425 (0%) |
| b@6 | 34.0 → 34.0 (0%) | 1.0 → 1.0 (0%) | 1.7 → 2.4 (128%) | 4.2 → 5.0 (84%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19425 → 19425 (0%) |
| c@4 | 35.0 → 35.0 (0%) | 306 → 306 (0%) | 203 → 216 (9%) | 8987 → 2007 (5%) | 20.0 → 20.0 (0%) | 18227 → 2914 (4%) | - | - | - | 20.0 → 20.0 (0%) | 19724 → 19724 (0%) |
| c@6 | 35.0 → 35.0 (0%) | 306 → 306 (0%) | 327 → 322 (4%) | 13395 → 3037 (2%) | 20.0 → 20.0 (0%) | 27222 → 4454 (2%) | - | - | - | 20.0 → 20.0 (0%) | 19724 → 19724 (0%) |
| d@4 | 34.0 → 34.0 (0%) | 205 → 205 (0%) | 60.0 → 59.9 (19%) | 238 → 228 (8%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 11.5 → 11.0 (4%) | 15.3 → 14.9 (12%) | 24.0 → 24.0 (33%) | 0.0 → 0.0 (0%) | 19443 → 19443 (0%) |
| d@6 | 34.0 → 34.0 (0%) | 205 → 205 (0%) | 91.4 → 87.8 (27%) | 362 → 363 (10%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 17.5 → 17.6 (4%) | 25.0 → 24.1 (8%) | 32.0 → 32.0 (25%) | 0.0 → 0.0 (0%) | 19443 → 19443 (0%) |
| e@4 | 34.0 → 34.0 (0%) | 154 → 154 (0%) | 38.5 → 43.0 (16%) | 165 → 184 (20%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19427 → 19427 (0%) |
| e@6 | 34.0 → 34.0 (0%) | 153 → 154 (1%) | 81.3 → 64.8 (40%) | 282 → 290 (12%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | - | - | - | 0.0 → 0.0 (0%) | 19427 → 19427 (0%) |

e re-run:

| run | Frames | LayoutCount | LayoutDur ms | ScriptDur ms | long tasks | long ms | built | Nodes |
|---|---|---|---|---|---|---|---|---|
| e@4 | 34.0 → 34.0 (0%) | 154 → 154 (0%) | 38.2 → 48.2 (48%) | 173 → 175 (35%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 19427 → 19427 (0%) |
| e@6 | 34.0 → 34.0 (0%) | 153 → 153 (2%) | 73.8 → 66.9 (32%) | 283 → 283 (24%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 0.0 → 0.0 (0%) | 19427 → 19427 (0%) |

**LOOK.**

- `board-perf.mjs shot` captured the long board in light and dark at 12
  scroll stops on S0 and on e04c7ea1b.
- `diff` found 0.016-0.118% of pixels differing per capture. Every box
  falls on the clock-time labels (the topic bars' `HH:MM` and the
  "System events · HH:MM" rows): the captures were taken at different
  minutes.
- On inspection there is no layout or style difference.

**Tests.**

- Phase 1's asset and Go tests were added or retargeted:
  - mount on load, toggle, restoreOpen, boardJump, insert and history
    load, plus the paced pass;
  - sandbox and srcdoc set before connect;
  - no board `<iframe` in the served markup;
  - the heights batch;
  - `data-th`;
  - keyed reuse;
  - the tick;
  - typing flushes;
  - themed placeholders carry `data-theme`.
- `go test ./internal/wswhiteboard ./internal/wsdashboard` passes. So does
  the `internal/mcp` dashboard, whiteboard, board, panel, theme, closed,
  topic and wiki subset.
- The hashed-asset route test left with the fix 6 revert.

### Phase 2: Viewport mount/unmount and content-visibility

Requires `261113-feat-ws-dashboard-board-reorder-hold` Phase 1 (the shared
visibility observer). Before starting, the worker verifies that phase has
landed (`git log --grep=261113-feat-ws-dashboard-board-reorder-hold` shows a
commit recording its Phase 1 result) and stops and reports otherwise. The
prerequisite is prose, deliberately not a typed blocked-by: a typed block
would gate Phase 1 too.

- Viewport mount/unmount of open frames (Decision 9).
- `content-visibility` on topics with the rect-reader audit (Decision 10).

Verification: harness numbers for scenarios (a), (c) and (e) before and
after; a headless check that scrolling a long board mounts frames ahead of
the viewport, unmounts far ones with their height kept (no scroll jump), and
remounts with mermaid redrawn; `boardJump` to an unmounted entry works;
LOOK captures unchanged.

### Result (232f25ded) - 2026-11-14

Decisions 9 and 10 are implemented. The before/after numbers move work
from load to scroll (details below), so the owner may want to re-judge
scenario (e). The fix 6 e-only A/B recommends keeping it; fix 6 was not
re-applied.

Commits:

- fd1941cb1: the view kit observers take their margin as a scroll margin
  as well, and `wbHistory.bands` reports support for it.
- 48807e4c7: the viewport bands, content-visibility, above-view
  compensation, `boardJump` keep, graft, and pill.
- 6fb477c87: the headless `bands` scenario, `serve --mermaid`, and
  placeholder-aware fixtures.
- 235163d57 (lite review fix): a frame graft keeps `data-wbcv`.
- 232f25ded: `board-perf shot` re-reads its stop count at every stop.

Implementation.

- **Bands, one per topic.** Each board topic gets two
  `wbHistory.watch` subscriptions: NEAR (`100% 0px`) and FAR
  (`400% 0px`).
  - There is no second observer.
  - A topic's shown frames mount, paced, once it reports NEAR. They turn
    back into placeholders once it leaves FAR.
  - The placeholder keeps the frame's attributes and `style.height`. It
    also keeps a pending height that has not landed yet, and the swap's
    first-height marks (`wbKeep`, `wbGrow`).
  - Held frames stay mounted: a graft's pair (`wbNext`, `wbOut`), the
    focused frame, `mpend.frame`, and `wbHoldSelection`. Today the product
    sets `wbHoldSelection` only on reorder-hold's unmerged branch.
- **Scroll margins.** The kit passes `scrollMargin` where Chromium supports
  it, because `rootMargin` does not reach past `#bcol`'s clip. Without
  support, `bands` is false and every shown frame mounts as in Phase 1.
- **Content-visibility.** `content-visibility: auto` with
  `contain-intrinsic-size: auto 600px` applies only to a topic marked
  `data-wbcv`.
  - The mark is set on the topic's first band report, so
    `contain-intrinsic-size: auto` has already recorded the real size.
  - A freshly inserted CV topic reads 600px synchronously.
  - Maximized topics and topics holding a maximized panel are excluded.
  - A topic that is skipping its rendering mounts nothing. It mounts once
    it renders again; an iframe created inside a skipped subtree gets 0
    width.
- **Above-view compensation.** Since reorder-hold, `.bcol` and `.bpins`
  have `overflow-anchor: none`.
  - `topicResized`, a ResizeObserver on each topic, scrolls the topic's
    scroller only for a size change the bands themselves cause, and only
    for a topic wholly above the view.
  - Band-caused changes are a mounted or remounted frame's first height
    that changes it (`wbFirst` -> `wbLanded` in `applyFrameHeights`), and
    skip/unskip.
  - A running keep session compensates by itself. Any other above-view
    growth stays uncompensated, as before; the keep scenario pins that.
- **`boardJump`.** For a non-history target under the bands,
  `scrollIntoView` is followed by `wbHistory.keep(target)`. The target
  therefore holds while the topics it passed mount and settle.
- **`frameGraft`.** A frame in a skipped topic takes the placeholder path.
- **Swap marks.** The swap marks every `.doc-frame` `wbKeep`, so
  placeholders get the mark too.
- **Mermaid on remount.** A remount loads srcdoc fresh, so mermaid redraws
  by itself.
- **Load order.** The load band and mount pass moved after
  `window.wbForm()`, because `wbHistory` is defined there.
  - The pre-existing load-time `boardBoxes().forEach(watchBoard)` (about
    line 760) still runs before `wbForm` and is a no-op. It was left as it
    was, for reorder-hold to judge.
- **Fewer frames at load.** Placeholders with no height yet collapse the
  board at load, so about 16 frames mount at first (Phase 1: 32). Far ones
  then get heights only as the view approaches.
- **Rect readers (Decision 10).**
  - `pill.js` reads each topic's rect once per update, and a far item takes
    its topic's side.
  - `captureBoardHistory` / `wbHistory.capture` read no layout any more.
  - The old `stream.js` `position()` became the shared kit; its anchors
    are entries in view.
  - `below()`, `collapse`, `boardSpots` and the reorder-hold hold view read
    topic boxes only, which content-visibility leaves laid out.
- **Reorder-hold compatibility.** 8e74c185d wraps `watchBoard` with an
  `onChange` and reads topic rects. The bands use separate watches and
  never read inside skipped topics. Expect a textual merge conflict near
  the swap block's `ins` lines; there is no semantic conflict.

Harness numbers.

- Run: `run --tree before,after --scenarios a,c,e --runs 5 --gc --rates
  4,6`, alternating run by run.
- Before is c55f9f3f1 and after is 6fb477c87. Medians are shown with the
  spread for the larger side.

| run | Frames | built | LayoutCount | LayoutDur ms | RecalcDur ms | ScriptDur ms | TaskDur ms | long ms | Nodes |
|---|---|---|---|---|---|---|---|---|---|
| a@4 | 34 → 18 (0%) | 32 → 16 | 157 → 103 (12%) | 161 → 136 (23%) | 240 → 212 (22%) | 265 → 209 (27%) | 1480 → 1212 (17%) | 346 → 340 (16%) | 21379 → 19975 |
| a@6 | 34 → 17.5 | 32 → 15.5 | 164 → 104 | 267 → 220 | 394 → 350 | 429 → 340 | 2416 → 1971 | 569 → 526 | 21379 → 19931 |
| c@4 | 35 → 19 (0%) | 20 → 20 | 240 → 240 | 156 → 150 (10%) | 384 → 333 (9%) | 9423 → 9264 (5%) | 20849 → 20376 (4%) | 18955 → 18674 | 47924 → 46538 |
| c@6 | 35 → 18 | 20 → 20 | 240 → 240 | 237 → 230 | 588 → 510 | 14179 → 14077 | 31305 → 30979 | 28555 → 28522 | 47924 → 46450 |
| e@4 | 34 → 26 (15%) | 0 → 16 | 154 → 224 (4%) | 41.9 → 73.0 (16%) | 93.2 → 125 (18%) | 220 → 278 (23%) | 793 → 1078 (18%) | 0 → 0 | 21382 → 20760 |
| e@6 | 34 → 22 | 0 → 16 | 153 → 232 | 72.0 → 133 | 146 → 243 | 331 → 513 | 1198 → 1913 | 0 → 0 | 21382 → 20369 |

- **(a) improves.** At 4x: Frames -47%, LayoutCount -35%, TaskDuration
  -18% and ScriptDuration -21%. Quiet time drops 2094 → 1422 ms. The long
  task count is unchanged.
- **(c) is flat.** Frames -46%; everything else is within ±3%, except
  RecalcStyle at -13%.
- **(e) regresses.** At 4x: LayoutDuration +74%, TaskDuration +36%,
  ScriptDuration +26%. There are still no long tasks.
  - The cause is the 16 frames the scroll now builds as topics come near:
    work Phase 1 did at load moved into the scroll.
  - Summed over (a) and (e), TaskDuration at 4x is 2273 → 2290 ms.
  - The ticket's keep rule is a Phase 1 rule and the harness's `compare`
    prints FAIL for (e); this is recorded, not reverted.
- **Content-visibility split for (e).** e only, at 4x, 6 alternating runs
  per side, after vs after without the CV rule:
  - LayoutCount 227 vs 205 (5% spread)
  - LayoutDuration 76.5 vs 69.7 (17%)
  - RecalcStyle 141 vs 165 (20%)
  - TaskDuration 1125 vs 1141 (14%)

  On this synthetic board CV is neutral during scrolling. The (e) cost is
  the frame builds.

Headless checks (`scripts/board-headless.mjs`), all PASS on the final
tree, adapter and board page:

- **`bands`** (`serve --mermaid --topics 30`):
  - The mermaid topic is a placeholder at load, and topics in view are
    mounted.
  - A hash jump to the mermaid topic's panel (an unmounted entry) lands
    with ≤1px drift over 2 s, with mermaid drawn.
  - Up and down steps of 0.6 viewport move the reference topic exactly by
    the step (±1px), with no placeholder in view.
  - At the top, topics beyond 4 viewports are placeholders that keep their
    height, mermaid included.
  - Scrolling back down remounts the mermaid frame in a new frame, with
    the diagram drawn.
  - Without compensation the same scenario fails with an offset of
    1892px.
- **Other scenarios:**
  - `keep` and `observer`
  - `history` and `history-cards` at `--width 1800`
  - `history-regressions`
  - `history-paint`: its known ResizeObserver flake did not occur
- **The `history` adapter run.** It first failed:
  - Its `growFrame` fixture adds height inside a live frame document, and
    that height is lost when the frame unmounts.
  - The remounted frame then landed 1006px shorter above the view.
  - That is what made remounts count as band-caused height changes (see
    above).

LOOK.

- **Method.** `board-perf shot` on before and after, light and dark, 13
  stops each. The shot now re-reads its stop count at every stop, because
  the after-side column grows as frames mount.
- **Layout.** A per-stop layout dump of topic, frame, clip and fold
  positions and heights relative to `#bcol` is identical at every stop
  over everything up to the view. It differs only in topics below the
  view that have not been visited yet, which are unmeasured placeholders.
- **With CV, pixels differ 0.2-5.9% per capture.** The cause is a 1px
  shift of content inside topics.
  - Topic heights are fractional (for example 1301.75).
  - Content-visibility's paint containment snaps a topic's content to the
    topic's own pixel origin.
- **Without the CV rule, 0.007-0.88%.** Most of it is the clock labels.
  Stops 08, 11 and 12 show 1px text snapping on closed rows, with the
  layout identical; this appears to come from the far frames now being
  placeholders.
- **Verdict.** No layout or style change, but CV's 1px snapping is a real
  pixel change.

Fix 6 e-only A/B (lead note).

- Method: 143eedccb against its parent 277615827, scenario (e) only, 8
  alternating runs per side, `--gc`.
- Counts are identical on both sides: Nodes 19427, Frames 34, mounted 32,
  LayoutCount 154.

| metric | 4x before → after (spread) | 6x before → after (spread) |
|---|---|---|
| LayoutDuration | 34.2 → 38.9, +13.6% (33%) | 58.8 → 57.2, -2.7% (42%) |
| RecalcStyleDuration | 79.3 → 83.7, +5.5% (35%) | 124 → 122, -2.0% (44%) |
| ScriptDuration | 150 → 180, +20.2% (39%) | 269 → 262, -2.5% (72%) |
| TaskDuration | 618 → 724, +17.2% (27%) | 1042 → 1007, -3.4% (34%) |

- Every 4x change is within its spread, and 6x shows the opposite sign.
- No frame is built during (e) on that stack, so fix 6 has no mechanism to
  act there. The original (e) "regression" was noise.
- Recommendation: **keep (re-apply) fix 6**, for the owner to decide.
  - Its target, c@4 ScriptDuration, fell 77.7%.
  - Phase 2 now builds frames while scrolling (16 per (e) pass), where
    shared hashed frame assets save parse work.

Tests.

- New or changed tests:
  - `view_kit_test.go`: scroll margins
  - `board_frame_bands_test.go`: bands, compensation, and the CSS rule
  - `board_frame_lazy_test.go`: jump keep, skipped graft,
    `data-wbcv` kept, the first-height landing in the heights batch, and
    call sites after `wbForm`
  - `board_keep_test.go`
  - `pill_segments_test.go`: far items read by their topic
- Mutation checks confirm that the new cases catch their breakage.
- `go test ./internal/wsdashboard ./internal/wswhiteboard` passes, as does
  the `internal/mcp` dashboard, whiteboard, board, panel, theme, closed,
  topic and wiki subset. One earlier full run hit a
  `TestDevAssetsShowEditsWithoutARestart` client timeout under load; it
  passed 5/5 in isolation and in the final full run.

Lite review: no Critical or Important findings. It raised four Minor
findings:

- Graft dropped `data-wbcv`: fixed in 235163d57.
- The `wbHoldSelection` guard looked dead: kept, because reorder-hold's
  branch sets it.
- Mounting waits for the browser's own CV margin inside NEAR when a topic
  is skipped: accepted. The `bands` step check shows no placeholder in
  view at 0.6-viewport steps.
- The keep scenario was narrowed to topics that stay mounted: accepted.
  Late heights from a far topic's update now arrive as a placeholder,
  which is the design.

### Phase 3: Incremental board events

Requires `261113-feat-ws-dashboard-iframe-shell` Phase 2 (the render
fan-out cache). Before starting, the worker verifies that phase has landed
(`git log --grep=261113-feat-ws-dashboard-iframe-shell` shows a commit
recording its Phase 2 result) and stops and reports otherwise. As in Phase 2,
the prerequisite is prose, deliberately not a typed blocked-by.

**Resumption prerequisite (Phase 3 stop, 2026-11-14).** The original LOOK
failure below remains open. Before resuming its acceptance run, the root
lead must confirm that the separately owner-approved history-fit correction
(641/642; newest 3 -> 2 -> 1 panels within 1000px, minimum one panel,
first-usable-height cache, finalized boundaries never re-cut) has passed its
own review/native verification and landed. The correction's plan is
`ai-docs/.plans/2026-11/14-1357-history-fit-approved-panels.md` on its producer
branch `fix/history-fit-panels`; its Authority and Contract sections record
the separate approval. The root verified independently reviewed producer
`8f24cec5b9ee9e09d7c2d58e5002ddab03842152`, its final native checks, and
develop integration `28d8576226cc62312bf2d85d9c4cb306882f3df0`; the landed
stream SHA-256 is
`5a7e86ab9dc8624e2296335d113d69dcd349aaa65c0e0cbb8765aa806634555a`.
This satisfies the separate landing dependency only, not performance LOOK
or this ticket's mandatory resumption design gate. This retained worktree
still has the old source; the common-history A/B stack below must still be
prepared and verified through the root-owned integration/resumption path.
No source integration, activation or acceptance execution is authorized by
this fact-only ticket update. This need-first gate applies to Phase 3
resumption only, so it is prose rather than a ticket-wide typed
`blocked-by`.

- Changed-topics-only `board-topics` events with the `topics=1` opt-in and
  the full-region fallback (Decision 11).

Verification: a Go test that a one-topic change sends a `board-topics` event
carrying only that topic plus order; a page without the opt-in still
receives the full `region` event; a connection's first send, a reconnect,
and a non-topic board change each send the full region; a test that two
connections of one session cause one per-topic render (the fan-out cache); a
client asset test that an event the page cannot apply falls back to a full
region; harness scenario (c) before and after.

For the resumed long-board LOOK check, put the **same reviewed, landed
history-fit revision** on both the performance baseline and candidate;
record both exact source/commit identities and confirm the history source
is identical. Keep the existing board, dates, styles, viewport, capture
procedure and scroll stops unchanged apart from that common approved
prerequisite. Before comparing captures, verify matched finalized history
boundary IDs and settled frame geometry at corresponding stops, including
sequential/late iframe-height delivery without boundary re-cut. If either
side does not settle or the IDs/geometry differ, preserve the evidence and
return the discrepancy rather than force heights or normalize captures.
Repeat the long-board LOOK comparison and inspect the result captures
side by side. The no-intended-visual-change constraint judges the Phase 3
difference on this common-history baseline; it does not demand restoration
of the 640px history contract that the separate approval replaces only upon
landing (current source still uses STEP = 640 and cumulative snapping:
`agents-plugin-tool/internal/wswhiteboard/assets/stream.js#L28-L73`; approved
replacement: producer plan
`ai-docs/.plans/2026-11/14-1357-history-fit-approved-panels.md#L6-L7`).
Retain the original failing captures, 219px mismatch, traces and inferred
timing explanation as failed
historical evidence, never relabeling them as a pass. The original tests
cover relevant first-height/growth/restore behavior but do not explicitly
replay the captured three-frame 219px sequence (existing restore/graft cases:
`agents-plugin-tool/internal/wswhiteboard/stream_script_test.go#L375-L400`;
exact failing sequence: `ai-docs/tickets/assets/261113-feat-ws-whiteboard-board-frontend-performance/phase3/README.md#L128-L154`);
the resumed verification must account for that sequence. The Important
LOOK finding stays open, and no Phase 3 Result or ticket closure is recorded,
until this check and its
independent review pass. Existing scenario (c) metrics and the disclosed
35.5%/36.8% Nodes cost remain evidence of their tested identities, not a
substitute for LOOK parity or measurements of the common-history stack.

#### Implementation checkpoint (1341947b3) - 2026-11-14

Phase 3 is implemented in `3ec591d82`, with contract tests in `1341947b3`,
but is **not complete**: the long-board LOOK constraint remains an Important
independent review finding. No Phase 3 Result or ticket closure is recorded.

- Verified iframe-shell Phase 2 commit `3310356af` is an ancestor of this
  run's baseline `d1c3b9d6b`.
- Added T0 `topics=1` / `board-topics` on both SSE and WebSocket, with
  shared render decomposition, per-connection hash differences, first-send,
  reconnect and non-topic full-region fallbacks, and client resynchronization.
  Immutable incoming snapshots retain accumulated updates through hidden and
  deferred application, and feed the existing morph for pins and frame reuse.
- The dashboard/whiteboard suites pass (140.774s; whiteboard cached), as
  does the MCP dashboard/whiteboard/board/panel/theme/closed/topic/wiki subset
  (66.371s). The JS syntax check and focused parser test pass.
- Three alternating scenario (c) runs per side at 4x/6x reduced median
  ScriptDuration by 45.1%/46.2%, TaskDuration by 39.0%/38.9%, and long-task
  time by 62.0%/60.6%. Retained canonical inputs increased Nodes by
  **35.5%/36.8%**; that cost remains disclosed under Decision 12. The owner
  accepted this CPU/event/identity evidence, not LOOK parity.
- An isolated browser saw the candidate's initial full board followed by
  20 incremental updates, each carrying only T00, while preserving T01's
  element identity. Baseline updates were full regions.
- Independent lite review found no Critical or additional concrete logic
  defect, and one Important validation gap: later LOOK captures differ,
  with T04's history clip 219px taller on the candidate. Stops 06-11 differ
  by 15.310-52.863% of pixels. This is **not** a clock-only difference or a
  full LOOK pass.
- Observation-only diagnostic traces reproduced a persistent partially
  clipped entry on the baseline and candidate. The unchanged
  `wswhiteboard/assets/stream.js:53-67` measures extra height against the
  current clip but adds it to the original base; sequential iframe heights
  produce clip 640 -> 738 -> 859px with entry e723 still crossing the edge
  by 97.390625px after three further seconds. The history-live observer
  (`:363-378`) does not observe the clip itself. Different delivery batches
  are a supported explanation of the original difference, not proof that
  Phase 3 cannot influence its timing.
- Per the owner's resumption boundary, return this pre-existing history-snap
  blocker rather than expand the history contract or weaken source, dates,
  or fixtures to certify parity. Proposed next step: a separately authorized,
  regression-tested sequential-height snap repair, then repeat Phase 3 LOOK
  validation. No live activation, integration, launchers, or shared-state
  mutations occurred.

Exact baseline/candidate captures, commands, samples, traces, complete
measurement output, and independent review are preserved at
`ai-docs/tickets/assets/261113-feat-ws-whiteboard-board-frontend-performance/phase3/README.md`.
The original copies remain at `/tmp/ws-board-topics-phase3.3pBANc`.
