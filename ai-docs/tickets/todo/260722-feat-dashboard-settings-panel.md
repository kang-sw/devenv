---
title: Dashboard general-purpose Settings panel (section registry, Terminal style first)
sage-review-design: required
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
spec: 260722-ws-dashboard-settings-panel
related:
  260722-feat-dashboard-hotkey-config-framework: future consumer — the
    hotkey-rebind editor section (Phase 2, out of scope here) will read/write
    through this framework's binding registry; this panel must not require
    rework to host that section later
related-mental-model:
  - ws-web-dashboard
sage-review-completeness: required
---

# feat: Dashboard general-purpose Settings panel (section registry, Terminal style first)

## Background

The dashboard has no general settings/preferences UI today (`grep -i
settings` over the frontend returns nothing). Preferences are ad-hoc
per-feature: each hand-rolls its own versioned browser-local blob keyed
`"ws-dashboard.<feature>.v<N>"` on top of the one shared low-level accessor
`browserStorage()` (`ws-dashboard/frontend/src/workRootFiles.ts:786`).
Examples following this pattern: `hotkeys.ts` (`ws-dashboard.hotkeys.v1`,
see its `browserStorage()`-based load/save around
`ws-dashboard/frontend/src/hotkeys.ts:420-515`), `workNavOrder.ts`,
`workbench/terminalVisualRestore.ts`, `terminals.ts`, and
`workbench/layoutRestore.ts`. There is no shared preferences-store module or
settings-surface schema registry — this ticket adds both, using Terminal
style options as the first concrete section.

Terminal font/theme is currently hardcoded at the single xterm construction
site, `ws-dashboard/frontend/src/App.tsx:9089-9099`:

```ts
const terminal = new Terminal({
  cursorBlink: true,
  fontFamily:
    '"MesloLGS NF", "JetBrainsMono Nerd Font", "CaskaydiaCove Nerd Font", ' +
    '"FiraCode Nerd Font", "Hack Nerd Font", ui-monospace, SFMono-Regular, ' +
    'Menlo, Consolas, "Liberation Mono", monospace',
  fontSize: 12,
  theme: { background: "#0b0d10" },
});
```

This sits inside a hot, heavily-annotated terminal-mount `useEffect` with
restore/reattach logic spanning roughly `App.tsx:9083-9230`; changes here
must not disturb visual-restore/reattach behavior. xterm.js supports
post-construction option mutation (e.g. `terminal.options.fontSize = ...`),
so a live preference change can update already-open panes without a full
terminal remount.

The dashboard already has a react-aria-components-based (`App.tsx:52`)
confirmation modal precedent to follow for visual/interaction consistency:
`GitWorktreeRemoveModal` (`App.tsx:3214`, using the `ModalOverlay` / `Modal`
/ `Dialog` composition, see the pattern starting around `App.tsx:3203`).

## Decisions

- **Global popup modal, not workroot/worktree-scoped.** Settings apply
  dashboard-wide and must be reachable regardless of which (or whether a)
  workroot is focused.
- **Section-registry architecture, not a hard-coded screen.** Option groups
  ("sections") register independently into the panel; the panel shell does
  not know their internals. This is required so the hotkey-rebind editor
  (future) and any later section can be added without reworking the shell.
- **First concrete section: Terminal style** (font family, font size,
  theme) — the driving requirement for this ticket.
- **Design for, but do not build, the hotkey-rebind section now.** The
  binding registry it will read from already exists
  (`260722-feat-dashboard-hotkey-config-framework`, done). This ticket's
  Phase 1 must leave the registry contract able to host that section later
  without shell rework; actually building it is a future phase/ticket.

## Constraints

- Terminal preference changes must apply live to already-open terminal
  panes (via xterm option mutation) and persist across reload — no forced
  full remount of live sessions just to apply a style change.
- Must not regress the terminal visual-restore/reattach logic around
  `App.tsx:9083-9230`.
- The panel must be operable without a mouse: open, navigate sections, and
  close via keyboard.
- Follow the existing `"ws-dashboard.<feature>.v<N>"` versioned-JSON,
  defensive-parse, `browserStorage()`-backed persistence convention rather
  than introducing a new storage mechanism.

## Prior Art

- `GitWorktreeRemoveModal` (`App.tsx:3214`) — react-aria `ModalOverlay` /
  `Modal` / `Dialog` composition to reuse for the Settings modal shell.
- `hotkeys.ts` (`ws-dashboard.hotkeys.v1`) — existing versioned-blob
  load/save precedent to generalize into a shared helper.
- `260722-feat-dashboard-hotkey-config-framework` (done) — the binding
  registry the future hotkey-rebind section will read from; also a
  candidate source for the keyboard entry point into this panel.

## Spec Impact

Covered by `## 🚧 Dashboard Settings Panel {#260722-ws-dashboard-settings-panel}`
in `ai-docs/spec/ws-web-dashboard/index.md`: the global Settings modal
surface, the section-registry contract, the shared namespaced
preferences-store contract, and the Terminal-style section behavior. The
section is marked `🚧` (planned) until Phase 1 lands.

## Phases

### Phase 1: Settings modal shell, section registry, shared prefs store, Terminal-style section

- Add a shared namespaced-preferences helper generalizing the existing
  per-feature pattern, e.g. `loadNamespacedPrefs<T>(key, version, parse)` /
  `saveNamespacedPrefs<T>(key, version, value)` over `browserStorage()`,
  matching the `"ws-dashboard.<feature>.v<N>"` key convention and the
  defensive-parse-with-fallback-to-defaults behavior used by `hotkeys.ts`
  and siblings.
- Add a Settings modal shell (react-aria `ModalOverlay`/`Modal`/`Dialog`,
  following `GitWorktreeRemoveModal`'s pattern) that is global/app-level,
  not workroot-scoped, and mountable from a single place regardless of
  which workroot is focused.
- Add a section registry: sections declare an id, label, and render
  function/component; the shell renders a section list/nav plus the active
  section's UI without knowing section internals. Design the registration
  contract so a later hotkey-rebind section can register without shell
  changes (do not build that section now).
- Implement the Terminal-style section: font family, font size, and theme
  controls, persisted via the shared prefs helper under a
  `ws-dashboard.settings.terminal.v1`-style key (exact key/shape to be
  finalized during implementation/spec).
- Wire the persisted Terminal preference into the terminal construction
  site (`App.tsx:9089-9099`): initial construction reads the persisted
  value (falling back to today's hardcoded defaults), and a live preference
  change updates already-open panes via xterm's post-construction option
  mutation (e.g. `terminal.options.fontSize = ...`) rather than remounting.
  Must not disturb the surrounding restore/reattach logic
  (`App.tsx:9083-9230`).
- Add a keyboard-reachable entry point to open the modal — a hotkey binding
  through the existing hotkey framework and/or a visible affordance; exact
  binding choice is an implementation/design decision, but opening the
  panel without a mouse is required.

Acceptance criteria:

- Settings modal opens and closes cleanly, including via keyboard, from
  any workroot context (or none focused).
- Terminal font family, font size, and theme changes apply live to
  already-open terminal panes and persist across a full page reload.
- Sections are independently registered (adding one does not require
  editing another section's code), and the registry contract is shown to
  accommodate a hypothetical future section (e.g. hotkeys) without shell
  changes.
- No regression to terminal visual-restore/reattach behavior.

### Phase 2: Hotkey-rebind editor section (future)

Add a hotkey-rebind editor section over the existing
`260722-feat-dashboard-hotkey-config-framework` binding registry, registered
into the Phase 1 section registry. Not required for this ticket's initial
slice; tracked here so the dependency is visible from both sides.
