---
title: "Terminal pane split snaps back: movePane mirror filters paneId against logicalKey-keyed terminalPanes"
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
related:
  260711-idea-dashboard-readonly-file-pane-order-split-registry-bug: distinct mechanism - that ticket covers readOnlyFilePaneOrderByGroup (no mirror write at all yet); this ticket covers the already-mirrored terminal path, broken by a key-format bug introduced inside the bc566a78 mirror fix itself
completed: 2026-07-14
---

# Terminal pane split snaps back: movePane mirror filters paneId against logicalKey-keyed terminalPanes

## Background

Commit `bc566a78` (2026-07-11) fixed a dogfooded regression where dragging a
terminal pane into another Dockview group snapped back to its original group
on the next render, by mirroring `movePane`'s drag-move result into
`terminalPaneOrderByGroup` (a separate flat registry consulted by
`terminalWorkbenchPanesByGroup`, which falls back a pane to `groups[0]` when
its id is missing from that registry).

That fix's mirror filter itself carried a key-format bug and never actually
worked:

```ts
next[groupId] = paneIds.filter((id) => id in terminalPanes);
```

`terminalPanes` (`App.tsx` ~3434) is a `Record<string, TerminalPaneState>`
keyed by `pane.logicalKey` (`persistentTerminal/${serverScopedIdentity}/${terminalId}`,
see `terminalPaneLogicalKey` in `terminals.ts:293-303`), set via
`[pane.logicalKey]: pane` in `createTerminalPane`. But `paneIds` here are
`WorkbenchPane.id` values - `pane.paneId` (`terminal:${encodeURIComponent(...)}`,
see `terminalPaneId` in `terminals.ts:305-309`), the id space
`commitWorkbenchPaneMoveIntoDynamicGroup`'s `result.paneOrderByGroup` and
`terminalWorkbenchPanesByGroup`'s `paneById` (keyed by `pane.id = pane.paneId`,
`terminalWorkbenchPane`) both operate in.

Since the two key shapes never intersect, `id in terminalPanes` was always
`false` for every terminal pane, so the "fix" filtered `paneIds` down to an
empty array on every move - the observable symptom (split snaps back to the
original group) was unchanged after `bc566a78` landed, because the mirror
write itself always emptied the target group's pane order.

## Root Cause

Confirmed by direct code inspection (not re-derived speculatively):

- `terminalPanes` state keyed by `pane.logicalKey` - `App.tsx:4880`
  (`setTerminalPanes` call in `createTerminalPane`).
- `TerminalPaneState` (`terminals.ts:53-56`) carries both `logicalKey: string`
  and `paneId: string` as distinct fields with disjoint string shapes
  (`terminalPaneLogicalKey` vs. `terminalPaneId`, `terminals.ts:293-309`).
- `WorkbenchPane.id = pane.paneId` - `terminalWorkbenchPane`, `App.tsx:7744`.
- `terminalWorkbenchPanesByGroup` (`App.tsx:7655-7689`) builds `paneById` from
  `pane.id` (`paneId` space), reads `terminalPaneOrderByGroup[groupId]`, and
  falls any pane not found there back to `groups[0]` (`App.tsx:7684-7686`).
- The buggy filter lived at `App.tsx:5519-5527`, inside `movePane`.

## Phases

### Phase 1: Fix the mirror filter to compare against the live paneId space

### Result (this commit) - 2026-07-14

Changed the filter in `movePane`'s `terminalPaneOrderByGroup` mirror
(`App.tsx` ~5519-5532) to build `livePaneIds` from
`Object.values(terminalPanes).map((pane) => pane.paneId)` and filter
`paneIds` against that set, instead of testing `id in terminalPanes`
(a logicalKey-keyed map). This is a minimal, surgical change to the existing
mirror write added in `bc566a78`; no other pane-order logic (agent-chat,
read-only-file panes) was touched.

No cheap unit-test seam exists for this exact code path: the mirror lives
inline inside the `movePane` closure in the `App()` component, reading
component state (`terminalPanes`) and the `result` from
`commitWorkbenchPaneMoveIntoDynamicGroup`; `App.tsx` exports only the `App`
component (no test file imports from it), and neighboring module-level pane
helpers in the same file (`placeTerminalSessions`,
`terminalWorkbenchPanesByGroup`) are likewise untested at the unit level. Per
guidance not to force a seam that doesn't exist, no new unit test was added.

Verification: `cd ws-dashboard/frontend && npm run build` (`tsc -b && vite
build`) passes clean. All existing frontend test scripts in `package.json`
(`test:routes`, `test:api-error`, `test:resource-model`, `test:commands`,
`test:root-picker`, `test:workbench`, `test:work-root-files`,
`test:work-root-activity`, `test:agent-chat-tabs`, `test:agent-chat-bubbles`,
`test:agent-chat-stream-merge`, `test:agent-chat-capabilities`,
`test:agent-chat-client`, `test:terminals`, `test:open-work-root`,
`test:document-viewer`, `test:git`) pass unchanged.

Forward note: live/manual verification in a running dashboard (dragging a
terminal pane between Dockview groups and confirming it stays put across a
re-render) was not performed as part of this fix, since the task explicitly
prohibited touching the live dogfooding daemon/gateway process. The fix
logic was verified by static/code-level confirmation of the key-space
mismatch and its correction, not by an interactive repro-then-fix cycle.

## Spec Impact

No existing spec stem addresses terminal pane drag-move/split-registry
behavior. This is a regression fix restoring already-intended behavior from
`bc566a78` (a terminal pane, once moved into another Dockview group, stays in
that group across renders) - no new caller-visible contract is introduced,
only a defect inside a previously-committed fix being corrected. Spec area:
none yet identified. Contract-first spec: no.
