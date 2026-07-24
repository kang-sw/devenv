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

### Result (4b492e60) - 2026-07-22

Landed 10 commits (339714fe..4b492e60). App.tsx 9,808 -> 7,626 lines (-2,182).
Cumulative Phase 1+2: 11,382 -> 7,626 (-3,756, ~33%).

Extracted (all props-only, verified NOT capturing App()/WorkbenchShell state):
ReadOnlyDocumentPane -> readOnlyDocumentPane.tsx, TerminalPaneBody ->
terminalPaneBody.tsx, AgentChatPaneBody -> agentChatPaneBody.tsx. Deferred
step-7 constructors -> workbench/ (readOnlyWorkbenchPane.tsx,
terminalWorkbenchPane.tsx, agentChatWorkbenchPane.tsx;
buildWorkbenchEditorGroups + types + initialWorkbenchGroups -> new neutral
workbench/editorGroups.ts; workRootActivityPlacementState/
workRootActivityWorkbenchPane -> workbench/activityPlacement.tsx). New
neutral module dismissableMenu.ts (useDismissableMenu). Relocated
TerminalPrefsContext -> terminalPrefs.ts, realAgentChatHarness ->
activitySessionClient.ts. resourcePresentation.tsx got a compile-only
`.js`-suffix barrel-import fix.

Barrel back-import hazard avoided: shared symbols relocated to neutral
modules imported by BOTH pane bodies and WorkbenchShell (not duplicated),
no `../App` back-imports (grep-verified per commit). The editorGroups.ts
<-> workbench/activityPlacement.tsx runtime circular import was verified
safe (cross-referenced bindings read only inside function bodies after
module-graph load, never top-level); opus correctness review confirmed
clean.

Verification: per-commit `npm run build` + NodeNext
`tsc -p tsconfig.route-tests.json` green; all 20 `test:*` suites green;
`npm run test:browser` fails ONLY at pre-existing
dashboard-acceptance.spec.ts:2714 (unrelated, tracked by
260722-bug-e2e-terminal-resize-frame-assertion-fails), no new failures.

Review: partitioned correctness (opus) clean + test (sonnet) clean with 1
deferred minor (pane bodies/workbench modules lack paired `.test.ts`; no
coverage regression - untested before the move too; consistent with Phase 1
precedent). No Critical/Important.

Residual acceptance (why this ticket is NOT yet closed to .done/): the
ticket's stated bar (full `npm run test:browser` green + manual smoke of
terminal restore/reattach, the four modals, and dockview layout) is not
fully met - full browser-green is blocked by the unrelated pre-existing
2714 failure, and manual smoke is pending a human pass. Both phases now
have Results; closure awaits the 2714 fix + a manual smoke pass.

Forward: buildWorkbenchEditorGroups now lives in a testable neutral module
(workbench/editorGroups.ts); adding a direct unit test for its
group-assignment / agentChat-append-last branching is a worthwhile
follow-up.

## Blocked (2026-07-24)

Both original verification prerequisites are now resolved — `260722-bug-e2e-open-work-root-locator-ambiguity` and `260722-bug-e2e-terminal-resize-frame-assertion-fails` are both in `.done/` — and the refactor itself is confirmed behavior-preserving: Phase 2's `AgentChatPaneBody` extraction (commit `edb8ae6d`) is a verified pure move (identical JSX, `agent-chat-pane-transcript` class, `data-testid`, and `if (pane.session)` gate; `styles.css` untouched), all `test:*` suites are green, and every browser-acceptance step reachable before the abort point passed (git worktree add/remove modals, dockview structure, per-root split isolation).

Full `test:browser` green — this ticket's stated closure bar — is now blocked by an INDEPENDENT pre-existing bug: `260713-bug-dashboard-acceptance-codex-tile-transcript-hidden` (currently `idea/`). Now that the 2714 resize-frame fix has landed, the serial acceptance suite advances to `dashboard-acceptance.spec.ts:2938` and fails there (the codex agent-chat transcript stays `hidden`, governed by a Dockview ancestor this refactor never touched; 260713 was filed 2026-07-13, nine days before this refactor, and independently reproduced the identical failure on pre-refactor baselines). Because the suite runs `mode: "serial"`, that failure also prevents the post-2938 acceptance steps (terminal restore/reattach, dockview split-durability) from executing, so they cannot be browser-validated until 260713 is fixed.

Two of the four ticket-named manual-smoke modals — SettingsModal and LinkedServerModal — additionally have no automated acceptance coverage and are genuinely human-smoke-only.

Closure therefore awaits, and cannot be completed autonomously: (1) `260713` fixed so the acceptance suite reaches green and validates the post-2938 terminal-restore / dockview-split steps, and (2) a human manual smoke of the SettingsModal and LinkedServerModal open/close flows. Skip this ticket in goal-drain selection until a human clears these.

## Unblocked & Closed (2026-07-25)

Both closure blockers are resolved; the ticket's stated bar (full
`npm run test:browser` green + terminal restore/reattach, the four modals, and
dockview layout validated) is now met by automation, so this ticket closes to
`.done/`. This supersedes the `## Blocked (2026-07-24)` "cannot be completed
autonomously" conclusion above.

Resolution path — an owner directive (2026-07-25) suspended the dashboard
agent-GUI feature rather than fixing 260713:

- **Blocker 1 (260713 codex-tile / serial suite stalled at :2938) — cleared by
  agent-GUI suspension.** The agent-chat GUI is suspended behind
  `AGENT_GUI_SUSPENDED` (Tier 1, commit `c3f5b42b`): the five agent-GUI
  acceptance steps (1983 / 2927 / 2985 / 3051 / 3930) are quarantined behind the
  flag, so the `mode: "serial"` suite advances past :2938 and now executes AND
  passes the previously-unreached post-2938 steps — terminal restore/reattach
  and dockview split-durability — inside `dashboard workRoot UI browser
  acceptance`.
- **Blocker 2 (SettingsModal / LinkedServerModal human-smoke-only) — cleared by
  new automated coverage.** Commit `c3f5b42b` adds Playwright acceptance steps
  exercising SettingsModal open / section-nav / close and the Add-server
  LinkedServerModal open / validate (submit disabled→enabled) / cancel flows.
  The other two named modals (GitWorktree Add/Remove) already had acceptance
  coverage. The "genuinely human-smoke-only" gap is closed; no human smoke
  remains required.

Verification (committed tree `c3f5b42b`, clean rebuild): `npm run test:browser`
→ vite build OK, Playwright `2 passed (48.8s)` — both `dashboard workRoot UI
browser acceptance` and `linked server root picker ...` green. Typecheck
`tsc -b` EXIT 0; all 20 `test:*` unit suites green.

Scope note: this refactor's own deliverable (behavior-preserving leaf / modal /
pane-body extraction, App.tsx 11,382 → 7,626, ~33%) was already complete with
both phase Results; only the external verification gate remained. No plugin
version bump (changes confined to `ws-dashboard/frontend/` + `ai-docs/`).

Related direction: the agent-GUI suspension is tracked by
`260725-refactor-dashboard-agent-gui-physical-module-isolation` (Tier 2
wire-out) and the `260713-*` family's `## Suspended` notes; the forward
PTY-agent pivot is `260725-research-ws-dashboard-pty-agent-pivot`.

## Spec Impact

No spec addressing: this is a behavior-preserving refactor with no
caller-visible change and no new behavioral contract. Contract-first spec: no.
