# Plan: 260707-bug-dashboard-e2e-multi-root-locator-leakage — Phase 2: Diagnose and fix the TOML/text language-detection mismatch

## Relevant Ticket Contract
- Phase 2 goal (verbatim): "Root-cause why `.document-source-viewer[data-editor-read-only="true"]` reports `data-editor-language="text"` instead of the expected `"toml"` for the relevant fixture file, and fix the underlying cause (or the test fixture if the test's expectation is itself stale)."
- The phase body is deliberately open ("fix the underlying cause"), even though the title/Background frame the defect as a "language-detection mismatch." The Spec Impact section already anticipates that diagnosis may reveal "a real product bug fix" vs "a stale-fixture fix," and instructs that if it turns out to be a genuine product-visible behavior change, the phase "should add its own `## Spec Impact` addendum before landing."
- Phase 1 has landed (`026b1b8c`); the failing assertion's locator is already correctly root-scoped. The remaining failure is NOT a locator-scoping bug.
- Verification boundary: `npm run test:browser` run twice consecutively; once both phases have landed, the full suite must pass green on both runs. If landing Phase 2 alone, at minimum the TOML assertion itself must pass.

## Out of Scope
- Phase 1's locator-scoping work (landed and verified independently).
- Any still-undiscovered failures later in the spec file (ticket says the suite has only ever been observed to fail at this one point after Phase 1's fixes).
- The `documentEditorLanguageId` language-detection logic and the server-side `language_hint_for_extension` mapping — both were read in full and are correct for `.toml` in every path; do NOT edit them.
- Changing the test fixture or the assertion's expectation to make it pass — the assertion's intent (a single-clicked file becomes the visible pane) is correct product behavior; weakening it would be a test-passing bypass.

## Codebase Findings

### Confirmed root cause (premise correction)
The ticket's "language-detection mismatch" framing is **wrong**. Confirmed by the survey's live repro (screenshot + accessibility snapshot at `ws-dashboard/frontend/test-results/dashboard-acceptance-dashb-734ce-kRoot-UI-browser-acceptance/`): at the failing assertion the visible `.document-source-viewer` is the **previously-pinned `gate-readme.txt` pane** (a `.txt` file, for which `data-editor-language="text"` is objectively correct), not the newly single-clicked `gate-config.toml` preview. Dockview only mounts the **active** tab's content within a group, so the inactive toml tab's viewer is not in the DOM at all — the locator correctly resolves to the one mounted (readme) viewer. The real defect: **single-clicking a file to open a preview does not activate the new preview tab when a sibling pinned pane in the same group is currently active.**

### Test sequence that triggers it
`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`:
- Prior step (~L1901-1935): double-clicks a file to **pin** it into `group-2`; that pinned pane becomes the group's active pane.
- Failing step (L1939-1954): `tomlRow.click()` (single click) opens `gate-config.toml` as a **preview** pane in the **same** `group-2`, but focus stays on the pinned readme pane. Assertion at `spec.ts#L1954` then reads the readme viewer's `text` language.

### The activation pipeline (fully traced — executor need not re-derive)
Single-click open routes through this chain, all in `ws-dashboard/frontend/src/App.tsx` unless noted:
1. `openReadOnlyFile` (`App.tsx#L736-880`), gesture `singleClick` → `mode = "preview"`. For a new file with no existing pinned pane it takes the `openNew` path (`decideSurfaceOpenWithDynamicGroups`, `src/workbench/policy.ts#L169-227`), which targets the existing editor row `group-2` (editor `rowPolicy` prefers group 2), `createdGroupId = null`. The pane is appended to `readOnlyFilePaneOrderByGroup[group-2]` (`App.tsx#L829-838`, runs because `placement.type === "openNew"`), then `focusPane(pane.id)` (`App.tsx#L840`) sets `activeReadOnlyFilePaneRequest = { paneId, sequence }`.
2. Effect `App.tsx#L4486-4511` (deps `[activeReadOnlyFilePaneRequest, editorGroups]`): once `editorGroups` contains the new pane, it finds `targetGroup` and calls `setActivePaneByGroupForSelected(selectWorkbenchPane(current, targetGroup.id, paneId))`. Sequence-guarded via `focusedReadOnlyRequest.current` so it fires once per open.
3. `setActivePaneByGroupForSelected` (`App.tsx#L3766-3782`) writes `activePaneByRoot[selectedRootKey][group.id] = paneId`.
4. `activePaneByRoot[rootKey]` is passed as `activePaneByGroup` to each `DockviewWorkbenchLayout` (`App.tsx#L5026`).
5. `DockviewWorkbenchLayout` re-runs `syncPanels` whenever `activePaneByGroup` or `groups` change (`src/workbench/dockviewLayout.tsx#L161-197`, `useEffect(syncPanels, [syncPanels])` at L295; `syncPanels` is `useCallback(..., [activePaneByGroup, groups])`).
6. `syncDockviewWorkbench` (`src/workbench/dockviewLayout.tsx#L403-520`): new panels are added with `inactive: pane.id !== activePaneId` (L445). For an **already-existing** panel it calls `existingPanel.api.setActive()` only when `pane.id === activePaneId && !dockviewPanelIsSelectedWithinGroup(existingPanel)` (L506-511; predicate in `src/workbench/dockviewLayoutModel.ts#L27-31`).

Note: the render-time group model (`openWorkRootInstances[].editorGroups`, `App.tsx#L5016`) and the effect's `editorGroups` (`workbenchModel.editorGroups`, `App.tsx#L3874-3888`) are built by the **same** `buildEditorGroupsForRoot` builder, so a divergence between the two group models is ruled out — the effect's `targetGroup` lookup does see the new pane.

### Why the two-pass design leaves the pane inactive (mechanism)
React commits effects child-first. On the render where the toml pane first enters `editorGroups`, `activePaneByRoot[group-2]` is still the **stale** pinned-readme id (the L4486 effect that updates it has not run yet). So the child `syncPanels` runs first and executes `addPanel(toml, inactive: tomlId !== readmeId → true)` — the toml panel is created **inactive**. The parent effect (L4486) then updates `activePaneByRoot[group-2] = tomlId`, which *should* trigger a second `syncPanels` pass that hits the L506-511 `setActive()` branch. The observed failure is that the toml panel remains inactive despite this — i.e. the second-pass activation does not take effect. Every code path is statically correct, which is itself the signal that the break is a runtime render/effect-ordering or Dockview-internal-state issue (e.g. `addPanel({inactive})` interaction with Dockview's own active-panel tracking, or the second `syncPanels` pass reading a Dockview group state where `dockviewPanelIsSelectedWithinGroup` unexpectedly returns true) — not a static logic error a fresh reading will reveal.

### Reusable mechanism for the fix (no new machinery needed)
`setActivePaneByGroupForSelected` + `selectWorkbenchPane` (`src/workbench/editorGroupModel.ts#L187-196`) is the existing, correct way to declare a group's active pane. The `openNew` path in `openReadOnlyFile` already knows both the target group (`placement.groupId`) and the new pane id (`pane.id`) **synchronously**. The fix is to declare the active pane in the **same state batch** that adds the pane, so the very first `syncPanels` pass creates the panel active (`inactive: false`) instead of relying on the fragile deferred second pass.

### Rejected shortcut paths
- Editing `documentEditorLanguageId` or the server language hint: rejected — they are correct; this would mask nothing (the toml viewer isn't even mounted).
- Weakening/adjusting the test assertion or fixture: rejected — the assertion encodes correct product intent (single-click preview should be the visible pane).
- Blindly "hardening" `syncDockviewWorkbench`'s activation without a runtime repro: rejected — this pipeline is shared by pinned/preview/terminal/activity panes (Phase 1's audit had to reason about it for multi-root state); a blind change risks regressing tab-focus elsewhere.

## Implementation Plan

> This fix touches the shared pane-activation pipeline. Do the instrumented confirmation step BEFORE the edit, and the full two-run verification after — both are cheap (~28s to the failing assertion).

1. **Confirm the mechanism at runtime (diagnostic, revert before landing).** Temporarily add a `console.log` (or Playwright-visible marker) inside `syncDockviewWorkbench` (`src/workbench/dockviewLayout.tsx`) at both the add-panel branch (L438-472) and the existing-panel activate branch (L506-511), logging `group.id`, `pane.id`, `activePaneId`, the computed `inactive` value, and `dockviewPanelIsSelectedWithinGroup(existingPanel)`. Run `npm run test:browser` once and read the log for the `group-2` toml pane. This confirms which failure mode is real: (a) the second-pass `setActive()` never runs (activePaneByGroup not updated / effect not re-firing), or (b) it runs but `dockviewPanelIsSelectedWithinGroup` returns true so it is skipped, or Dockview ignores the late `setActive()`. Remove the instrumentation before committing.

2. **Apply the synchronous-activation fix (recommended primary fix).** In `openReadOnlyFile` (`App.tsx#L736-880`), in the `openNew` path (after `placement.groupId` and `pane.id` are known — alongside the existing `setReadOnlyFilePaneOrderByGroup` update at L821-839, and together with the existing `focusPane(pane.id)` at L840), also declare the pane active synchronously in the same batch via `setActivePaneByGroupForSelected(current => selectWorkbenchPane(current, placement.groupId, pane.id))`. This makes `activePaneByGroup[placement.groupId] === pane.id` true on the first render the pane appears, so `syncDockviewWorkbench` creates the panel with `inactive: false` on the first pass — eliminating dependence on the deferred second-pass activation.
   - Contract note: `setActivePaneByGroupForSelected` no-ops unless the pane's root is the selected root (`App.tsx#L3771`). Single-click-from-file-explorer always targets the selected root, and the existing effect path already routes through selected-root state, so this is consistent. Do NOT remove the existing `focusPane`/`activeReadOnlyFilePaneRequest` effect indirection — it remains the general mechanism (and still covers the pinned-existing early-return path at `App.tsx#L770-784`, where the target group id is not directly available). The synchronous call is an additive, same-batch fast path for the `openNew` case.
   - If step 1's diagnostic reveals the break is specifically (b) — Dockview ignoring a late `setActive()` even though `activePaneId` is correct — the synchronous fix still resolves it (the panel is never created inactive in the first place), so it is the correct fix under either failure mode.

3. **Add the Spec Impact addendum.** Because the diagnosis is a genuine product-behavior fix (preview activation), per the ticket's Spec Impact instruction append a short `## Spec Impact` addendum to the phase/ticket noting the corrected root cause (preview-tab activation, not language detection) and that no existing spec stem covers it. This is a ticket-doc edit, not a source change.

## Verification Plan
- Post-impl, manual (the definitive check): `npm run test:browser` — confirm the assertion at `spec.ts#L1954` now reads `toml` (the toml preview is the visible/active pane). Reproduces in ~28s to the assertion.
- Then run `npm run test:browser` twice consecutively; with both phases landed, the full suite must be green on both runs (the ticket's verification boundary).
- Regression guard: confirm existing pane-activation behaviors still work — pinning a file (double-click), switching between terminal tabs, and opening the WorkRoot Activity pane should each still activate the intended tab. The placement unit tests (`src/workbench/workbenchModel.test.ts`) and any dockview-layout unit tests should stay green.
- Remove the step-1 instrumentation before the final run/commit.

## Escalations
**Lead decision required before execution — two items:**

1. **Premise / scope amendment.** The ticket's Phase 2 title and Background frame this as a "TOML/text language-detection mismatch." The live-verified root cause is a **preview-tab activation bug** (a single-clicked preview does not become active when a sibling pinned pane in the same group is active), not a language-detection defect. The phase body ("fix the underlying cause") and Spec Impact section (which pre-authorizes a Spec Impact addendum if it turns out to be a real product bug) both accommodate this, so this does **not** require redefining the settled goal — but a research/ticket-updating change should not silently reinterpret the phase title. Confidence in the diagnosis: **high** (live repro, screenshot, accessibility snapshot). Recommend the lead approve proceeding under the corrected root cause and adding the Spec Impact addendum (Implementation step 3).

2. **Touching the shared pane-activation pipeline.** The recommended fix adds a synchronous active-pane declaration in `openReadOnlyFile`'s `openNew` path. Confidence the fix resolves the symptom: **medium-high** from static analysis, but the exact runtime failure mode is only confirmable by the step-1 instrumented repro (static analysis shows the current two-pass mechanism *should* already work, so the break is runtime-level). This pipeline is shared by pinned/preview/terminal/activity panes; the fix is additive and same-batch (low blast radius), but the lead should confirm that touching this shared pipeline (vs. a dedicated design pass) is acceptable given Phase 1's audit already flagged its sensitivity. Evidence: full pipeline trace in Codebase Findings above; debug artifacts under `ws-dashboard/frontend/test-results/dashboard-acceptance-dashb-734ce-kRoot-UI-browser-acceptance/` (regenerable via `npm run test:browser`).

### Resolved (2026-07-07): both escalations approved

1. Approved proceeding under the corrected root cause (preview-tab
   activation bug, not language detection). Ticket Phase 2 title/Background
   will be reframed by the lead directly (ticket-doc edit) alongside the
   Spec Impact addendum from Implementation step 3 — implementer does not
   need to touch the ticket file itself beyond what step 3 already asks.
2. Approved touching the shared pane-activation pipeline via the proposed
   additive, same-batch fix, gated on running the instrumented repro (step
   1) first to confirm the exact runtime failure mode before committing to
   the fix. Proceed with the plan as written.

## Debug Artifacts (research continuity)
- Live repro screenshot: `ws-dashboard/frontend/test-results/dashboard-acceptance-dashb-734ce-kRoot-UI-browser-acceptance/test-failed-1.png` (readme pinned+active, toml open but unfocused).
- Full Playwright trace: `ws-dashboard/frontend/test-results/dashboard-acceptance-dashb-734ce-kRoot-UI-browser-acceptance/trace.zip` (`npx playwright show-trace <path>`).
- Untracked/gitignored build outputs; safe to regenerate or delete.
