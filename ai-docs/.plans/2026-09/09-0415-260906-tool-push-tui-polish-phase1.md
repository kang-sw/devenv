# Plan: 260906-feat-ws-pi-tool-and-push-tui-polish — Phase 1: Complete the remaining display-only tool and push polish

## Relevant Ticket Contract

- Direct-tool previews cover exactly `do-i-really-have-to-read-this-myself`
  (`UGLY_READ_TOOL_NAME`) and `do-i-really-have-to-run-this-myself`
  (`ONE_LINER_EXEC_TOOL_NAME`). Both already register through the common
  `registerWsTool` renderer with no per-tool overrides (default YAML/RAW
  path). Preserve titles, arguments, read offset/limit semantics, command
  4KB/30s execution limits, and model-facing results unchanged; only the
  human-visible collapsed OUTPUT preview changes, to "at most ten
  newline-separated logical lines... not a strict cap of ten wrapped
  terminal rows," with an overflow/expand indicator and full recovery on
  expansion.
- Shared push presentation: apply the SAME "ten newline-separated logical
  lines" collapse + full expansion to all families the common push renderer
  registers (`ws-agent-report`, `ws-agent-settled`, `ws-agent-question`,
  `ws-agent-approval`, `ws-agent-advisory`, `ws-agent-orphaned`). Add a
  theme-aware **shared background** AND a subdued/gray **foreground**
  (foreground dimming is additional to the existing shared-background
  requirement) while retaining family/agent identity, status info,
  error/approval/question meaning, and existing interaction controls. This
  is display-only — model-facing payload, metadata, delivery, wake/settle
  ordering, report dedup, and controls are unchanged.
- Confirmed background policy (2026-09-08, reaffirmed): tool call/result
  input/output inherit Pi's uniform native parent lifecycle background;
  do NOT install a separate input/output background override, do NOT
  restore separated surfaces. This is a DIFFERENT surface from the push
  background, which remains in scope and must be adapter-drawn (pushed
  custom messages have no Pi-native parent lifecycle chrome to inherit).
- Reuse the cycle-free `src/tool-result-render.ts` helper and its native
  cached layout; do not revive custom grapheme/visual-row counting or an
  ASCII fallback; unavailable native helpers must fall back to Pi's
  standard per-slot display, never break registration or headless import.
- Sibling boundary: do not add dispatch-tool input-summary or
  resolved-model-line hooks — that is YAML Phase 2's scope, already merged
  (`.done/260906-feat-ws-pi-tool-result-yaml-tui-rendering.md`); the two
  direct tools and the push renderer never touch `ToolPreviewOverrides.buildCallPreview`/`resolvedLine`.
- Owner-live acceptance (real Pi TUI) is explicitly out of automated scope
  for this phase's implementation work.

## Out of Scope

- Any change to the five dispatch tools' (`explore`, `ws-execute`,
  `ws-agent-spawn`, `ws-agent-send`, `ws-fork`) existing `createDispatchToolPreview`
  summaries/resolved-model lines — YAML Phase 2, already done.
- Any change to bridged MCP tools' generic YAML/RAW preview behavior
  (the ~15 other `registerWsTool` call sites in `bridge.ts`, `ask.ts`,
  `fork.ts`, `lead-skills.ts`, `goal-loop.ts`, `spawner.ts`,
  `execute-gateway.ts`'s `GATED_EXEC_TOOL_NAME`/`APPROVE_TOOL_NAME`) — none
  of these pass the new `resultLineBudget` override, so their physical-row
  collapse policy stays byte-identical.
- Reintroducing separate tool input/output backgrounds, or any change to
  the bridged YAML input/output styling described in
  `ai-docs/spec/pi-adapter-runtime.md` §"Tool exposure and name
  sanitization" beyond incidental regression verification.
- `agents-plugin-tool/` (ws-mcp Go source) — untouched, adapter-only work.
- Owner-live real-TUI acceptance walkthrough (manual, post-implementation).
- `ConversationViewComponent`'s own collapsed tool-call/tool-result item
  rendering (`src/ask.ts` / spec §"Shared conversation-view component") —
  it already reuses "the same previews the push/tool-result surface
  produces" and needs no direct edit; verify only that this phase's changes
  to those shared previews don't regress it (a targeted look, not new work).

## Codebase Findings

- `agents-plugin-pi/src/execute-gateway.ts:739-793` — `UGLY_READ_TOOL_NAME`
  and `ONE_LINER_EXEC_TOOL_NAME` are registered via bare
  `registerWsTool(pi, {...}, toolPreviewTuiRef)` with no `renderCall`/
  `renderResult` in the definition and no overrides argument (there is
  currently no 4th param) — confirmed on the default YAML/RAW path.
- `agents-plugin-pi/src/tool-result-render.ts:116-119` — `PREVIEW_ROWS = 10`
  is a **physical wrapped-row** budget: `physicalPreviewLayout` (lines
  138-184) checks `rows.length === limit` inside the per-logical-line
  wrap loop (line 159), i.e. after every wrapped row, not after every
  logical (newline-separated) line. A single long logical line that wraps
  across multiple rows can exhaust the 10-row budget before 10 *logical*
  lines are shown — this is exactly the wrapped-row-vs-logical-line gap
  the ticket calls out.
- `agents-plugin-pi/src/tool-result-render.ts:536-561` — the default
  (`!overrides?.resolvedLine`) branch of `renderResult`, shared by every
  bare `registerWsTool` caller including the two direct tools, throws
  `UseNativeResultFallback` for partial/error/non-single-text results
  (preserving raw/error/non-text semantics), and otherwise calls
  `updateText(tui, component.output, rendered, ..., { expanded: options.expanded, trimOuterWhitespace: false }, previewTheme)` —
  **no `lineBudget` concept exists yet**; this is the exact call site to
  extend.
- `agents-plugin-pi/src/tool-result-render.ts:479-492` — `ToolPreviewOverrides`
  already has two optional hooks (`buildCallPreview`, `resolvedLine`).
  This is "the existing seam" the ticket says to satisfy the logical-line
  contract through — add a third optional field here, `resultLineBudget?:
  "physical" | "logical"`, rather than adding a new hook shape.
- `agents-plugin-pi/src/tool-result-render.ts:608-635` — `registerWsTool`'s
  default (no custom renderCall/renderResult) branch builds
  `cachedRenderers = createToolPreviewRenderers(tui, definition.name)` with
  **no overrides threaded through at all** — every bare-registered tool
  gets identical behavior today. `registerWsTool` needs a new optional 4th
  parameter (`overrides?: ToolPreviewOverrides`) forwarded into
  `createToolPreviewRenderers(tui, definition.name, undefined, overrides)`,
  so only the two direct-tool call sites (execute-gateway.ts:739,761) pass
  `{ resultLineBudget: "logical" }` and every other call site (which stays
  on the 3-arg form) is unaffected.
- `agents-plugin-pi/src/tool-result-render.ts:296-308` (`BoundedText.render`)
  — the `layoutKey` cache key currently encodes `expanded`,
  `trimOuterWhitespace`, `markerIndent`; must also encode the new
  `lineBudget`/`startIndent`/`continuationIndent` fields so a format change
  invalidates the cached physical layout.
- `agents-plugin-pi/test/native-tool-registration.test.ts:44-79` — the
  "actual native registrations" test iterates both direct tool names
  and asserts `renderCall && renderResult` exist and a cold ref (`ref.current`
  unset) throws. This must keep passing unchanged after adding the 4th
  `overrides` param (cold-fallback behavior is independent of overrides).
- `agents-plugin-pi/src/push-render.ts:126-147` (`buildPushComponent`) —
  confirmed current state per the ticket: emits **every** `parts.body`
  line as a separate `tui.Text` with no cap, no `Box` background (only
  `new tui.Box(1, 0)`, no 3rd `bgFn` arg despite `PushTuiModules.Box`
  already declaring one at line 99), and dims only `parts.status` via
  `theme.fg("dim", ...)`; head uses `"customMessageLabel"`, body uses
  `"customMessageText"`.
- `agents-plugin-pi/src/push-render.ts:159-167` (`registerPushMessageRenderers`)
  — the registered callback signature is `(message, _options, theme) =>
  buildPushComponent(tui, message, theme)`; `options.expanded` is
  **discarded** (`_options`). This is the pre-existing hook to wire real
  expand/collapse through — `MessageRenderOptions.expanded` (see below)
  already exists on the callback Pi invokes.
- `node_modules/@earendil-works/pi-coding-agent/dist/modes/interactive/theme/theme.d.ts:4-5`
  — `ThemeColor` already includes `"muted"` and `"dim"`; `ThemeBg` already
  includes `"customMessageBg"` (used by Pi's OWN default custom-message
  box) alongside `"toolPendingBg"/"toolSuccessBg"/"toolErrorBg"`. These are
  the exact tokens to use for the push renderer's shared background
  (`theme.bg("customMessageBg", text)`) and gray foreground
  (`theme.fg("muted", ...)` or `"dim"`), instead of inventing new colors.
- `node_modules/@earendil-works/pi-coding-agent/dist/modes/interactive/components/custom-message.js:7-85`
  (`CustomMessageComponent`) — ground truth for push lifecycle: it holds
  `_expanded` state itself and calls `this.rebuild()` (which **re-invokes
  our registered renderer from scratch**, `this.customRenderer(this.message,
  {expanded, outputPad}, theme)`) on `setExpanded()`, `setOutputPad()`, and
  `invalidate()` (theme change). Its own default box already uses
  `new Box(1, 1, (t) => theme.bg("customMessageBg", t))` for the exact
  purple-background pattern to mirror for our custom-rendered box. This
  means: (a) our renderer is called fresh with the live `theme` on every
  expand toggle and every theme change — no cross-call state/`context`
  plumbing is available or needed (unlike tool renderCall/renderResult's
  `context.lastComponent` reuse); (b) the internal caching payoff is
  *within* one renderer call's returned Component, across ordinary
  same-content redraws (e.g. terminal resize/repaint) between rebuild
  triggers — exactly what `BoundedText`'s width-keyed cache already does.
- `node_modules/@earendil-works/pi-tui/dist/tui.d.ts:9-29` — `Component`
  requires both `render(width): string[]` and `invalidate(): void`;
  `push-render.ts`'s own `PushTuiModules.Box` return type (line 99-104)
  currently omits `invalidate()` from its declared shape (the real Box
  has it; only the local TS surface doesn't expose it) — irrelevant for
  correctness (per the CustomMessageComponent trace above `rebuild()`
  discards our whole component rather than calling `invalidate()` on it),
  but the type should be widened to `Component`-shaped for any child we
  add that itself needs `invalidate()` (a `BoundedText` reused from
  tool-result-render.ts implements it).
- `agents-plugin-pi/test/push-render.test.ts:96-119` (`fakeTui`) — the
  existing `FakeBox` test double does **not** apply a captured background
  function to rendered lines and ignores a 3rd constructor arg entirely
  (unlike `test/tool-result-render.test.ts:48-71`'s `FakeBox`, which DOES
  capture `background` and applies it per line in `render()`). This double
  needs updating to mirror the tool-result-render test's `FakeBox` so
  background assertions are meaningful.
- `ai-docs/spec/pi-adapter-runtime.md:59-88` — the only current tool-display
  spec section; documents the physical-row ("ten content rows") collapse
  policy for bridged/dispatch tools and the native-parent-lifecycle
  background policy. This section is retained verbatim (regression-only);
  a NEW subsection is needed for (a) the two direct tools' distinct
  ten-*logical*-line contract and (b) the push-message collapse/background/
  foreground contract — neither currently documented anywhere in this file.

## Implementation Plan

1. **`src/tool-result-render.ts` — add a logical-line budget mode to the
   shared layout, additive only.**
   - Extend `PreviewFormat` (line 66-71) with three new optional fields:
     `lineBudget?: "physical" | "logical"`, `startIndent?: number`,
     `continuationIndent?: number`. Every existing call site that doesn't
     set them keeps today's behavior byte-for-byte (defaults resolve to
     the current `INPUT_START_INDENT`/`CONTINUATION_INDENT` constants and
     the current physical-row check).
   - In `physicalPreviewLayout` (lines 138-184): swap the hardcoded
     `INPUT_START_INDENT`/`CONTINUATION_INDENT` references for
     `startIndent ?? INPUT_START_INDENT` / `continuationIndent ?? CONTINUATION_INDENT`.
     Add a `logicalLineIndex` counter incremented once per completed
     logical line (after its `do...while` wrap loop finishes, before
     advancing `lineStart`). When `lineBudget === "logical"`, check
     `logicalLineIndex === limit` **once, at the top of the outer loop**
     (before wrapping the next logical line) instead of the existing
     `rows.length === limit` check inside the wrap loop; the existing
     physical-row check stays active (only) when `lineBudget` is not
     `"logical"`. This guarantees a logical line, once started, always
     finishes wrapping — the cap only ever falls on a logical-line
     boundary.
   - Extend `BoundedText.render`'s `layoutKey` (line 296-298) to also
     include `component.format.lineBudget ?? "physical"` (the two indent
     fields don't need to be in the key unless a caller varies them across
     renders on the same instance, which none will — note this as a
     defensive nice-to-have only if trivial).
   - Export `BoundedText`, `PreviewFormat`, `createBoundedText`, and
     `updateText` (currently private, lines 73-89, 66-71, 276-347,
     349-373) so `push-render.ts` can reuse the exact same cached
     preview-text component instead of re-implementing width-keyed
     caching. No behavior change to any existing caller — purely widening
     the module's export surface.
   - Add `resultLineBudget?: "physical" | "logical"` to
     `ToolPreviewOverrides` (line 479-492).
   - In `createToolPreviewRenderers`'s default `renderResult` branch (line
     536-561), thread it through: the `updateText(...)` call for
     `component.output` gets `lineBudget: overrides?.resultLineBudget` added
     to its format object. (Leave the `resolvedLine`-override branch,
     lines 564-597, and the call-preview/`component.input` formatting
     entirely alone — out of scope, unaffected by tools that don't set the
     new field.)
   - Widen `registerWsTool`'s signature (line 608-635) with an optional
     4th parameter `overrides?: ToolPreviewOverrides`, forwarded into
     `createToolPreviewRenderers(tui, definition.name, undefined, overrides)`
     inside its `renderers()` closure. Every existing 3-arg call site is
     unaffected.

2. **`src/execute-gateway.ts` — opt the two direct tools into the logical
   budget through the seam from step 1.**
   - Line 739 (`UGLY_READ_TOOL_NAME`) and line 761
     (`ONE_LINER_EXEC_TOOL_NAME`): change
     `registerWsTool(pi, {...}, toolPreviewTuiRef)` to
     `registerWsTool(pi, {...}, toolPreviewTuiRef, { resultLineBudget: "logical" })`.
     No other change to either tool's `execute`, schema, or description.

3. **`src/push-render.ts` — collapsed/expanded body, shared background,
   muted foreground.**
   - Widen `PushTuiModules` (line 98-104) to also declare
     `stripTerminalSequences`/`truncateToWidth` (both already present at
     runtime via `loadHostPiTui()`, matching `ToolResultTuiModules`) so a
     value of this shape can be passed to `createBoundedText`/`updateText`
     imported from `tool-result-render.ts`. Import those two plus
     `type BoundedText` from `./tool-result-render.ts` (a new,
     one-directional, cycle-free edge: `tool-result-render.ts` imports only
     from `./pi-tui.ts`).
   - Extend `PushRenderTheme` (line 107-110) with an optional
     `bg?(color: string, text: string): string` alongside the existing
     `fg?`/`bold?`, mirroring the existing defensive `paint` helper's
     try/catch-and-fall-back-to-plain-text pattern for a new `paintBg`
     helper.
   - Rewrite `buildPushComponent` (line 126-147) to accept a 4th
     parameter `expanded = false`:
     - Compute the body's full text as `parts.body.join("\n")`.
     - Build one `BoundedText` via `createBoundedText(tui)` and
       `updateText(tui, bodyText, joinedBody, (t) => paint("muted", t) /* or "customMessageText"+muted per decision below */, { expanded, trimOuterWhitespace: false, lineBudget: "logical", startIndent: 0, continuationIndent: 0, markerStyle: (m) => paint("muted", m) }, theme)`
       — `startIndent: 0`/`continuationIndent: 0` preserve today's
       flush-left push body look (the shared layout's default 4/3-column
       indent is a tool-preview convention, not a push one).
     - Keep `head`/`status` as plain `tui.Text` instances (short,
       fixed-shape single lines; no logical-line cap needed) but recolor
       both to a muted/gray token (e.g. `"muted"` for head, keep `"dim"`
       for status — both already-gray tokens) instead of
       `"customMessageLabel"`/`"customMessageText"` — satisfies "gray out
       the foreground... not merely the background" for the whole message
       while leaving `"dim"` status coloring as-is (already gray).
     - Construct the `Box` with the shared background:
       `new tui.Box(1, 0, (t) => paintBg("customMessageBg", t))` (3rd ctor
       arg, already declared on `PushTuiModules.Box` but never passed
       today).
     - `addChild` the head `Text`, then the body `BoundedText` (a
       `render(width)`/`invalidate()`-shaped component, exactly like
       `inputBox.addChild(createInputPreview(input))` in
       `tool-result-render.ts:404`), then the status `Text` if present.
   - Update `registerPushMessageRenderers` (line 159-167) to stop
     discarding `options`: pass `options.expanded` through as the new 4th
     `buildPushComponent` argument.
   - Note for the implementer: `buildPushComponent`'s renderer is called
     FRESH by `CustomMessageComponent.rebuild()` on every expand toggle
     and every theme change (see Codebase Findings) — no `context`/
     `lastComponent` reuse across calls is needed or possible here; the
     `BoundedText`'s own internal width-keyed cache is what pays for
     itself across ordinary same-content redraws within one call's
     returned component, matching the tool-preview precedent.

4. **`ai-docs/spec/pi-adapter-runtime.md` — Spec Impact.**
   - In the existing "Tool exposure and name sanitization" section
     (currently ends around line 88 with "...both slots retain Pi's
     standard display."), add a clearly-scoped addendum (new paragraph or
     small subsection) stating: `do-i-really-have-to-read-this-myself` and
     `do-i-really-have-to-run-this-myself` collapse their completed
     single-text result to at most ten **newline-separated logical
     lines** (not ten wrapped terminal rows — a distinct budget from the
     generic bridged/dispatch ten-content-row policy documented above),
     with a truncation marker and full recovery on expansion; everything
     else about their display (native cache reuse, RAW/error/partial
     fallback, headless safety) matches the existing bridged contract.
   - Add a new subsection (near the push/report-channel material, e.g.
     after the "Child→lead report channel" heading) documenting: pushed
     `ws-agent-*` messages collapse their payload body to ten logical
     lines with full-expansion recovery via Pi's own expand control
     (`MessageRenderOptions.expanded`); the whole message (head, body,
     status) paints with a theme-aware shared background
     (`customMessageBg`) and a subdued/gray foreground (`muted`/`dim`),
     retaining family/agent identity, status meaning, and existing
     controls; this changes human rendering only — model-facing content,
     metadata, delivery, wake/settle ordering, dedup, and fan-in are
     unchanged.
   - Explicitly note both additions are this ticket's Phase 1 only; YAML
     Phase 2's dispatch-summary/resolved-model-line contract (already
     documented/landed) is unchanged.

## Verification Plan

- `node --test test/tool-result-render.test.ts` — extend with:
  - Unit coverage of `physicalPreview`/`physicalPreviewLayout` with
    `lineBudget: "logical"`: a long single logical line that would exceed
    10 physical rows on its own must render in full (no premature cut)
    when it is the only logical line; 10 short logical lines render fully
    with no marker; an 11th logical line triggers the marker at the
    logical-line boundary regardless of how many physical rows the first
    10 consumed; `expanded: true` removes the cap entirely (mirrors the
    existing "wraps long logical rows before the ten-row budget and
    expands full output" test at line 296-310, but asserting on *logical*
    count instead of physical-row count).
  - A `createToolPreviewRenderers(..., overrides: { resultLineBudget: "logical" })`
    test confirming only the OUTPUT format changes (input/title
    formatting, partial/error/non-text fallback via
    `UseNativeResultFallback`, and empty-result handling stay identical to
    the existing fallback-cases table at line 439-450).
- `node --test test/native-tool-registration.test.ts` — the existing
  "actual native registrations retain schemas/executors while sharing cold
  and late-filled preview refs" test (line 44-79) must keep passing
  unchanged (cold-ref throw, shared ref, schema/executor identity for both
  direct tools) after `registerExecuteGateway` starts passing the 4th
  overrides arg; add a case exercising the two direct tools' actual
  registration end-to-end with a >10-logical-line multi-line text result,
  asserting the rendered collapsed output shows exactly 10 logical
  lines + marker and the expanded render shows all lines, through the real
  `registerExecuteGateway`-produced tool (not just the pure helper).
- `node --test test/push-render.test.ts` — extend with:
  - Update `fakeTui`'s `FakeBox` (line 96-119) to capture and apply a 3rd
    constructor `bgFn` arg per rendered line (mirroring
    `tool-result-render.test.ts`'s `FakeBox`), and to expose
    `invalidate()`.
  - Short (<10 line), exactly-10-line, and >10-line (long) report/settled
    bodies: collapsed shows the capped body + marker, `expanded: true`
    (passed as the renderer's `options.expanded`) shows the full body.
  - Assert the shared `customMessageBg` background call reaches every
    push family (loop over `PUSH_FAMILIES`, extending the existing
    `registerPushMessageRenderers` test at line 172-193).
  - Assert head/body/status all use a muted/dim foreground token (not
    `"customMessageLabel"`/`"customMessageText"`), while head/body/status
    TEXT content, identity, and status-line detection
    (`buildPushRenderLines`, untouched) are unchanged — reuse the existing
    fixtures at lines 27-94 verbatim for content-shape coverage.
  - No-theme and throwing-theme degrade gracefully (extend the existing
    test at line 143-157) for the new `paintBg` helper the same way
    `paint` already degrades.
- Full adapter suite: `cd agents-plugin-pi && npm test` (or the project's
  configured `node --test test/` runner) — must stay green; explicitly
  confirm headless/no-native-tui import paths (`ref.current` unset /
  `loadHostPiTui()` unavailable fallback) are unaffected for both files.
- Manual/documented-only: real-Pi-TUI owner-live acceptance (theme
  switching, long-session responsiveness comparison) — out of automated
  scope per the ticket; do not attempt to simulate in unit tests beyond
  the theme-change unit assertions above.

## Escalations

- None.
