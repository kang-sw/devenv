# Plan: 260908-feat-ws-pi-conversation-view-component — Phase 1: pi-tui dependency and the shared component

## Relevant Ticket Contract

- Golden rule: `agents-plugin-tool/` untouched; all work in `agents-plugin-pi/`.
- Dependency: add `"@earendil-works/pi-tui": "0.84.4"` (exact pin, matching the
  version `pi-coding-agent`'s own `^0.84.4` range nests) to
  `agents-plugin-pi/package.json`; `npm ls @earendil-works/pi-tui` must report
  one deduped copy — two copies is a stop-and-align outcome, never ship-with-two.
- One resolution point: new `agents-plugin-pi/src/pi-tui.ts` re-exports what the
  adapter uses from pi-tui through one static import. The component and the
  three existing guarded dynamic importers (`push-render.ts`,
  `tool-result-render.ts`, `index.ts`) import from it; their "pi-tui
  unavailable" fallback branches (dead once the package resolves) and the
  tests exercising those branches are deleted. `overlay-chat.ts`'s own guarded
  dynamic import (`loadMarkdownRenderer`) is **not** one of the three named
  importers and stays untouched — it is deleted only in Phase 2 with the rest
  of the file.
- Runtime instance: the epic requires the runtime to use the host's pi-tui
  instance. Phase 1 verifies instance identity, not just version, inside a
  live lead; this check is owner-run and recorded, not an automated gate. The
  only acceptable fallback if identity fails is `pi-tui.ts` resolving through
  the host at runtime (the `loadMarkdownRenderer` shim shape) while tests keep
  the static path — never a duplicated instance.
- Build `conversation-view.ts`: `ConversationViewComponent` (pi-tui
  `Component`: `render(width)`, `handleInput(data)`) with the `ConversationItem`
  model, `mode: "view" | "interactive"` (raise-only via `setMode`), the
  `ConversationChannel`/`ChildLiveness` liveness input read at render time, and
  the full key contract (Tab/Shift+Tab/Space/Ctrl+O precedence rules, Esc via
  `isEscapeKey`, `\x03` swallowed in both modes, `/done` interception, Enter in
  `view` mode → `onEnter`), on `ScrollView`/`Markdown`/`Text`/`Editor`.
- Primitives factory: each of the four primitives must construct/render under
  `node --test` with a fake `tui` exposing `requestRender` only and no host
  theme; whichever primitive cannot is reached through an injectable
  `primitives` factory on the component (defaulting to the real classes) so
  tests inject a minimal fake for only that one primitive — offline
  `render(width)` tests at 40/80/120 columns are the ticket's evidence, not a
  live TTY.
- Move `visibleWidth` (and any helper pi-tui lacks) to a new
  `agents-plugin-pi/src/text-width.ts`; repoint `agent-widget.ts` at it instead
  of `overlay-chat.ts`. Leave `overlay-chat.ts` running untouched otherwise.
- Tool items: head line and expanded body come from `tool-result-render.ts`
  (`yamlInputPreview` for a call head, `completedTextPreview`/`logicalPreview`
  for a result head, `yamlContainerDisplay` for the expanded body) — the
  component adds only collapse/expand chrome, never its own preview logic.
- Width invariant carried over from the existing overlay/widget discipline:
  every rendered line's `visibleWidth(line) <= width` at 40/80/120 columns.
- Out of this phase (do not implement): `ask.ts` rebinding, `overlay-chat.ts`
  deletion, persisted-transcript migration, the audit window, the
  owner-steering ownership rule, the Esc modal — all Phase 2 / child B.

## Out of Scope

- Phase 2 (`ask.ts` → `ConversationChannel` rebind, `overlay-chat.ts` deletion,
  `TranscriptEntry[]` → `ConversationItem[]` hydration, spec amendment) —
  explicitly the next phase in the same ticket.
- Child B (`260908` sibling ticket): the audit window, `idle-awaiting-owner`
  production logic, the Esc modal. Phase 1 only defines the `ChildLiveness`
  type and renders its three states structurally against a fake channel.
- `overlay-chat.ts`'s own `loadMarkdownRenderer` dynamic import and its
  `test/overlay-chat.test.ts` "unavailable" case — not one of the three named
  importers, stays as-is until Phase 2 deletes the file.
- `agents-plugin-tool/` — golden rule, untouched.
- Any change to `spawner.ts`'s `streaming`/`threadBound`/`overlayAttached`
  record semantics (ticket Constraints: no change in this ticket).

## Codebase Findings

- `agents-plugin-pi/package.json#L14-L17` — current `dependencies` list only
  `@earendil-works/pi-coding-agent: ^0.84.4` and `yaml: 2.9.0`; no `pi-tui`
  entry yet. `npm ls @earendil-works/pi-tui` today shows exactly one copy,
  nested: `ws-pi-bridge → @earendil-works/pi-coding-agent@0.84.4 →
  @earendil-works/pi-tui@0.84.4`. `pi-coding-agent`'s own `package.json`
  depends on `"@earendil-works/pi-tui": "^0.84.4"` — compatible with the exact
  `0.84.4` pin the ticket adds, so `npm install` should hoist/dedupe to one
  top-level copy rather than producing two.
- `node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-tui/dist/index.d.ts` —
  the package's real export surface: `Box`, `Editor`, `Markdown`, `ScrollView`,
  `Text`, `TruncatedText`, `VStack`/`HStack`, `Container`, `visibleWidth`,
  `stripTerminalSequences`, `truncateToWidth`, `wrapTextWithAnsi`, key/kitty
  helpers (`isEscapeKey`-equivalent building blocks: `decodeKittyPrintable`,
  `parseKey`, `isKittyProtocolActive`), and `TuiMainScreen`/`TuiAltScreen`
  (concrete `TUI`-implementing classes) plus `type TUI` (an **interface**, not
  a class). **`visibleWidth` is already exported by pi-tui** — the ticket's
  "helper pi-tui does not export" carve-out for `text-width.ts` applies to
  anything beyond what pi-tui ships (there is currently nothing else
  `conversation-view.ts`/`agent-widget.ts` need that pi-tui lacks); the
  straightforward implementation is `text-width.ts` re-exporting pi-tui's
  `visibleWidth` rather than reimplementing it.
- `node_modules/.../pi-tui/dist/tui.d.ts#L144-L175` — `export interface TUI
  extends Component { ... }`: **`TUI` is a type-only export, not a class.**
  A literal `tui instanceof TUI` (as the ticket phrases the live check) does
  not compile — `instanceof` requires a value. Confirmed by
  `node_modules/@earendil-works/pi-coding-agent/dist/modes/interactive/interactive-mode.js:10,185,202`:
  Pi imports `TuiAltScreen`/`TuiMainScreen` (both concrete, value-exported
  classes) from `@earendil-works/pi-tui` and constructs `new TuiAltScreen(...)`
  (fullscreen) or `new TuiMainScreen(...)` (regular) as `this.ui`/`this.renderer`
  — the same object type-checked elsewhere in that file via
  `... instanceof TuiMainScreen` / `... instanceof TuiAltScreen` (lines 605,
  623, 1531, 3967, 5132, 5145). **Risk signal — ticket-text/runtime-contract
  mismatch**: the owner-run identity check should be written as
  `tui instanceof TuiMainScreen || tui instanceof TuiAltScreen` using those two
  classes re-exported from `pi-tui.ts`, not `tui instanceof TUI`. This does
  not block Phase 1 coding (the fix is a one-line correction discovered here,
  not a strategy question) but the plan step below encodes it explicitly so
  the owner-run check is written correctly the first time.
- `node_modules/.../pi-tui/dist/components/editor.d.ts#L1-L36` — `Editor`'s
  constructor is `constructor(tui: TUI, theme: EditorTheme, options?:
  EditorOptions)`, i.e. it needs the **full** `TUI` interface (20+ members:
  `addChild`, `setFocus`, `showOverlay`, `requestRender`,
  `addInputListener`, terminal color queries, etc.), not just
  `requestRender()`. `ScrollView`/`Markdown`/`Text` take no `tui` reference at
  all (`ScrollView(component, options)`, `Markdown(text, paddingX, paddingY,
  theme, ...)`, `Text(text, paddingX, paddingY, bgFn?)`) and construct/render
  fine under `node --test` with the real classes directly. **This is the
  "primitive that does not construct with the minimal fake" the ticket
  anticipates** — `Editor` is the one primitive routed through the injectable
  `primitives` factory; `ScrollView`/`Markdown`/`Text` can default straight to
  the real pi-tui classes in tests with no injection needed.
- `agents-plugin-pi/src/tool-result-render.ts#L637-L649` — `loadToolResultTuiModules`
  is the guarded dynamic importer for tool-result rendering; no test in
  `test/tool-result-render.test.ts` or `test/native-tool-registration.test.ts`
  exercises its catch/undefined branch directly (only `test/tool-result-render.test.ts`'s
  own `requireFromPi.resolve(...)` workaround tests, which are unrelated
  integration tests against the real Pi package and are **not** "unavailable"
  cases — leave them as-is). Nothing to delete on the test side for this
  importer; only the source-side dynamic-import/catch is removed in favor of
  a static import from `pi-tui.ts`.
- `agents-plugin-pi/src/push-render.ts#L108-L122` and
  `agents-plugin-pi/test/push-render.test.ts#L201-L209` — `loadPushTuiModules`'s
  dynamic import/catch, and the two tests exercising unavailability
  (`"with pi-tui unavailable nothing is registered..."` and
  `"loadPushTuiModules resolves to undefined under node --test..."`), are the
  concrete deletion targets named by the ticket.
- `agents-plugin-pi/src/index.ts#L372-L392` — the `session_start` wiring around
  `registerPushMessageRenderers`/`loadToolResultTuiModules`, with comments
  framing both as "no-op when pi-tui cannot be loaded." Once both underlying
  loaders always resolve (static import), this framing is stale; the retry
  guard against `pushRenderersRegistered` flipping `false` on a genuine
  runtime rejection (e.g. `assertActive()` during teardown) is a **different,
  still-legitimate** failure mode and must not be deleted — only the
  import-unavailability framing/comments change.
- `agents-plugin-pi/src/overlay-chat.ts#L1-L263` — the exact local helpers the
  ticket names for removal from the *new component* (never touched in this
  file itself): `visibleWidth` (L216), `wrapLine` (L245), `stripAnsi` (L204),
  `isEscapeKey` (L148, kitty-safe Esc detection — pi-tui exports
  `decodeKittyPrintable`/`parseKey`/`isKittyProtocolActive`/`Key`/`matchesKey`
  as its own key-handling primitives; confirm during implementation whether
  `conversation-view.ts` reuses `isEscapeKey`'s existing well-tested regex
  logic as-is (simplest, ticket explicitly says Esc is "detected with the
  existing kitty-protocol-safe `isEscapeKey` logic") rather than reimplementing
  against pi-tui's lower-level key primitives — the ticket's own wording
  ("the existing ... logic") supports copying/reusing this function directly
  into `conversation-view.ts` or a shared module, not rebuilding it from
  pi-tui parts).
- `agents-plugin-pi/src/agent-widget.ts#L38` — the only call site to repoint:
  `import { visibleWidth } from "./overlay-chat.ts";` → `from "./text-width.ts"`.
  `test/agent-widget.test.ts` has no direct `overlay-chat` import to update.
- `agents-plugin-pi/src/ask.ts#L80,#L971-L1000,#L1283-L1293` — `createForkChannel`
  (`ForkChannel`/`isStreaming`) and `openOverlayChat` call site: confirmed
  present and unmodified in this phase (Phase 2 territory); read only to
  confirm Phase 1 introduces `ConversationChannel`/`ChildLiveness` as new,
  independent types in `conversation-view.ts` with no wiring into `ask.ts` yet.
- `agents-plugin-pi/src/tool-result-render.ts#L221-L246` — `logicalPreview`,
  `completedTextPreview`, `yamlInputPreview`, `yamlContainerDisplay` are
  already pure, already exported, and directly reusable for the tool-call/
  tool-result head and expanded-body text the component needs; no new preview
  logic should be written in `conversation-view.ts`.
- No existing "injectable primitives factory for testability" pattern exists
  elsewhere in `agents-plugin-pi/src` (checked `spawner.ts`, `fork.ts`,
  `agent-widget.ts`) — this is a new pattern introduced by this ticket, not a
  reuse of an established one; the executor should keep it minimal (a plain
  options field defaulting to the real constructors) rather than inventing a
  larger DI framework.
- `ai-docs/mental-model/plugin-runtime.md` — describes the Codex/Claude plugin
  packaging contract (`agents-plugin/`, launcher, `runtime.json` compatibility
  gates). It governs `agents-plugin/` packaging, not `agents-plugin-pi/`'s own
  `package.json`/npm dependency tree; no plugin-runtime coupling rule in it
  applies to adding an npm dependency inside `agents-plugin-pi/`. Consulted per
  the ticket's `related-mental-model: plugin-runtime` front-matter; no
  actionable constraint found for this phase beyond general npm-dependency
  hygiene already covered above.

## Implementation Plan

1. `agents-plugin-pi/package.json` — add `"@earendil-works/pi-tui": "0.84.4"`
   to `dependencies` (exact pin, alongside the existing exact `yaml: 2.9.0`
   precedent). Run `npm install`, then `npm ls @earendil-works/pi-tui` and
   confirm exactly one entry (top-level, deduped against `pi-coding-agent`'s
   `^0.84.4` range). If two copies are reported, stop and align the pins
   rather than proceeding — do not ship two copies.
2. Add `agents-plugin-pi/src/pi-tui.ts`: one static
   `import { Box, Editor, Markdown, ScrollView, Text, TuiMainScreen,
   TuiAltScreen, stripTerminalSequences, truncateToWidth, visibleWidth,
   wrapTextWithAnsi, type Component, type TUI, ... } from "@earendil-works/pi-tui";`
   re-exporting exactly what `conversation-view.ts`, `push-render.ts`, and
   `tool-result-render.ts` need (superset check against their current
   `PushTuiModules`/`ToolResultTuiModules` structural interfaces). Re-export
   `TuiMainScreen`/`TuiAltScreen` alongside `TUI` specifically so the owner-run
   identity check (step 7) has a value to `instanceof`-check against.
3. `agents-plugin-pi/src/push-render.ts` — replace `loadPushTuiModules`'s
   dynamic `import()` + try/catch with a direct static import from
   `./pi-tui.ts`; drop the `undefined`-return/unavailable branch and the
   now-dead shape check. `registerPushMessageRenderers` no longer needs to be
   conditionally async-degradable; keep its signature only if
   `index.ts`'s call site needs no other change, otherwise simplify per
   "surgical changes."
4. `agents-plugin-pi/src/tool-result-render.ts` — replace
   `loadToolResultTuiModules`'s dynamic `import()` + try/catch (L637-L649)
   with a static import from `./pi-tui.ts`; drop the shape-check/undefined
   path. Leave `ToolPreviewTuiRef`/`createToolPreviewTuiRef` and the
   `UseNativeResultFallback` throw-when-`tuiRef.current` — those remain valid
   for the ref's late-fill ordering and per-tool override semantics, unrelated
   to package resolvability.
5. `agents-plugin-pi/src/index.ts#L372-L392` — repoint the `session_start`
   wiring at the updated `push-render.ts`/`tool-result-render.ts` APIs; update
   the "pi-tui cannot be loaded" comments to reflect that resolution is now
   guaranteed, while preserving the `pushRenderersRegistered` retry-on-rejection
   guard for genuine runtime teardown races (do not delete that guard).
6. `agents-plugin-pi/src/text-width.ts` — new file; re-export `visibleWidth`
   from `./pi-tui.ts` (pi-tui already exports it — no reimplementation).
   Repoint `agents-plugin-pi/src/agent-widget.ts#L38` from
   `import { visibleWidth } from "./overlay-chat.ts"` to
   `import { visibleWidth } from "./text-width.ts"`. Leave `overlay-chat.ts`'s
   own local `visibleWidth` definition in place (file stays running untouched).
7. Owner-run only (not agent-cleared): inside a live lead session, confirm
   `tui instanceof TuiMainScreen || tui instanceof TuiAltScreen` (using the
   classes re-exported from `pi-tui.ts`, per the Codebase Findings correction
   of the ticket's literal `instanceof TUI` wording) for the `tui` a
   widget/overlay factory receives. Record the result (pass, or "loader
   resolved the adapter tree's copy" failure) in the phase's `## Result` /
   handoff notes; do not gate automated tests on this.
8. `agents-plugin-pi/src/conversation-view.ts` — new file. Define
   `ConversationItem`, `ConversationChannel`, `ChildLiveness`, `mode`, and
   `ConversationViewComponent` per the ticket's Decisions section, importing
   `ScrollView`/`Markdown`/`Text`/`Editor` from `./pi-tui.ts` through an
   injectable `primitives` option (default: the real classes; per Codebase
   Findings, `Editor` is the one primitive the test tier must inject a fake
   for — `ScrollView`/`Markdown`/`Text` can use the real classes directly in
   tests). Reuse `tool-result-render.ts`'s `yamlInputPreview`/
   `completedTextPreview`/`logicalPreview`/`yamlContainerDisplay` for
   tool-call/tool-result head and body text; reuse (copy or share) the
   existing `isEscapeKey` kitty-safe matcher for Esc detection. Implement the
   full key-precedence table (Tab/Shift+Tab/Space/Ctrl+O/Esc/Ctrl+C/Enter) and
   the `setMode("interactive")` raise-only transition exactly as specified.
9. `agents-plugin-pi/test/conversation-view.test.ts` — new file, pure
   `render(width)` tests at 40/80/120 columns per the ticket's Phase 1 test
   list (every item kind renders and stays width-bounded; collapse/expand
   behavior and key precedence; header hint once; `lead-message` label;
   `working…` marker keyed on `liveness()`; `idle-awaiting-owner` vs `settled`
   prominence; view/interactive mode differences; `/done`/Esc/Enter routing;
   `setMode` state preservation; `\x03` swallowed; `agent_start` triggers a
   render), using a fake `ConversationChannel` and a fake `tui` (`requestRender`
   only) with the injected `Editor` fake from step 8.
10. `agents-plugin-pi/test/push-render.test.ts` — delete the two
    "unavailable"/`loadPushTuiModules`-degrades tests (current L201-L209);
    keep the rest of the suite exercising `buildPushRenderLines`/
    `buildPushComponent`/`registerPushMessageRenderers` with the injected fake
    `PushTuiModules` object (that pattern is unaffected — it never depended on
    the dynamic import itself).
11. Sanity-check `test/tool-result-render.test.ts`,
    `test/native-tool-registration.test.ts`: no "unavailable" case exists
    there today for `loadToolResultTuiModules`, so no test deletion is
    expected on that side — confirm this holds after the source change in
    step 4 (i.e., nothing new breaks by removing the dynamic-import branch).

## Verification Plan

- `cd agents-plugin-pi && npm ls @earendil-works/pi-tui` — exactly one entry.
- `cd agents-plugin-pi && npm test` (i.e. `node --test`) green, specifically:
  - `test/conversation-view.test.ts` (new) covers the full Phase 1 test list
    at 40/80/120 columns with no TTY.
  - `test/push-render.test.ts` green with the two unavailable-case tests
    removed and the rest intact.
  - `test/tool-result-render.test.ts` green unmodified (the `260906` rendering
    tests the ticket requires to "stay green").
  - `test/agent-widget.test.ts` green after the `visibleWidth` import
    repoint (behavior-preserving: pi-tui's `visibleWidth` vs. the local
    `overlay-chat.ts` copy — verify no width-classification regression for
    any fixture string agent-widget tests already exercise).
  - `test/overlay-chat.test.ts` green and unmodified (file untouched; its
    `loadMarkdownRenderer` "unavailable" case is explicitly out of scope).
- Manual/owner-run (not part of `npm test`): the live `tui instanceof
  TuiMainScreen || tui instanceof TuiAltScreen` identity check from
  Implementation step 7, run inside a live lead session, with the pass/fail
  outcome recorded in the phase handoff — never treated as agent-cleared.

## Escalations

- None.

## Addendum — dual-package resolution (lead decision, 2026-09-09)

**Blocker found at step 1:** `@earendil-works/pi-coding-agent` ships an
`npm-shrinkwrap.json` that pins its own nested `@earendil-works/pi-tui@0.84.4`.
npm treats a shrinkwrapped nested subtree as authoritative and will not
hoist/dedupe it, so adding a top-level `pi-tui` dependency yields **two physical
copies** (confirmed: fresh install, `npm dedupe`, and `overrides` all leave it
nested; both copies are byte-identical `0.84.4`). The two are different physical
files, so classes re-exported from a top-level *static* import are not the same
class objects the host's `tui` is built from — `npm ls … == one copy` is
physically unattainable by pin alignment.

**Decision (governed by the ticket's sage-settled "Runtime instance" clause, so
pre-authorized — not a contract change):** adopt the ticket's explicitly named
"only acceptable fallback." `pi-tui.ts` is the single resolution point that
**resolves pi-tui through the host at runtime** (the existing guarded dynamic
`import("@earendil-works/pi-tui")` shim shape that `push-render.ts` /
`tool-result-render.ts` / `overlay-chat.ts`'s `loadMarkdownRenderer` already use
to reach the host's modules) while exposing a **static import path for
tests/types**. Runtime therefore uses the host's single instance — "not a
duplicated instance" is satisfied at the level that matters (the instance in
use), even though two copies sit on disk. Option (b) (accept two copies, amend
the "never two copies" gate) is rejected: it reverses a sage-settled ticket
decision.

**Consequences for the steps below:**
- Step 1 gate: record the two-on-disk topology + this rationale; do NOT treat
  two-copies-on-disk as a ship blocker, because runtime routes through the host.
- Step 2 `pi-tui.ts`: not a bare "one static import." It keeps the host-runtime
  dynamic resolution as the runtime path and a static import as the
  test/types/default path, and re-exports the value classes
  (`TuiMainScreen`/`TuiAltScreen`/`ScrollView`/`Markdown`/`Text`/`Editor`/`Box`
  + helpers) and types from that single point.
- Steps 3–5 (importer migration): the three importers consume `pi-tui.ts`'s
  single resolution point instead of each carrying its own dynamic-import +
  unavailable branch — the per-importer duplication collapses into `pi-tui.ts`,
  which is the ticket's "one resolution point" intent. Keep whatever guard the
  offline `node --test` path needs (the static path covers tests); the live
  host always has pi-tui.
- Step 8 (component): the component takes its primitives via the mandated
  injectable `primitives` factory, so it never needs a synchronous static class
  import — the default is `pi-tui.ts`'s static classes (fine for tests and as a
  default), and live wiring (Phase 2 / child B) can feed host-resolved classes
  when the owner-run identity check requires it. This is the seam that makes the
  dual-package reconciliation invisible to the component's own code.
- Step 7 (owner-run identity check): unchanged and still owner-run; with the
  host-runtime resolution it is now expected to pass.

If, while implementing, the host-runtime resolution proves mechanically
impossible for a specific consumer (a hard technical obstacle, not a
preference), stop and report that specific obstacle rather than falling back to
option (b).
