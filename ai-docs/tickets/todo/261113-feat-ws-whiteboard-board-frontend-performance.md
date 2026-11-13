---
title: "Whiteboard board frontend performance: lazy frames, cheaper swaps, measurement harness"
related:
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "this ticket absorbs its board scope ((417) note typing, long whiteboard) and the (473) Mac/Windows measurement"
  261113-feat-ws-dashboard-iframe-shell: "independent; Phase 1-2 land without it; Phase 3 waits for its Phase 2 (fan-out cache, sse.go)"
  261113-feat-ws-dashboard-board-reorder-hold: "prerequisite for Phase 2 only: its Phase 1 builds the shared visibility observer in wswhiteboard/assets/stream.js"
  261113-feat-ws-whiteboard-topic-history-load-more: "shares the visibility observer with Phase 2; its worker decides template- vs fetch-based older pages"
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
   `srcdoc` for frames; the client sets `srcdoc` when the frame's whole
   `<details>` chain is open, on `toggle`, and on insert in `morphBoard`,
   `adopted` and `wbhistoryload`. Heights stay keyed by `data-doc`
   (`frameHeights` / `setFrameHeights`). The static full page (`framed=false`)
   is unchanged.
3. **Batched frame heights** ((532)=a): height messages are queued in a Map
   and applied in one `requestAnimationFrame`; the "from" height is read from
   `style.height`, not `getBoundingClientRect`; `docFrame` resolves through a
   `Map(contentWindow -> iframe)` refreshed on insert.
4. **Typing and idle paths** ((532)=a): `pill.js` defers `input`-driven
   `update` to rAF / a short debounce (or re-classifies only the edited item);
   `form.js` `build()` coalesces to rAF and scopes its class toggles to the
   changed item's topic; `tickTimes` collects its `time[data-f]` set once per
   swap instead of querying the document every second.
5. **Per-topic hash** ((532)=a): the server emits a per-topic content hash
   attribute (`data-th`); `morphBoard` compares hashes instead of
   `outerHTML`. T0 attribute.
6. **Keyed frame reuse** ((532)=a): inside a changed topic, an incoming frame
   whose immutable `data-doc-key` (`entry:eN`) matches an old frame takes the
   old frame via `Element.moveBefore`, generalizing `frameGraftable`'s
   first-frame graft; without `moveBefore`, today's behaviour stays.
7. **Hashed shared frame assets** ((532)=a): frame CSS and JS are served as
   public, immutable, content-hashed assets (`<link>` / `<script src>`)
   instead of inline, following the mermaid route precedent
   (`access.go:129-138`, `pages.go:1048, 1249-1254`). T0 additive route. The
   theme pin (`ThemeFrames`) is unaffected; the session-page CSP has no
   `style-src` / `script-src` today (`pages.go:1276`): if iframe-shell or
   browser-pane adds one when this lands, the hashed asset path is allowed in
   it.
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
   run there; the (473) Windows numbers are recorded when that run happens.
   Rejected: committing Playwright as a repo dependency (the cached
   Playwright under `~/.npm/_npx` is used, and the harness says how to get
   it).
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

## Phases

### Phase 1: Measurement harness and S fixes

- The harness (Decision 8), with a baseline run recorded before any fix.
- Lazy frame mount (Decision 2), batched heights (Decision 3), typing and
  idle paths (Decision 4), per-topic hash (Decision 5), keyed frame reuse
  (Decision 6), hashed shared frame assets (Decision 7), in that order, each
  measured A/B with the harness.

Verification: the Result records the harness numbers per scenario at 4x and
6x before and after each fix (Frames, LayoutCount, LayoutDuration,
ScriptDuration, long tasks, keystroke Event Timing); asset tests that frames
under a closed `<details>` carry `data-srcdoc` and mount on open, on insert
and after `wbhistoryload`; a test that N height messages cause one host
layout batch; a test that an unchanged topic (same `data-th`) is not
replaced and a changed topic keeps frames whose `data-doc-key` is unchanged;
a Go test for the hashed asset route (immutable cache headers, hash in the
path, served without a session); LOOK captures of a long board before and
after with no visual difference; the dashboard and whiteboard Go and JS test
suites pass.

### Phase 2: Viewport mount/unmount and content-visibility

Requires `261113-feat-ws-dashboard-board-reorder-hold` Phase 1 (the shared
visibility observer).

- Viewport mount/unmount of open frames (Decision 9).
- `content-visibility` on topics with the rect-reader audit (Decision 10).

Verification: harness numbers for scenarios (a), (c) and (e) before and
after; a headless check that scrolling a long board mounts frames ahead of
the viewport, unmounts far ones with their height kept (no scroll jump), and
remounts with mermaid redrawn; `boardJump` to an unmounted entry works;
LOOK captures unchanged.

### Phase 3: Incremental board events

Requires `261113-feat-ws-dashboard-iframe-shell` Phase 2.

- Changed-topics-only board events (Decision 11).

Verification: a Go test that a one-topic change sends only that topic plus
order; a page without the opt-in still receives the full region; harness
scenario (c) before and after.
