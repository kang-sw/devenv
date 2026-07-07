---
title: "Playwright acceptance gate fails on dead .panel-header selector; PanelHeader component is unused"
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

Unresolved question for whoever picks this up: is `.panel-header` actually
supposed to still exist somewhere in the current Dockview-based hierarchy
(and the redesign regressed it), or is the test's expectation simply stale
and should be updated to assert against Dockview's own header markup?
Needs a look at the redesign history / spec before deciding which side to
fix.

## Phases

### Phase 1: Decide and fix the panel-header drift

Determine whether the current Dockview-based workbench should still expose
a `.panel-header`-equivalent surface, then either restore it (and wire the
orphaned `PanelHeader` component back in, if still the right shape) or
update `expectContextSurfaceHierarchy` in
`e2e/dashboard-acceptance.spec.ts` to assert against the current header
DOM. Remove the dead `PanelHeader` component if it turns out to be fully
superseded.
