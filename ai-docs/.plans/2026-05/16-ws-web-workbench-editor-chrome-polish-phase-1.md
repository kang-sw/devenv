# Implementation Plan: 260516-feat-ws-web-workbench-editor-chrome-polish Phase 1

## Goal

Replace the current explanatory workbench mock with compact editor-like pane chrome while preserving the `left nav | workRoot workbench` information architecture, existing route/resource/auth behavior, and frontend-only workbench model boundaries.

## Concrete Changes

1. **Use editor-group data in `App.tsx`**
   - Replace the current `WorkbenchSplitGroup`/`SurfaceRow`/`SurfaceTile` topology UI at `ws-dashboard/frontend/src/App.tsx#L434-L510` with two editor groups whose visible model is `tabs + active pane body`.
   - Keep `WorkbenchToolbar` and existing `data-command-id` behavior at `ws-dashboard/frontend/src/App.tsx#L516-L584` unchanged unless a visible action truly dispatches through the same command layer.
   - Build deterministic frontend fixture surfaces from the existing `selection`, `selectedEntity`, and registry data:
     - group 1: main agent, persistent terminal, selected resource viewer/sub projections.
     - group 2: editor/detail, task view, diagnostics/events, inspector.
   - Track active tab per group with local React state; visible tabs must call `setActive...` and update the dominant pane body. Do not add fake clickable or draggable controls.
   - Remove large visible labels `Primary`, `Support`, `split group`, `Pinned row`, and `Opened row`; keep pinned/opened only as subtle metadata where useful.
   - Preserve left nav behavior in `WorkspaceRows`/`flattenEntities` at `ws-dashboard/frontend/src/App.tsx#L738-L770` and `ws-dashboard/frontend/src/App.tsx#L980-L1025`; do not re-add main/sub instance rows.

2. **Dockview integration boundary**
   - Revise `ws-dashboard/frontend/src/workbench/dockviewBridge.ts#L10-L18` so the bridge allows tab movement/reorder (`disableDnd` should no longer be hard-disabled) while keeping `disableFloatingGroups: true`.
   - If integrating `DockviewReact` directly into the visible shell, import Dockview styles (`dockview/dist/styles/dockview.css`) and mount panels through dashboard-owned component/render data; keep raw `DockviewApi`, panel handles, and group handles inside `App.tsx` or the bridge.
   - Use Dockview options from `dockview-core` carefully: `disableFloatingGroups` exists as an option, while `disableDnd` disables all Dockview tab/group drag (`node_modules/dockview-core/dist/esm/dockview/options.d.ts#L84-L101`).
   - Do not call or expose `addFloatingGroup`; Dockview exposes it on the API (`node_modules/dockview-core/dist/esm/api/component.api.d.ts#L516-L518`), but product behavior must keep floating/popout unavailable.
   - If Dockview drop overlays permit unwanted edge/floating behaviors, gate them with `onWillDrop`/`onWillShowOverlay` via bridge-owned logic instead of returning raw Dockview lifecycle objects.

3. **Pane components and CSS**
   - Replace card/grid styles in `ws-dashboard/frontend/src/styles.css#L604-L734` with dense editor/workbench styles: split container, group shell, thin tab strip, active tab, inactive tabs, pane body, pane toolbar/status line, and placeholder content.
   - Keep semantic tokens from `ws-dashboard/frontend/src/styles.css` and visual rules from `ws-dashboard/frontend/DESIGN.md`; avoid raw light colors, rounded cards, gradients, and heavy shadows.
   - Render close/PTY/lifecycle details such as `close: detach` and `pty: 80x24` as low-emphasis metadata/status text, not major chrome.
   - Update responsive rules around `ws-dashboard/frontend/src/styles.css#L774-L815` so groups stack on narrow screens without reverting to card grids.

4. **Tests**
   - Update `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts#L178-L185` to assert the new bridge contract: tab movement is allowed where practical and floating groups remain disabled.
   - Keep existing serialization/registry/placement/lifecycle/PTY assertions in `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts#L60-L257` and `#L259-Lend`; they protect the adapter boundary and logical surface policy.
   - Add focused assertions only if new pure helpers are introduced for fixture surface/group construction or tab state; avoid browser-only tests unless the project already has a DOM test harness.

## Constraints / Non-Goals

- No live PTY/editor/viewer/task/diagnostics/inspector/back-end lifecycle behavior.
- No persistent layout storage beyond existing model behavior.
- No default left-nav mainInstance/subInstance rows.
- No floating/popout groups.
- No fake controls: clickable tabs must switch the active pane; draggable affordances must be real Dockview tab movement or absent/disabled.
- Keep layout serialization sanitized: attachment ids/arrangement only, no daemon ids or registry-derived metadata (`ws-dashboard/frontend/src/workbench/layoutSerialization.ts#L52-L98`).

## Verification Commands

```sh
cd ws-dashboard/frontend && npm run test:routes
cd ws-dashboard/frontend && npm run test:workbench
cd ws-dashboard/frontend && npm run build
```

Optional manual visual check before Phase 2: run the Vite/dashboard shell and confirm the workbench reads as two editor groups with dominant pane bodies, not topology cards.
