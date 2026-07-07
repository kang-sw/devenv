# Plan: 260707-bug-dashboard-e2e-panel-header-dead-code-drift — Phase 1: Remove dead PanelHeader component and re-point the e2e hierarchy assertion

## Relevant Ticket Contract
- Remove the unused `PanelHeader` component and its dead `.panel-header` CSS
  from `App.tsx` (and `styles.css` if the rule becomes fully unused).
- Update `expectContextSurfaceHierarchy` to drop the `.panel-header` query and
  re-point `panelHeaderBackground`/`panelHeaderMinHeight` at
  `.dv-tabs-and-actions-container` (Dockview's tabbar) vs `.workbench-toolbar`,
  preserving the same background-contrast and min-height-match invariant.
- Resolved decision: do not resurrect `PanelHeader`; Dockview's own tab strip
  is the intentional replacement (`dockviewLayout.tsx` CONTRACT comments).
- Verification boundary: re-run `npm run test:browser`
  (`e2e/dashboard-acceptance.spec.ts`), confirm both tests pass
  deterministically, run at least twice.
- No spec impact (internal cleanup, no product contract change).

## Out of Scope
- `dockviewLayout.tsx` / Dockview tab-header machinery itself — already correct,
  not touched.
- Phase 6/7 terminal-restore work — unrelated, do not touch `App.tsx` restore
  ref/effect wiring.
- Any new product-visible header redesign — this is cleanup + assertion
  re-pointing only.

## Codebase Findings
- `ws-dashboard/frontend/src/App.tsx#L1303-L1355` — `PanelHeader` component
  definition (`<div className="panel-header ws-toolbar">`). Confirmed zero
  `<PanelHeader` call sites via grep — fully dead.
- `ws-dashboard/frontend/src/App.tsx#L1320` — `StateLine` is only invoked from
  inside `PanelHeader` (`grep -n "StateLine" src/App.tsx` shows exactly this
  one call site plus the `function StateLine` definition at L8060). Removing
  `PanelHeader` leaves `StateLine` (L8060-8070) with zero call sites. Ticket
  phase text only names `PanelHeader` + `.panel-header` CSS; `noUnusedLocals`
  is NOT set in `tsconfig.app.json`/`tsconfig.node.json`, so an orphaned
  top-level function will not fail `tsc -b`. Executor judgment call: leaving
  `StateLine` in place is safe and strictly in-scope; removing it too is a
  reasonable minor extension but not mandated. Recommend leaving it unless a
  fast follow-up wants full dead-code sweep — do not expand scope silently.
- `ws-dashboard/frontend/src/App.tsx#L1318-1323` — `panel-title-block`,
  `panel-title`, `action-strip` classNames used only inside `PanelHeader`
  (`grep` confirms no other className usages of `panel-title-block`/
  `panel-title`; `action-strip` also appears in `styles.css` media query at
  L1445 but no other App.tsx usage). Same judgment call as `StateLine`: their
  CSS rules become orphaned once `.panel-header` is removed, but ticket text
  only mandates `.panel-header`. Leave them unless doing a full sweep.
- `ws-dashboard/frontend/src/App.tsx#L5228` — `.workbench-toolbar` div uses
  `className="workbench-toolbar ws-toolbar"`; `.ws-toolbar` CSS class stays
  (still used here and at `readonly-text-pane-header` L7431) — do NOT remove
  `.ws-toolbar` CSS, only `.panel-header`.
- `ws-dashboard/frontend/src/styles.css#L333-342` — `.panel-header` rule to
  delete in full.
- `ws-dashboard/frontend/src/styles.css#L1438-1444` — combinator selector
  `.panel-header,\n  .detail-heading { ... }` inside `@media (max-width: 560px)`.
  `.detail-heading` is still used (`App.tsx#L7937`) — remove only the
  `.panel-header,` selector line, keep the `.detail-heading` rule body intact.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L328-389` —
  `expectContextSurfaceHierarchy`. Already queries
  `.dv-tabs-and-actions-container` at L344-346 into `tabbarStyle`, and already
  exposes `tabbarBackground`/`tabbarDivider` (used in existing assertions at
  L387-388). The re-point should reuse this existing `tabbarStyle` rather than
  add a second query — add a `tabbarMinHeight: tabbarStyle.minHeight` field,
  drop `panelHeaderStyle` (L335) and `panelHeaderBackground`/
  `panelHeaderMinHeight` fields (L354-355), and change the two assertions:
  - L380: `expect(hierarchy.panelHeaderBackground).not.toBe(hierarchy.toolbarBackground)`
    → `expect(hierarchy.tabbarBackground).not.toBe(hierarchy.toolbarBackground)`
  - L381: `expect(hierarchy.panelHeaderMinHeight).toBe(hierarchy.toolbarMinHeight)`
    → `expect(hierarchy.tabbarMinHeight).toBe(hierarchy.toolbarMinHeight)`
  Note L387 already asserts `tabbarBackground !== paneBodyBackground` — this
  is a distinct invariant, keep it; do not merge/dedupe with the new
  `tabbarBackground !== toolbarBackground` assertion.

## Implementation Plan
1. `ws-dashboard/frontend/src/App.tsx#L1303-1355` — delete the `PanelHeader`
   function entirely.
2. `ws-dashboard/frontend/src/styles.css#L333-342` — delete the `.panel-header`
   rule.
3. `ws-dashboard/frontend/src/styles.css#L1438-1444` — in the
   `@media (max-width: 560px)` block, remove the `.panel-header,` selector
   line, leaving `.detail-heading { align-items: stretch; flex-direction: column; }`
   as a standalone rule.
4. `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L328-377`:
   - Remove L335 (`panelHeaderStyle` query).
   - Add `tabbarMinHeight: tabbarStyle.minHeight,` next to the existing
     `tabbarBackground`/`tabbarDivider` fields.
   - Remove the `panelHeaderBackground`/`panelHeaderMinHeight` fields from the
     returned object.
5. `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L380-381` — rewrite
   the two assertions to use `hierarchy.tabbarBackground` vs
   `hierarchy.toolbarBackground` (not-equal) and `hierarchy.tabbarMinHeight`
   vs `hierarchy.toolbarMinHeight` (equal), per Codebase Findings above.
6. Grep sweep after edits: confirm `.panel-header` has zero remaining
   references in `src/` and `e2e/`, and `PanelHeader` has zero remaining
   references in `src/`.

## Verification Plan
- `cd ws-dashboard/frontend && npm run test:browser` — runs `tsc -b && vite
  build`, builds the daemon, then Playwright suite
  (`e2e/dashboard-acceptance.spec.ts`).
- Run the suite at least twice in a row to rule out flake, per the ticket's
  explicit verification boundary.
- Confirm both tests in the file pass deterministically both runs.

## Escalations

### Resolved (2026-07-07): tabbarMinHeight === toolbarMinHeight invariant does not hold

Implementer confirmed `.dv-tabs-and-actions-container` never sets `min-height`
(Dockview controls tab-strip height via `--dv-tabs-and-actions-container-height`,
unset in this project, so computed `minHeight` is always `"auto"`), while
`.workbench-toolbar` has an explicit `min-height: 56px`. The old
`panelHeaderMinHeight === toolbarMinHeight` assertion could never have passed
against a live `.panel-header` either, since `PanelHeader` had zero render
call sites by the time this drift was noticed — it was already stale/unvalidated,
not a real product invariant this ticket should restore.

Decision: drop the min-height equality assertion entirely rather than
introduce a Dockview CSS var override (that would be a real visual change,
out of scope per this ticket's Spec Impact: "no product contract change").
Keep only the background-contrast assertion, re-pointed at
`tabbarBackground`/`toolbarBackground` as already planned. Remove the now
unused `tabbarMinHeight` field from `expectContextSurfaceHierarchy`'s
returned object instead of adding it.

Updated step 5: rewrite only the background-contrast assertion
(`hierarchy.tabbarBackground` vs `hierarchy.toolbarBackground`, not-equal);
delete the `panelHeaderMinHeight`/min-height-equality assertion line outright,
no replacement assertion needed.

### Resolved (2026-07-07): expectDockviewWorkbench duplicate-selector drift, in-scope extension authorized

Implementer found a second, pre-existing, unrelated e2e drift blocking full
verification: `expectDockviewWorkbench` (same spec file) asserts a bare
`[data-workbench-layout-owner="dockview"]` locator resolves to exactly one
element, which held before the multi-root-mount feature landed
(`c7a1f59c feat(ws-dashboard): mount one dockview workbench instance per
open work root`) but no longer holds once a second work root is opened —
each mounted (even inactive, `display:none`) root instance carries that
same attribute, so Playwright's strict-mode locator throws on 2+ matches.
Confirmed pre-existing (not caused by this ticket's diff) by stashing the
diff and reproducing a different, earlier crash instead.

Decision: authorize fixing `expectDockviewWorkbench` in this same phase/
commit as an in-scope extension — same file, same "stale e2e assertion
after a workbench redesign" drift category as the ticket's named
`.panel-header` issue, low-risk locator-scoping change, no product code
touched. Scope the locator to the active work root only (find the correct
active-root discriminator attribute by inspecting `App.tsx`'s
`openWorkRootInstances.map` rendering, e.g. pairing with whatever attribute
marks the active/visible root — do not guess a name, read the actual
render code first). Do not touch the multi-root-mount feature itself
(`App.tsx`'s `openWorkRootInstances.map`) — test-only fix.

### Resolved (2026-07-07): verification boundary redefined; further e2e drift split to a new ticket

After the `expectDockviewWorkbench` fix, the suite progressed further but
hit a third failure (TOML vs text language-detection mismatch — unrelated
feature bug, not locator-scoping) and, on a second run, a fourth failure
(`confirmSessionClose` tab locator matching 2 elements — same
multi-root-leakage class as the just-fixed `expectDockviewWorkbench` case,
but a different call site). This confirms the multi-root-mount feature
(`c7a1f59c`) left multiple, scattered unscoped locators across the 2700+
line spec file, plus at least one unrelated feature bug — an open-ended
set, not bounded by this ticket's named scope.

Decision: stop expanding this ticket. Redefine Phase 1's verification
boundary to: confirm zero `panel-header`/`PanelHeader` references remain,
confirm the two authorized assertion fixes (`.panel-header` removal +
`expectDockviewWorkbench` active-root scoping) are each individually
correct — i.e., run the suite far enough to confirm neither previously
-fixed issue regresses/reappears — then commit. A full green
`npm run test:browser` run is deferred to a new follow-up ticket
(systemic e2e multi-root-locator-leakage audit + the TOML/text
language-detection bug), which the lead will file separately. Do not
diagnose or fix the third/fourth failures under this ticket.
