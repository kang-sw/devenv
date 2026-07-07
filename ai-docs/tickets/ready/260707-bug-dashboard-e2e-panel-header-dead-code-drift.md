---
title: "Playwright acceptance gate fails on dead .panel-header selector; PanelHeader component is unused"
sage-review: completed
---

# Playwright acceptance gate fails on dead .panel-header selector; PanelHeader component is unused

## Background

This was the first time `ws-dashboard/frontend`'s Playwright acceptance
suite (`npm run test:browser`, `e2e/dashboard-acceptance.spec.ts`) could
actually run in this sandbox — system deps (`libasound2`, `xvfb`, fonts,
etc.) were missing until now, so it had never been exercised end-to-end in
this environment before. It fails deterministically (reproduced twice, not
a flake) at the very first UI assertion after opening a workRoot:

```
Error: page.evaluate: TypeError: Failed to execute 'getComputedStyle' on
'Window': parameter 1 is not of type 'Element'.
  at expectContextSurfaceHierarchy (e2e/dashboard-acceptance.spec.ts:329)
```

Root cause, confirmed via a temporary diagnostic patch (reverted, not
committed): of the 9 selectors `expectContextSurfaceHierarchy` queries,
only `.panel-header` resolves to `null`. `App.tsx:1303` still defines a
`PanelHeader` component that renders `<div className="panel-header
ws-toolbar">`, but `grep -rn "<PanelHeader" src` finds zero call sites —
the component is dead code. The workbench appears to have been redesigned
onto Dockview's own panel/tab header machinery
(`src/workbench/dockviewLayout.tsx`, `IDockviewPanelHeaderProps`) without
either removing the orphaned `PanelHeader` component or updating this e2e
assertion to match the new header structure.

Not related to the `260703` Phase 6/7 terminal-restore work verified in
this session — this selector check runs before any terminal/restore
assertions execute, and Phase 6/7 touched only `App.tsx`'s restore
ref/effect wiring and `workbench/{layoutRestore,terminalVisualRestore}.ts`.

Resolved decision (sage-review design pass): `dockviewLayout.tsx`'s
`DockviewWorkbenchTab` carries explicit CONTRACT comments stating the old
custom two-row pinned/opened header was intentionally retired in favor of
Dockview's own tab strip. Restoring `PanelHeader` would contradict that
documented redesign intent, and Spec Impact below confirms there is no
product contract requiring it back. Fix direction: remove the dead
`PanelHeader` component (and its now-unused `.panel-header`/`.ws-toolbar`
CSS, if not used elsewhere) and update the e2e assertion to match the
current Dockview-based header DOM, rather than resurrecting the old
component.

The two dropped assertions at `e2e/dashboard-acceptance.spec.ts:381-382`
(`panelHeaderBackground !== toolbarBackground` and `panelHeaderMinHeight
=== toolbarMinHeight`) encoded a real visual-hierarchy invariant. Re-point
that invariant at the already-queried `.dv-tabs-and-actions-container`
(Dockview's tabbar) vs `.workbench-toolbar` instead of deleting it outright
— assert the tabbar's background differs from the toolbar's background and
compare their min-heights, preserving the same contrast/alignment check the
original assertions encoded.

## Spec Impact

No existing spec stem addresses this behavior. This ticket removes dead
code and updates a stale e2e assertion; it introduces no new caller-visible
product contract (the workbench header surface's actual rendered behavior
is unchanged either way — only whether an orphaned component/selector
exists). Spec area: none (internal cleanup, no product contract change).
Contract-first spec: no.

## Phases

### Phase 1: Remove dead PanelHeader component and re-point the e2e hierarchy assertion

Remove the unused `PanelHeader` component and its dead `.panel-header` CSS
from `App.tsx` (and `styles.css` if the rule becomes fully unused). Update
`expectContextSurfaceHierarchy` in `e2e/dashboard-acceptance.spec.ts` to
drop the `.panel-header` query and re-point the
`panelHeaderBackground`/`panelHeaderMinHeight` comparisons at
`.dv-tabs-and-actions-container` (Dockview's tabbar) vs
`.workbench-toolbar`, preserving the same background-contrast and
min-height-match invariant the original assertions encoded. Verification:
re-run `npm run test:browser` (`e2e/dashboard-acceptance.spec.ts`) and
confirm both tests in the suite pass deterministically (run at least twice
to rule out the flake the ticket's Background explicitly considered and
ruled out for the original failure).
