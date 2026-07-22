---
title: Decompose App.tsx - behavior-preserving leaf, modal, and pane-body extraction
related:
  260722-refactor-dashboard-app-tsx-state-decomposition: follow-on that decomposes the entangled WorkbenchShell/App() core
  260722-bug-e2e-open-work-root-locator-ambiguity: verification prerequisite - the Playwright acceptance gate for this refactor cannot pass until this e2e locator fix lands
related-mental-model:
  - ws-web-dashboard
sage-review-design: completed
sage-review-completeness: completed
---

# Decompose App.tsx - behavior-preserving leaf, modal, and pane-body extraction

## Background

`ws-dashboard/frontend/src/App.tsx` is ~11,382 lines. Most *logic* is already
extracted into sibling modules (24 flat `.ts/.tsx` files + a `workbench/`
subdirectory of 17 files, mostly with paired `.test.ts`); what remains in
App.tsx is the JSX-heavy UI component tree plus orchestration wiring. This
ticket covers only the low-risk, behavior-preserving half of shrinking that
file: hoisting leaf/pure/presentational/modal/pane-body code into sibling
modules following the established pattern. The hard part - untangling the two
deeply state-coupled giants (`WorkbenchShell`, `App()`) - is deliberately
deferred to the follow-on ticket `260722-refactor-dashboard-app-tsx-state-decomposition`.

Note: the cluster/line references below are a point-in-time snapshot
(2026-07-22) and will drift as the file changes. Treat the *clusters* as the
guide, not exact line numbers.

## Constraints

- Pure moves only: no behavior change, no new caller-visible contract beyond
  module boundaries. Terminal restore/reattach and dockview layout must not
  regress.
- Do NOT touch `WorkbenchShell`/`App()` state ownership or the 23-prop drill -
  that is the follow-on ticket's job.
- Prefer several small, reviewable extraction commits over one mega-commit.

## Phases

### Phase 1: Mechanical leaf extraction

Extract the zero/low-entanglement clusters into sibling modules mirroring the
existing pattern (paired `.test.ts` where any logic warrants it):
- Tail pure-format helpers (DetailItem, StateLine/Badge/Dot, normalizeServerRoute,
  kindLabel, resourceRowTone, etc. - no hooks, no shared state).
- Presentational components: ChromeIconButton, ResourceGlyph, WorkRootKindIcon,
  ToggleIcon, GitStatusPill, WorkbenchActivityBadge, InlineNotice.
- The four modals (each already prop-driven): GitWorktreeAddModal,
  GitWorktreeRemoveModal, SettingsModal, LinkedServerModal - alongside the
  already-extracted gitWorktreeAdd.ts/gitWorktreeRemove.ts/gitToolbar.ts logic.
- Pane-placement pure functions: buildWorkbenchEditorGroups, placeAgentChatPane*,
  placeTerminalSessions*, readOnlyFile* helpers - hoist into `workbench/`
  alongside policy.ts/editorGroupModel.ts.

### Result (34562f3c) - 2026-07-22

- Landed 6 of 7 planned extraction commits (d5e7b4b9..34562f3c). App.tsx
  11,382 -> 9,808 lines (-1,574).
- New sibling modules: resourcePresentation.tsx, chrome.tsx,
  workbenchChips.tsx, gitWorktreeAddModal.tsx, gitWorktreeRemoveModal.tsx,
  settingsModal.tsx, linkedServerModal.tsx, workbench/{paneOrder.ts,
  agentChatPlacement.ts, terminalPlacement.ts, readOnlyFilePlacement.ts,
  activityPlacement.tsx}; workbench/index.ts barrel extended;
  ActivityConsole.tsx got a compile-only 2-line `.js`-suffix import fix.
- Verification: `npm run build` green after every commit; all 19 `test:*`
  suites green; `npm run test:browser` fails ONLY at
  dashboard-acceptance.spec.ts:2714 (pre-existing, unrelated, tracked by
  260722-bug-e2e-terminal-resize-frame-assertion-fails) with no new
  failures. Full browser-green stays blocked by 2714, not by this refactor.
- Review: partitioned correctness + test, both clean with 1 deferred minor
  each (compile-only `: ReactNode` annotation on WorkRootActivityPane; no
  paired `.test.ts` for the 5 workbench modules, which had zero prior direct
  coverage). No Critical/Important.
- Deviation: plan step 7 (buildWorkbenchEditorGroups, the three
  `*WorkbenchPane` single-constructors, and
  workRootActivityPlacementState/workRootActivityWorkbenchPane) was NOT
  extracted - it entangles with the Phase-2-scoped pane bodies
  (TerminalPaneBody/AgentChatPaneBody/ReadOnlyDocumentPane) still resident
  in App.tsx; moving it would recreate the barrel back-import route-tests-
  tsconfig pollution. Per the plan's own fallback clause this payload rolls
  into Phase 2.
- Doc: barrel back-import build hazard captured as a ws-web-dashboard
  mental-model Common-Mistakes entry (a2784d59).

### Phase 2: Pane-body extraction

Move the three leaf pane renderers into their own files: TerminalPaneBody
(~654 lines), AgentChatPaneBody (~482), ReadOnlyDocumentPane (~370). These
consume `TerminalPrefsContext` but are otherwise structurally extractable -
keep the context consumption, just relocate the component. Phase 2
additionally lands the deferred step-7 pane-placement constructors
(buildWorkbenchEditorGroups, the three `*WorkbenchPane` single-constructors,
workRootActivityPlacementState/workRootActivityWorkbenchPane), which stay in
App.tsx until the pane bodies move.

Verification (both phases): `npm run build` (tsc -b && vite build) green + all
`test:*` suites green + `npm run test:browser` (Playwright acceptance gate;
mandatory for UI-facing changes per the ws-web-dashboard domain rule) green +
a manual smoke that terminal restore/reattach, the four modals, and dockview
layout are unchanged. Note: the Playwright acceptance suite is currently
red-lined by `260722-bug-e2e-open-work-root-locator-ambiguity`; that fix must
land first for the browser gate to pass.

## Spec Impact

No spec addressing: this is a behavior-preserving refactor with no
caller-visible change and no new behavioral contract. Contract-first spec: no.
