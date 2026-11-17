# Dashboard: board note typing lag (whole-document restyle per keystroke)

## Context

Owner 2026-11-17 09:22: typing in a board note (topic/ask note wrapped by the
capsule contenteditable `.wsc-editor`) lags enormously, in Electron and in
the PWA. Trace `Trace-20261117T092233` (Korean IME, ~25 s typing at ~99 ms
cadence): input-to-frame median 2.0 s, max 5.7 s; keydown median 5.4 s. The
main thread is 91% busy; each keystroke costs ~110-125 ms, almost all native:

- whole-document style recalc once per `input` (always 14,962 elements =
  the entire document), 34-45 ms, then layout 30-40 ms (root `#document`,
  18.5k layout objects);
- native typing layout 27-43 ms inside keypress/textInput (root `#document`);
- JS only 1.5-3 ms.

Per-keystroke JS chain that invalidates: `capsules.js` editor `input`
listener (~205) -> `notify` (175-180) -> `cache()` (79, sets the hidden
backing textarea value) + synthetic `input` -> board doc listener
`dashboard-composer.js:1702-1708` -> `syncChips` (83-90) -> `renderChips`
(91+, `cchips.textContent=""` at 95 and rebuilds every chip, including a
`[topic] note text` chip that changes every keystroke).

Likely reason every DOM change restyles the whole document: `:has()` rules
added 11-15/11-16, e.g. `dashboard-layout.css:18`
(`#bcol:has([data-max], ...) {contain:style}`, fe8eda6e8),
`dashboard-layout.css:183` (`body:has(.fpane[data-max])`),
`dashboard-drawer.css:53` (`:root[data-shell]:has(.settings-pop...)`),
`dashboard-rail.css:57` (3fdb58c7c), `dashboard-rail.css:134`,
`dashboard-board.css:4`. Unproven (no invalidation tracking in the trace).
The capsule editor itself came from 44d01be77 (11-15).

A parallel worker is changing `wswhiteboard/assets/pill.js`, `rail.js` and
`wsdashboard/assets/dashboard-lifecycle.js` (forced layouts while
scrolling); stay out of those files.

## Outcome

1. Typing in a board note keeps up with fast typing (including Korean IME):
   per-keystroke main-thread cost well under one frame on a board the size of
   the trace (~15k elements), no queue build-up.
2. No whole-document style recalc per keystroke: the chip strip updates in
   place (only the changed chip's text) or once per animation frame, and the
   hidden backing textarea is not rewritten per keystroke if that is what
   invalidates (keep the field's value readable for send/save).
3. The `:has()` rules that make a local DOM change invalidate the whole
   document are replaced by a class/attribute toggled by JS on the state
   change, or scoped, with the same visual behaviour.
4. Behaviour unchanged: chips, references, IME composition, drafts, send.

## Approach

- Measure first with a Playwright/Chromium harness (real CSS/JS, a board of
  similar size, e.g. a page rendered by the Go fixtures on w5090 saved as
  static HTML, or a static harness): trace with the
  `disabled-by-default-devtools.timeline.invalidationTracking` category,
  type into a note, and identify which mutation/selector invalidates the
  whole document. Fix the proven causes; report before/after per-keystroke
  style element counts and ms.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing
  `agents-plugin-tool/`.
- View-only (T0). No new CSS class names colliding across modules
  (TestCSSClassesDoNotCollideAcrossModules).
- NO local `go test`, Go test exes, ws/ws-mcp executables, or launcher on this
  PC. Node tests locally. Go only on remote w5090:
  `D:/sys_lock.sh remote w5090 run <who> <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/groove-animosity-playroom' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard ./internal/wswhiteboard'`.
  Accepted reds: only TestTopicStateBadgeResearchingMetadata and
  TestTopicStateBadgeClockAndReconciliation.
- Temp only under `D:/Temp/note-typing-lag/`, deleted after.
- Commits carry no Co-Authored-By trailer. Do not push develop or main.

## Verification

- Before/after numbers from the harness (style recalc element count and ms
  per keystroke, total per 50 keystrokes).
- Existing Node tests (capsules, composer chips, IME) pass; add a test that
  the chip strip is not rebuilt on every input when the chip set is
  unchanged.
- Go tests for wsdashboard/wswhiteboard on w5090.
- R1 review; fixes-only R2 for substantive findings.
