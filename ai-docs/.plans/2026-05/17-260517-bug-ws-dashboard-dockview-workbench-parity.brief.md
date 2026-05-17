# Brief: 260517-bug-ws-dashboard-dockview-workbench-parity

## Intent

Restore the dashboard workbench so Dockview is the visible layout owner for
workbench groups, tabs, split sizing, and pane attachment while preserving the
file and terminal behavior already recovered in the dashboard browser gate. The
implementation must remove the current custom React/CSS/HTML5-drag tab/split
shell as the authoritative workbench layout and keep dashboard-owned policy
above Dockview.

## Scope Boundary

Implement the whole ready ticket:

- Phase 1: Mount Dockview as the workbench layout owner.
- Phase 2: Preserve file and terminal pane parity.
- Phase 3: Add substrate assertions and browser acceptance.

Existing skeleton commits are part of the acceptance surface:

- `7af96f18` - lead draft for Dockview layout contracts and browser substrate assertions.
- `3c75198` - populated compile-clean skeleton with `DockviewReact` mounted under
  `data-workbench-layout-owner="dockview"`.

Out of scope:

- Preview-to-pinned file tabs, close affordance polish, new tab insertion policy,
  configuration/settings UI, CodeMirror/Monaco, writable editing, and layout
  persistence redesign.
- Making raw Dockview panel/group handles product-facing.

## Caller-Visible Contract

- After opening a workRoot, the visible workbench is Dockview-backed. Browser
  acceptance can see `data-workbench-layout-owner="dockview"` and Dockview DOM
  under it, and the old `.workbench-splits > .workbench-group` layout is absent.
- Workbench groups still present the current two-group default intent:
  durable agent or persistent terminal surfaces prefer primary/focused group;
  support surfaces such as read-only files and detail panes prefer the support
  group.
- Read-only file panes still open from the file explorer, load daemon-authorized
  content, focus duplicates, and render only under their owning workRoot.
- Terminal panes still create daemon PTY sessions, render through xterm.js,
  connect through the WebSocket path, preserve keyboard/input fidelity, fill the
  visible pane, keep the active bottom row visible, reconstruct after refresh,
  and terminate the daemon session on explicit close.
- Dashboard policy owns surface identity, duplicate-open focus, placement,
  close behavior, and restore sanitization. Dockview owns visible layout
  mechanics only.

## Implementation Direction

- Start from the skeleton `DockviewWorkbenchLayout` in
  `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx`.
- Replace rebuild-only or placeholder synchronization with a robust enough
  adapter for the current two-group workbench. It may be simple, but it must not
  fight dashboard state, leak raw Dockview handles, or remount terminal xterm
  surfaces unnecessarily during ordinary tab selection.
- Preserve existing pane body ReactNodes from `App.tsx`; do not rewrite terminal
  or read-only file pane internals except where sizing/style adaptation is
  required for Dockview.
- Remove or demote the old `WorkbenchEditorGroup` / `WorkbenchTabLane` render
  path so it is not the visible layout authority.
- Keep existing workbench registry, logical surface keys, placement policy, and
  command ids as the product-facing policy layer.
- If exact pinned/opened two-row presentation is not cleanly compatible with
  Dockview in this slice, choose the closest deterministic Dockview-compatible
  policy and document the deviation in the implementation report.

## References

- [Must] `ai-docs/tickets/ready/260517-bug-ws-dashboard-dockview-workbench-parity.md`
- [Must] `ai-docs/mental-model/ws-web-dashboard.md`
- [Must] `ai-docs/spec/ws-web-dashboard/index.md`
  - `260516-ws-web-dashboard-workroot-workbench-substrate`
  - `260516-ws-web-dashboard-readonly-text-pane`
  - `260516-ws-web-dashboard-file-open-placement-policy`
  - `260516-ws-web-dashboard-terminal-pane`
  - `260516-ws-web-dashboard-terminal-tab-selection-and-empty-initial-state`
  - `260516-ws-web-dashboard-browser-terminal-emulator-behavior`
  - `260516-ws-web-dashboard-browser-ui-acceptance-gate`
- [Must] `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx`
- [Must] `ws-dashboard/frontend/src/workbench/dockviewBridge.ts`
- [Must] `ws-dashboard/frontend/src/workbench/editorGroupModel.ts`
- [Must] `ws-dashboard/frontend/src/App.tsx`
- [Must] `ws-dashboard/frontend/src/styles.css`
- [Must] `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`
- [Maybe] `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts`
- [Maybe] `ws-dashboard/frontend/src/terminals.ts`
- [Maybe] `ws-dashboard/frontend/src/workRootFiles.ts`

## Verification

Run at minimum:

```text
cd ws-dashboard/frontend && npm run test:workbench
cd ws-dashboard/frontend && npm run test:work-root-files
cd ws-dashboard/frontend && npm run test:terminals
cd ws-dashboard/frontend && npm run build
cd ws-dashboard/frontend && npm run test:browser
```

The browser gate must fail if the old custom tab/split shell returns as the
visible workbench layout owner.
