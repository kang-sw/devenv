# Plan: 260722-refactor-dashboard-app-tsx-leaf-extraction — Phase 2: Pane-body extraction

## Relevant Ticket Contract

- Move the three leaf pane renderers out of `App.tsx`: `TerminalPaneBody`
  (~654 lines), `AgentChatPaneBody` (~482), `ReadOnlyDocumentPane` (~370).
  They consume `TerminalPrefsContext` but are otherwise structurally
  extractable — keep context consumption, just relocate the component.
- Additionally land the deferred Phase-1 step-7 payload: the deferred
  functions entangle with the pane bodies, so they roll into this phase —
  `buildWorkbenchEditorGroups`, the three `*WorkbenchPane` single-constructors
  (`agentChatWorkbenchPane`, `terminalWorkbenchPane`, `readOnlyWorkbenchPane`),
  and `workRootActivityPlacementState`/`workRootActivityWorkbenchPane`.
- Pure moves only: no behavior change, no new caller-visible contract beyond
  module boundaries. Terminal restore/reattach and dockview layout must not
  regress.
- Prefer several small, reviewable extraction commits over one mega-commit.
- Verification (both phases): `npm run build` green + all `test:*` suites
  green + `npm run test:browser` green + manual smoke of terminal
  restore/reattach, the four modals, dockview layout.
- `npm run test:browser` is currently red-lined by two known, unrelated,
  already-tracked issues: `dashboard-acceptance.spec.ts:2714`
  (`260722-bug-e2e-terminal-resize-frame-assertion-fails`) and the locator
  ambiguity fix `260722-bug-e2e-open-work-root-locator-ambiguity`. Neither is
  this phase's job to fix; the bar for this phase is "no NEW browser failures
  beyond those two known red points."

## Out of Scope

- `WorkbenchShell`/`App()` state ownership and the 23-prop drill — that is
  `260722-refactor-dashboard-app-tsx-state-decomposition`.
- Fixing `260722-bug-e2e-open-work-root-locator-ambiguity` or
  `260722-bug-e2e-terminal-resize-frame-assertion-fails` — pre-existing,
  tracked separately; only "no new regression" applies here.
- `activationForAction` (`App.tsx#L6904-6912`) — textually adjacent to the
  moved cluster but functionally unrelated (workRoot activation action
  parsing, used only inside `WorkbenchShell`); leave in place.
- Any other App.tsx cluster not named in Phase 2 (Phase 1 already landed leaf
  helpers/presentational components/modals/placement pure functions).

## Codebase Findings

**Mental-model hazard (critical constraint for this phase)**
`ai-docs/mental-model/ws-web-dashboard.md#L228` — two TypeScript programs:
`tsconfig.app.json` (Bundler) vs `tsconfig.route-tests.json` (NodeNext).
`ws-dashboard/frontend/tsconfig.route-tests.json` includes
`"src/workbench/**/*.ts"` plus an explicit file list (NOT `App.tsx`).
Confirmed: nothing currently included pulls in `App.tsx`, and no file outside
`App.tsx` currently imports back into it (`grep -rln "\.\./App" workbench/`
→ empty). The compiler follows the *import closure* of the include roots,
not just glob-matched files, so any `.ts`/`.tsx` file reachable from
`workbench/index.ts` (a barrel, `workbench/index.ts#L1-15`) becomes part of
the NodeNext program even if it doesn't itself match the glob (already true
today for `workbench/activityPlacement.tsx`, a `.tsx` file pulled in only via
`index.ts`'s `export * from "./activityPlacement.js"`). Consequence for this
phase: once the deferred constructors move into `workbench/`, the pane-body
files they render (`<TerminalPaneBody/>` etc.) also become part of the
NodeNext program transitively — so those pane-body files, and everything
*they* import, must never import back from `../App`.

**Dependency audit — every symbol the three pane bodies + six deferred
constructors touch, confirmed already-leaf or App.tsx-local (verified via
direct reads + repo-wide grep, not inference):**

- `ReadOnlyDocumentPane` (`App.tsx#L8621-8989`, ~369 lines) — **zero**
  App.tsx-local dependencies. Every helper it calls
  (`documentDraftContentChangeDecision`, `buildDocumentModeSetCommand`,
  `buildDocumentSaveCommand`, `buildDocumentRevertCommand`,
  `writeWorkRootTextFile`, `documentSaveStateForError`,
  `buildDocumentTranslationRequestPayload`, `fetchTranslationProviders`,
  `requestDocumentTranslation`, `overlayFromTranslationResponse`,
  `isMarkdownDocumentSource`, `buildDocumentTranslationToggleCommand`,
  `DocumentViewer`, `DocumentRawEditor`) already lives in
  `workRootFiles.ts`, `commands.ts`, or `documentViewer.tsx`/
  `documentRawEditor.tsx`. Cleanest of the three — safe to move first,
  independent of every other step below.
- `TerminalPaneBody` (`App.tsx#L7859-8511`, ~653 lines) plus its
  same-file-only helper `terminalScreenFitsVisibleBox`
  (`App.tsx#L8513-8521`, must move together — not used anywhere else) — all
  called helpers (`resolveTerminalMountWrite`, `buildEffectiveTerminalFontFamily`,
  `clampTerminalSize`, `terminalWebSocketUrl`, `terminalWebSocketCursor`,
  `terminalVisualRestoreScrollbackLines`, `terminalVisualRestoreDebounceMs`)
  already live in `terminals.ts` / `terminalPrefs.ts` /
  `workbench/terminalVisualRestore.ts`. The **only** App.tsx-local dependency
  is `TerminalPrefsContext` (`App.tsx#L516-518`, `useContext` at `App.tsx#L7896`).
- `AgentChatPaneBody` (`App.tsx#L7262-7742`, ~481 lines) — App.tsx-local
  dependencies: `useDismissableMenu` (`App.tsx#L2055-2083`, a generic
  click-outside/Escape hook with **zero** App-state closure, also called from
  `WorkbenchShell` at `App.tsx#L6238,6479,9334` — must relocate to a shared
  neutral module, not duplicate) and `realAgentChatHarness`
  (`App.tsx#L504-508`, a 5-line pure guard, also called from `WorkbenchShell`
  at `App.tsx#L5294,5334,5416,5487` — same "shared, must not duplicate"
  situation). Everything else
  (`mergeStreamingTranscriptBlocks`, `stubBeginStreamingTurn`,
  `beginRealStreamingTurn`, `realSteerActivitySession`,
  `stubSteerActivitySession`, `agentChatHarnessLabel`, `agentChatHarnesses`,
  `AgentChatTranscriptBubbles`) is already a leaf import.
- **Correction to the render-prompt's framing**: only `TerminalPaneBody`
  consumes `TerminalPrefsContext`. `SettingsTerminalContext`
  (`settingsSections.tsx#L23-24`) is used only inside `App()`'s settings
  wiring (`App.tsx#L1919,2050`) — none of the three pane bodies touch it. No
  action needed for `SettingsTerminalContext` this phase.
- `WorkbenchPane` type (`App.tsx#L6914-6924`) and `WorkbenchEditorGroupModel`
  type (`App.tsx#L6926-6930`) and `initialWorkbenchGroups` const
  (`App.tsx#L494-497`) are App.tsx-local and required by every one of the six
  deferred constructors below, **and** still separately required inside
  `WorkbenchShell` itself (`App.tsx#L1217,3815-3816,4041,4278,4293` —
  `buildEditorGroupsForRoot`'s own return-type annotation and layout
  fallbacks). Confirmed via repo-wide `\bWorkbenchPane\b` grep that no file
  outside `App.tsx` currently imports the concrete type (only unrelated
  substring matches `WorkbenchPaneOrder`/`WorkbenchPaneCategory` in
  `workbench/editorGroupModel.ts`, and two comments). Must move to a new
  neutral `workbench/` module that both `App.tsx` (forward import) and the
  relocated constructors (forward import) consume — this is exactly what the
  existing in-file comment at `App.tsx#L6937-6942` already anticipates
  ("...they move to workbench/editorGroups.tsx...").
- `TerminalPaneActions` type (`App.tsx#L7780-7820`) and `AgentChatPaneActions`
  type (`App.tsx#L7176-7203`) are referenced only within the cluster itself
  (constructor signature + pane-body props) — `WorkbenchShell` builds the
  matching action objects as inline literals (`App.tsx#L4313-4335` for
  terminal actions) without an explicit type annotation, so no import update
  is needed there; these two types can move wholesale with their respective
  constructor/pane-body pair.
- `PendingChatMessage` type (`App.tsx#L7256-7260`) is used only inside
  `AgentChatPaneBody` — moves with it, no export needed.
- **Non-obvious coupling the ticket text doesn't name explicitly**: the two
  `*WorkbenchPanesByGroup` helpers (`agentChatWorkbenchPanesByGroup`
  `App.tsx#L7141-7174`, `terminalWorkbenchPanesByGroup`
  `App.tsx#L7744-7778`, `readOnlyWorkbenchPanesByGroup`
  `App.tsx#L8523-8570`) sit between `buildWorkbenchEditorGroups` and the
  three single-pane constructors — `buildWorkbenchEditorGroups` calls all
  three `*ByGroup` helpers directly (`App.tsx#L7055-7076`), and each
  `*ByGroup` helper calls its matching single-constructor
  (`App.tsx#L7154,7758,8545`). These `*ByGroup` helpers are **not** listed by
  name in the ticket's Phase 2 payload description but **must** move
  alongside their constructors — leaving them resident in `App.tsx` while
  `buildWorkbenchEditorGroups` moves to `workbench/` would recreate exactly
  the back-import hazard this phase exists to avoid.
- `workbench/activityPlacement.tsx#L1-70` already holds the rest of the
  WorkRoot-Activity pane cluster (`WorkRootActivityPane`,
  `workRootActivityPaneId`, `workRootActivityPaneLogicalKey`,
  `workRootActivityPaneRevision`); the in-file comment at `App.tsx#L6937-6942`
  states `workRootActivityPlacementState`/`workRootActivityWorkbenchPane`'s
  only reason for staying in `App.tsx` was the (soon-resolved)
  `WorkbenchEditorGroupModel`/`initialWorkbenchGroups` dependency — their
  natural destination is this same file.
- No `.test.tsx` file exists anywhere in this codebase (`find . -iname
  "*.test.tsx"` → empty); component-level render tests are not this
  project's established pattern. Phase 1's result accepted "no paired
  `.test.ts` for workbench modules with zero prior direct coverage" as a
  non-blocking deferred minor. Consistent with that precedent, this phase
  adds no new test files for the moved components — only relocates existing,
  already-tested pure helpers where they land in leaf modules that already
  have paired tests (none of the six relocated pure helpers currently have
  direct unit coverage of their own; they are exercised only via
  `App.tsx`/browser-level behavior today, and that stays true after the move).

## Implementation Plan

Sequence is dependency-ordered; each commit leaves `npm run build` and all
`test:*` suites green.

**Phase A — relocate shared neutral dependencies (no pane-body/constructor
code moves yet; each independently buildable):**

1. Move `TerminalPrefsContext` (`App.tsx#L516-518`) into `terminalPrefs.ts`
   (add `createContext` import there). Update `App.tsx` to import it instead
   of defining it locally; the `<TerminalPrefsContext.Provider>` usage
   (`App.tsx#L1918-2051`) is otherwise unchanged.
2. Move `useDismissableMenu` (`App.tsx#L2055-2083`) into a new
   `src/dismissableMenu.ts` (generic hook, only needs `useEffect`/`RefObject`
   from `react`). Update `App.tsx`'s three in-place call sites
   (`App.tsx#L6238,6479,9334`) to import from the new module.
3. Move `realAgentChatHarness` (`App.tsx#L504-508`) into
   `activitySessionClient.ts` (already exports `RealAgentChatHarness`; add a
   `type AgentChatHarness` import from `./agentChatSessions.js` and export the
   function). Update `App.tsx`'s four in-place call sites
   (`App.tsx#L5294,5334,5416,5487`) to import from there.

**Phase B — relocate the three pane bodies (each depends only on the
matching Phase A piece; constructors stay resident in `App.tsx` for now and
just forward-import the new sibling module — still buildable):**

4. Move `ReadOnlyDocumentPane` (`App.tsx#L8621-8989`) into new
   `src/readOnlyDocumentPane.tsx`. `readOnlyWorkbenchPane` (still in
   `App.tsx`) updates its JSX reference to import forward.
5. Move `TerminalPaneBody` + `terminalScreenFitsVisibleBox` +
   `TerminalPaneActions` type (`App.tsx#L7780-7820,7859-8521`) into new
   `src/terminalPaneBody.tsx`. Needs commit 1. `terminalWorkbenchPane` (still
   in `App.tsx`) forward-imports it.
6. Move `AgentChatPaneBody` + `PendingChatMessage` type +
   `AgentChatPaneActions` type (`App.tsx#L7176-7203,7256-7742`) into new
   `src/agentChatPaneBody.tsx`. Needs commits 2 and 3. `agentChatWorkbenchPane`
   (still in `App.tsx`) forward-imports it.

**Phase C — neutral type/const home for the constructors:**

7. Create `workbench/editorGroups.ts` with `WorkbenchPane`,
   `WorkbenchEditorGroupModel` (`App.tsx#L6914-6930`) and
   `initialWorkbenchGroups` (`App.tsx#L494-497`). Add
   `export * from "./editorGroups.js"` to `workbench/index.ts`. Update
   `App.tsx`'s existing `"./workbench"` import block
   (`App.tsx#L140-196`) to pull these three from there instead of defining
   them locally; update all read sites
   (`App.tsx#L1217,3815-3816,4041,4278,4293`).

**Phase D — relocate the deferred constructors into `workbench/` (each needs
commit 7 plus its Phase B pane body; independently buildable in any order
relative to each other):**

8. Move `readOnlyWorkbenchPane` + `readOnlyWorkbenchPanesByGroup`
   (`App.tsx#L8523-8619`) into new `workbench/readOnlyWorkbenchPane.tsx`,
   importing `ReadOnlyDocumentPane` from `../readOnlyDocumentPane.js`. Add to
   `workbench/index.ts`. Update `App.tsx`'s `"./workbench"` import block.
9. Move `terminalWorkbenchPane` + `terminalWorkbenchPanesByGroup`
   (`App.tsx#L7744-7847`) into new `workbench/terminalWorkbenchPane.tsx`,
   importing `TerminalPaneBody` from `../terminalPaneBody.js`. Add to
   `workbench/index.ts`. Update `App.tsx`'s import block.
10. Move `agentChatWorkbenchPane` + `agentChatWorkbenchPanesByGroup`
    (`App.tsx#L7141-7249`) into new `workbench/agentChatWorkbenchPane.tsx`,
    importing `AgentChatPaneBody` from `../agentChatPaneBody.js`. Add to
    `workbench/index.ts`. Update `App.tsx`'s import block.
11. Move `workRootActivityPlacementState` + `workRootActivityWorkbenchPane`
    (`App.tsx#L6943-7021`) into existing `workbench/activityPlacement.tsx`
    (joins `WorkRootActivityPane`/`workRootActivityPaneId`/etc. already
    there), importing `WorkbenchEditorGroupModel`/`WorkbenchPane` from
    `./editorGroups.js`. Update `App.tsx`'s import block.

**Phase E — final orchestrator (needs 8, 9, 10, 11):**

12. Move `buildWorkbenchEditorGroups` (`App.tsx#L7023-7139`) into
    `workbench/editorGroups.ts` (alongside the types/const from commit 7),
    importing the three `*WorkbenchPanesByGroup` helpers and
    `workRootActivityWorkbenchPane` forward from their new homes. This is the
    last symbol left in `App.tsx` from the deferred step-7 payload; after
    this commit `App.tsx`'s `"./workbench"` import block is the sole call
    site (`App.tsx#L4302`, inside `buildEditorGroupsForRoot`).

Commits 1–3 may be squashed into one "neutral dependency relocation" commit
if the executor judges the reduced commit count outweighs per-symbol
reviewability — each is independent of the others either way. Commits 8–10
may be reordered relative to each other freely (no cross-dependency between
them beyond commit 7 and their respective Phase B pane body).

## Verification Plan

- After every commit: `npm run build` (`tsc -b && vite build`) must stay
  green — this is the Bundler program and will NOT surface NodeNext-only
  back-import diagnostics, so it is necessary but not sufficient.
- After every commit: run all `test:*` npm scripts (they each invoke
  `tsc -p tsconfig.route-tests.json` first) — this is the program that
  actually catches a `workbench/*` → `../App` back-import regression, so
  treat it as the load-bearing gate, not `npm run build`.
- After the full sequence (and ideally after Phase D/E commits specifically,
  since those are the ones that pull the pane bodies into the NodeNext
  program): `npm run test:browser`. Expect the two pre-existing red points
  (`dashboard-acceptance.spec.ts:2714` and the `open-work-root` locator
  ambiguity) and otherwise zero new failures. Do not attempt to fix either
  pre-existing failure as part of this phase.
- Manual smoke (per ticket's Phase 2 verification clause, since automated
  browser-gate green is blocked by the two known issues above): open a work
  root, open a terminal, disconnect/reconnect or switch tabs to confirm
  restore/reattach still renders scrollback correctly; open an agent chat
  pane and send a message; open a read-only file pane in both view and edit
  mode; open each of the four modals (Git worktree add/remove, Settings,
  Linked server) to confirm they still render; drag a pane between dockview
  groups to confirm layout/placement is unchanged.
- After commit 7 and again after commit 12, specifically re-run `grep -rln
  "\.\./App" ws-dashboard/frontend/src/workbench/` (and the same grep against
  `src/terminalPaneBody.tsx`, `src/agentChatPaneBody.tsx`,
  `src/readOnlyDocumentPane.tsx`) to confirm it stays empty — this is the
  concrete, mechanical check for the mental-model hazard this phase exists to
  avoid.

## Escalations

- None.
