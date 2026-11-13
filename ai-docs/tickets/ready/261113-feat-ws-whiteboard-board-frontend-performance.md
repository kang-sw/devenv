---
title: "Whiteboard board frontend performance: lazy frames, cheaper swaps, measurement harness"
related:
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "this ticket absorbs its board scope ((417) note typing, long whiteboard) and the (473) Mac/Windows measurement"
  261113-feat-ws-dashboard-iframe-shell: "independent; Phase 1-2 land without it; Phase 3 waits for its Phase 2 (fan-out cache, sse.go)"
  261113-feat-ws-dashboard-board-reorder-hold: "prerequisite for Phase 2 only: its Phase 1 builds the shared visibility observer in wswhiteboard/assets/stream.js"
  261113-feat-ws-whiteboard-topic-history-load-more: "shares the visibility observer with Phase 2; its worker decides template- vs fetch-based older pages"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 55c7dce0b4a4e217
sage-review-completeness-reviewed: 55c7dce0b4a4e217
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
| scope.span | multi-file | wswhiteboard/render.go, wsdashboard/assets/dashboard.js, wswhiteboard/assets/form.js, pill.js, stream.js, wsdashboard/access.go, wsdashboard/pages.go, scripts/dev.py area |
| scope.surface | cross-module | server emits data-srcdoc, data-th, hashed asset route across wswhiteboard and wsdashboard; Phase 3 adds an event type |
| scope.new_public_symbol | yes | new immutable hashed frame-asset route, data-th attribute, opt-in board event type |
| scope.new_type_contract | no | none; wire additions only (attributes, route, event) |
| scope.test_surface | existing | wsdashboard/board_close_fold_test.go, cards_test.go, board_motion_test.go, mcp/dashboard_page_test.go, wswhiteboard/regions_test.go (several assert srcdoc= literally and need updating) |
| complexity.reuse_points | confirmed | mermaid hashed route (access.go publicAsset, pages.go serveMermaid), frameGraftable, setFrameHeights, reorder-hold visibility observer (pending 261113-feat-ws-dashboard-board-reorder-hold) |
| complexity.side_effect_risk | high | iframe mount timing, height keying, frameGraft, FLIP, mention selection and rect readers interact; moving frames reloads them today |
| risk.correctness | high | lazy srcdoc and moveBefore frame reuse can lose heights, mermaid redraw, or scroll position |
| risk.fit | moderate | overlaps in-flight iframe-shell and reorder-hold on dashboard.js and stream.js; rebase expected |
| risk.test | high | headless Chromium harness and visual LOOK checks needed; existing regex tests pin srcdoc= |
| risk.security_or_contract | moderate | new public unauthenticated asset route and CSP interplay; asset is static and content-hashed (T0) |

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

### Phase 3: Incremental board events

Requires `261113-feat-ws-dashboard-iframe-shell` Phase 2 (the render
fan-out cache). Before starting, the worker verifies that phase has landed
(`git log --grep=261113-feat-ws-dashboard-iframe-shell` shows a commit
recording its Phase 2 result) and stops and reports otherwise. As in Phase 2,
the prerequisite is prose, deliberately not a typed blocked-by.

- Changed-topics-only `board-topics` events with the `topics=1` opt-in and
  the full-region fallback (Decision 11).

Verification: a Go test that a one-topic change sends a `board-topics` event
carrying only that topic plus order; a page without the opt-in still
receives the full `region` event; a connection's first send, a reconnect,
and a non-topic board change each send the full region; a test that two
connections of one session cause one per-topic render (the fan-out cache); a
client asset test that an event the page cannot apply falls back to a full
region; harness scenario (c) before and after.
