# Plan: 260722-refactor-dashboard-app-tsx-leaf-extraction — Phase 1: Mechanical leaf extraction

## Relevant Ticket Contract

- Pure moves only: no behavior change, no new caller-visible contract beyond
  module boundaries. Terminal restore/reattach and Dockview layout must not
  regress.
- Do NOT touch `WorkbenchShell`/`App()` state ownership or the 23-prop drill —
  deferred to follow-on ticket `260722-refactor-dashboard-app-tsx-state-decomposition`.
- Prefer several small, reviewable extraction commits over one mega-commit.
- Enumerated Phase 1 targets (verbatim from ticket): (a) tail pure-format
  helpers (DetailItem, StateLine/Badge/Dot, normalizeServerRoute, kindLabel,
  resourceRowTone, etc.); (b) presentational components (ChromeIconButton,
  ResourceGlyph, WorkRootKindIcon, ToggleIcon, GitStatusPill,
  WorkbenchActivityBadge, InlineNotice); (c) the four already-prop-driven
  modals (GitWorktreeAddModal, GitWorktreeRemoveModal, SettingsModal,
  LinkedServerModal), co-located with `gitWorktreeAdd.ts`/`gitWorktreeRemove.ts`/
  `gitToolbar.ts`; (d) pane-placement pure fns (buildWorkbenchEditorGroups,
  placeAgentChatPane*, placeTerminalSessions*, readOnlyFile* helpers) hoisted
  into `workbench/` alongside `policy.ts`/`editorGroupModel.ts`.
- Verification boundary (both phases, per ticket): `npm run build` green + all
  `test:*` suites green + `npm run test:browser` green + manual smoke of
  terminal restore/reattach, the four modals, Dockview layout. The ticket text
  cites `260722-bug-e2e-open-work-root-locator-ambiguity` as the browser-gate
  blocker, but that ticket has since moved to `ready/` and its Phase 1 fix
  (commit `2bc160d4`) already landed — it unmasked a *further* pre-existing
  failure now tracked separately (see Verification Plan).

## Out of Scope

- `App()` (`App.tsx:521-2056`) and `WorkbenchShell` (`App.tsx:4582-7144`) —
  state ownership / 23-prop drill is the follow-on ticket's job. Nothing in
  this plan edits their bodies except call-site import swaps (function calls
  that already just invoke the now-imported helpers).
- Phase 2 pane-body renderers: `TerminalPaneBody` (`App.tsx:9213-9866`),
  `AgentChatPaneBody` (`App.tsx:8538-9019`), `ReadOnlyDocumentPane`
  (`App.tsx:10070-10439`). These stay in `App.tsx` this phase. They are
  directly JSX-referenced by three of the pane-placement functions this phase
  *does* move — see the circular-import finding below; that finding is about
  import direction, not about moving the pane bodies themselves.
- `terminalScreenFitsVisibleBox` (`App.tsx:9867-9875`) — textually sits inside
  the pane-placement cluster's line range but is used only by
  `TerminalPaneBody` (`App.tsx:9473`). It is a Phase 2 concern; do not move it
  with the placement helpers.
- Non-enumerated top-level components that are also structurally
  low-entanglement but are not named by the ticket's Phase 1 bullets:
  `OpenWorkRootControl` (2185-2904), `ResourceNavigation` (3792-3948),
  `ServerRows`/`ServerActionButton` (3948-4179), `WorkRootFileExplorer`/
  `FileExplorerRow` (4179-4557), `WorkRootGitToolbar` (7435-7865),
  `WorkbenchToolbar` (7192-7435), `WorkbenchClosePopover` (7144-7192),
  `SubInstancePane`/`ResourceSummary` (10440-10480), `WorkspaceRows`
  (10480-10668), `ResourceRow` (10697-11079), `ResourceDetail` (11079-11166),
  `ViewerReserve`/`StatusPane`/`EmptyWorkbenchPlaceholder` (11166-11259).
  Leave these in `App.tsx`; do not scope-creep into them.
- Full `npm run test:browser` green. `dashboard-acceptance.spec.ts:2714`
  (the `"create terminal and run a command"` step's resize-frame WebSocket
  assertion) is currently red, tracked by
  `260722-bug-e2e-terminal-resize-frame-assertion-fails` (idea/), confirmed
  pre-existing and unrelated to this refactor. Reaching that point clean with
  no new failures beyond it is the achievable bar this phase owns; fixing
  2714 itself is not this phase's responsibility.

## Codebase Findings

- `App.tsx:11259-11382` — tail pure-format helper cluster: `DetailItem`,
  `StateLine`, `StateBadge`, `StateDot`, `normalizeServerRoute`,
  `resourceEntityForWorkRoot`, `instanceSummary`, `closeContractLabel`,
  `resourcePresentationLabel`, `resourceRowTone`, `kindLabel`. All are
  top-level function declarations (not nested in `App()`/`WorkbenchShell`),
  take only params, call no hooks, and reference no module-mutable state.
  `closeContractLabel` calls `decideSurfaceClose` (already imported from
  `./workbench`); `normalizeServerRoute` calls `normalizeServerRouteLocation`
  (already imported from `./routeBasis`). Confirmed low-entanglement.
- `App.tsx:2086-2185` and `App.tsx:4565-4582` — presentational leaf
  components: `ChromeIconButton` (2086), `ResourceGlyph` (2118),
  `WorkRootKindIcon` (2142), `ToggleIcon` (2180), `InlineNotice` (4565). Pure
  props-in/JSX-out, no hooks, no closure capture. `ToggleIcon` calls
  `workbenchToggleIcon` (`App.tsx:8203-8216`), a pure switch not itself named
  by the ticket bullet but a direct, trivial, low-entanglement dependency —
  move it alongside `ToggleIcon`.
- `App.tsx:7865-8005` — `GitStatusPill` (7865, uses already-extracted
  `gitChangeStatusSegments`/`gitSyncStatusSegments`/`gitStatusSegments` from
  `./gitToolbar`) and `WorkbenchActivityBadge` (7969, uses
  `WorkRootActivityBadgeView` type from already-extracted `./workRootActivity`).
  Both pure props-driven, no hooks. Confirmed low-entanglement.
- `App.tsx:2904-3792` — the four modals: `GitWorktreeAddModal` (2904-3298),
  `GitWorktreeRemoveModal` (3298-3507), `SettingsModal` (3507-3595),
  `LinkedServerModal` (3595-3792). Each owns local `useState`/`useEffect`/
  `useCallback` but derives everything from props (`target`, `onCommand`,
  `onClose`, `onCreated`/`onRemoved`, `state`, `onLinked`, `open`, `sections`)
  — no closure capture of `App()`/`WorkbenchShell` locals. Confirmed
  low-entanglement. **Dependency ordering constraint**: all four call
  `ChromeIconButton` (2586/2808/3119.../3554/3704) and three call
  `InlineNotice` (GitWorktreeAddModal, GitWorktreeRemoveModal,
  LinkedServerModal) — the presentational-components commit must land before
  the modal commits.
- `App.tsx:8007-8163` — WorkRoot Activity pane-placement cluster:
  `workRootActivityPaneLogicalKey`, `workRootActivityPaneId`,
  `workRootActivityPlacementState`, `workRootActivityWorkbenchPane`,
  `WorkRootActivityPane` (its own self-contained component, using the
  already-extracted `ActivityConsole`), `workRootActivityPaneRevision`, plus
  the local `ActivityTranscriptRefreshSignal` type (8096-8102). Not literally
  named by the ticket's bullet text, but it is a **hard, direct dependency**
  of `buildWorkbenchEditorGroups` (called at 8321) which *is* named. Unlike
  the terminal/agentChat/readOnlyFile families, this cluster's pane-body
  (`WorkRootActivityPane`) is defined inside the cluster itself, not left
  behind in `App.tsx` — so moving it carries **zero circular-import risk**.
  Recommend moving it together with the other pane-placement pure fns in this
  phase (it is the same "pane-placement pure function" shape the ticket bullet
  describes, just not spelled out by name).
- `App.tsx:8228-8245`, `App.tsx:495-498` — `WorkbenchPane`/
  `WorkbenchEditorGroupModel` types and the `initialWorkbenchGroups` const are
  shared by both the pane-placement cluster (8024, 8277 fallback usage) and
  `App()`/`WorkbenchShell` themselves (`initialWorkbenchGroups` used directly
  at `App.tsx:1218, 4792-4793, 5018, 5261, 5270`; `WorkbenchEditorGroupModel`
  used directly at `App.tsx:5255`). Moving the placement cluster means these
  need to move too, with `App.tsx` importing them back — a normal one-way
  import, not a cycle by itself.
- **Circular-import risk signal** — `App.tsx:8417-8525` (`agentChatWorkbenchPanesByGroup`,
  `AgentChatPaneActions` type, `agentChatWorkbenchPane`), `App.tsx:9098-9201`
  (`terminalWorkbenchPanesByGroup`, `TerminalPaneActions` type,
  `terminalWorkbenchPane`), and `App.tsx:9972-10068`
  (`readOnlyWorkbenchPanesByGroup`, `readOnlyWorkbenchPane`) each construct a
  `WorkbenchPane.body` by directly JSX-referencing a pane-body component that
  stays in `App.tsx` this phase: `agentChatWorkbenchPane` embeds
  `<AgentChatPaneBody>` (8523), `terminalWorkbenchPane` embeds
  `<TerminalPaneBody>` (9199), `readOnlyWorkbenchPane` embeds
  `<ReadOnlyDocumentPane>` (10059). `buildWorkbenchEditorGroups`
  (`App.tsx:8246-8362`, the ticket's explicitly named target) calls all three
  `*WorkbenchPanesByGroup` functions directly (8278, 8286, 8293). Moving
  `buildWorkbenchEditorGroups` and these three families to `workbench/` this
  phase therefore requires `workbench/*` to import
  `TerminalPaneBody`/`AgentChatPaneBody`/`ReadOnlyDocumentPane` back from
  `../App`, while `App.tsx` imports the moved functions from `./workbench` —
  a genuine module cycle. This is a **mechanical, build-verifiable** risk, not
  a strategic one: all the functions involved are `function` declarations
  (hoisted) and the JSX references execute only at render time, so ES module
  / Vite (esbuild) circular imports of this shape are ordinarily fine — but it
  must be confirmed with `npm run build`, not assumed. See Implementation Plan
  step 7 for the verify-then-fallback sequencing this implies.
- `ws-dashboard/frontend/src/workbench/index.ts` — barrel re-exports every
  sibling module (`export * from "./X.js"`, NodeNext extensions). Any new
  `workbench/*.ts(x)` file must be added here for `App.tsx`'s existing
  `from "./workbench"` import to keep resolving the newly moved symbols.
- `ws-dashboard/frontend/tsconfig.route-tests.json` — explicit `include` list
  of source+test file pairs feeding the `test:*` npm scripts; any new paired
  `.test.ts` file must be added to this list and to the relevant `test:*`
  script in `package.json`.
- `ws-dashboard/frontend/package.json:8-30` — confirms `build` is
  `tsc -b && vite build`; `test:browser` is
  `npm run build && (cd .. && cargo build -p ws-dashboard-daemon) && playwright test`;
  no existing `.tsx` component in this codebase has a paired `.test.ts` (only
  logic-bearing `.ts`/`.tsx` files with real branching do, e.g.
  `documentViewer.tsx`, `agentChatBubbles.tsx`) — precedent supports skipping
  dedicated unit tests for the pure-presentational/modal commits and relying
  on the build type-check plus the browser acceptance gate for those, while
  the pane-placement pure functions (real branching logic) are reasonable
  candidates for paired tests following the `workbench/*.test.ts` pattern
  (executor discretion; not mandatory to stay in scope).

## Implementation Plan

Land as separate commits, each independently `npm run build`-green, in this
order (later commits depend on earlier ones per the dependency findings
above):

1. **Tail pure-format helpers** → new `ws-dashboard/frontend/src/resourcePresentation.tsx`.
   Move `DetailItem`, `StateLine`, `StateBadge`, `StateDot`,
   `normalizeServerRoute`, `resourceEntityForWorkRoot`, `instanceSummary`,
   `closeContractLabel`, `resourcePresentationLabel`, `resourceRowTone`,
   `kindLabel` out of `App.tsx:11259-11382`; import them back into `App.tsx`
   at existing call sites (982, 5276-5277, 7242, 7290, 8309-8314, 8502-8503,
   10458-11150 region).
2. **Presentational chrome components** → new
   `ws-dashboard/frontend/src/chrome.tsx`. Move `ChromeIconButton` (2086),
   `ResourceGlyph` (2118), `WorkRootKindIcon` (2142), `ToggleIcon` (2180) +
   its dependency `workbenchToggleIcon` (8203), and `InlineNotice` (4565).
   Import back into `App.tsx` at all existing JSX call sites.
3. **GitStatusPill + WorkbenchActivityBadge** → new
   `ws-dashboard/frontend/src/workbenchChips.tsx` (or split into two small
   files if preferred — no functional difference). Move `GitStatusPill`
   (7865) and `WorkbenchActivityBadge` (7969).
4. **GitWorktreeAddModal + GitWorktreeRemoveModal** → new
   `ws-dashboard/frontend/src/gitWorktreeAddModal.tsx` and
   `gitWorktreeRemoveModal.tsx`, co-located next to `gitWorktreeAdd.ts`/
   `gitWorktreeRemove.ts`. Requires commit 2 landed first (uses
   `ChromeIconButton`/`InlineNotice`).
5. **SettingsModal + LinkedServerModal** → new `settingsModal.tsx` and
   `linkedServerModal.tsx` (co-located next to `linkedServers.ts` for the
   latter). Requires commit 2 landed first.
6. **Pane-order utilities + per-family placement-state + WorkRoot Activity
   pane cluster** (zero circular-import risk) → new
   `ws-dashboard/frontend/src/workbench/paneOrder.ts` for the generic
   `addPaneToGroupOrder`/`removePaneFromOrder`/`groupIdForPaneOrder`/
   `activityPaneGroupIdFromOrder` (9915-9970); new
   `workbench/agentChatPlacement.ts` for `placeAgentChatPane`/
   `agentChatPlacementState` (8364-8416); new `workbench/terminalPlacement.ts`
   for `placeTerminalSessions`/`terminalPlacementState` (9020-9096); new
   `workbench/readOnlyFilePlacement.ts` for `readOnlyFilePlacementState`/
   `sameReadOnlyOpenRequest`/`readOnlyFilePaneRevision`/`hashText`
   (8164-8183, 9877-9913); new `workbench/activityPlacement.tsx` for the full
   WorkRoot Activity pane cluster (8007-8163, including
   `ActivityTranscriptRefreshSignal`). Register all new files in
   `workbench/index.ts`.
7. **`buildWorkbenchEditorGroups` + per-family `*WorkbenchPanesByGroup`/
   `*WorkbenchPane` single-constructors + shared types** (circular-import
   risk — verify-then-fallback) → extend the `workbench/agentChatPlacement.ts`/
   `terminalPlacement.ts`/`readOnlyFilePlacement.ts` files from step 6 (now
   `.tsx`) with `agentChatWorkbenchPanesByGroup`+`AgentChatPaneActions`+
   `agentChatWorkbenchPane` (8417-8525), `terminalWorkbenchPanesByGroup`+
   `TerminalPaneActions`+`terminalWorkbenchPane` (9098-9201), and
   `readOnlyWorkbenchPanesByGroup`+`readOnlyWorkbenchPane` (9972-10068)
   respectively; new `workbench/editorGroups.tsx` for
   `buildWorkbenchEditorGroups` (8246-8362), the `WorkbenchPane`/
   `WorkbenchEditorGroupModel` types (8228-8245), and `initialWorkbenchGroups`
   (495-498). Update `App.tsx`'s direct usages of `initialWorkbenchGroups`
   (1218, 4792-4793, 5018, 5261, 5270) and `WorkbenchEditorGroupModel` (5255)
   to import from the new module. Run `npm run build` immediately after this
   commit. If `tsc -b`/`vite build` rejects the resulting cycle between
   `App.tsx` and the `workbench/*` files (importing `TerminalPaneBody`/
   `AgentChatPaneBody`/`ReadOnlyDocumentPane` back from `../App`), fall back
   within this same commit attempt to keeping the three `*WorkbenchPane`
   single-constructors (and their immediate `*WorkbenchPanesByGroup` callers)
   in `App.tsx` — deferred to land together with Phase 2's pane-body move —
   while still moving `buildWorkbenchEditorGroups` itself to call them via a
   still-local wrapper. Do not silently merge this fallback without noting it
   in the commit's `## AI Context`.

## Verification Plan

- `npm run build` (`tsc -b && vite build`) green after every commit listed
  above.
- All `test:*` suites green (`test:routes`, `test:api-error`,
  `test:resource-model`, `test:commands`, `test:root-picker`, `test:workbench`,
  `test:work-root-files`, `test:work-root-activity`, `test:agent-chat-tabs`,
  `test:agent-chat-bubbles`, `test:agent-chat-stream-merge`,
  `test:agent-chat-capabilities`, `test:agent-chat-client`, `test:terminals`,
  `test:open-work-root`, `test:document-viewer`, `test:git`,
  `test:keydown-suppression`, `test:hotkeys`, `test:settings`).
- `npm run test:browser` (Playwright acceptance gate) is currently RED at a
  pre-existing, unrelated failure: `dashboard-acceptance.spec.ts:2714`
  (`"create terminal and run a command"` resize-frame assertion), tracked by
  `260722-bug-e2e-terminal-resize-frame-assertion-fails` (idea/). The
  achievable bar for this phase is "no NEW browser failures beyond that known
  2714 point" — full green is blocked on a separate ticket and is not this
  phase's responsibility.
- Manual smoke (per ticket): terminal restore/reattach, the four modals, and
  Dockview layout are visually/behaviorally unchanged.

## Escalations

- None.
