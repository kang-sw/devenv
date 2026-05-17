---
title: ws dashboard Dockview workbench parity
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260516-epic-ws-web-dashboard-workbench-substrate: promised a Dockview-backed workbench substrate behind dashboard-owned policy
  260516-epic-ws-web-dashboard-workroot-io-substrate: added live file and terminal panes that must survive the substrate correction
  260516-bug-ws-web-dashboard-ui-acceptance-recovery: restored baseline usability but did not prove Dockview was the visible layout owner
  260517-feat-ws-dashboard-workbench-tab-polish: must run after this corrective substrate parity work instead of polishing the custom tab engine
spec:
  - 260516-ws-web-dashboard-workroot-workbench-substrate
  - 260516-ws-web-dashboard-readonly-text-pane
  - 260516-ws-web-dashboard-file-open-placement-policy
  - 260516-ws-web-dashboard-terminal-pane
  - 260516-ws-web-dashboard-terminal-tab-selection-and-empty-initial-state
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
skeletons:
  phase-1: 3c75198
  phase-3: 3c75198
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-17
---

# ws dashboard Dockview workbench parity

## Background

The dashboard workbench spec and prior discussion treated Dockview as the
mechanical layout substrate behind a dashboard-owned policy layer. The current
visible workbench does not actually mount Dockview as the layout owner. It keeps
the `dockview` dependency and a small bridge/test surface, but renders the
product workbench through custom React tab lanes, CSS grid splits, and HTML5
drag/drop.

That breaks a settled implementation premise and makes further tab polish risky:
close buttons, preview tabs, insertion policy, split resize, and drag/drop would
otherwise be implemented on top of a custom tab engine that should not be the
long-term substrate. This ticket corrects the substrate first while preserving
the user-visible behavior already recovered for file panes and terminal panes.

## Failure Analysis

- `32d60940` added the `dockview` dependency and bridge contracts, but it did
  not mount Dockview in the visible frontend tree.
- The Phase 2 workbench substrate brief allowed a dashboard-owned React/CSS
  split shell, so `ef871a3d` introduced the visible custom workbench shell
  without treating the missing Dockview mount as a blocker.
- Later chrome and movement work entrenched that shell: `a9f7124d` introduced
  compact dashboard-owned tabs, `bb6b4240` added HTML5 drag/drop tab movement,
  and `92d642a0`/`c79d8a79` kept refining custom split/tab state.
- Review and tests optimized for visible behavior and policy boundaries, not
  substrate identity. Existing bridge tests use fake Dockview ports, while
  workbench model tests validate the custom tab/move model.
- Browser acceptance verified terminal and file-explorer behavior after UI
  recovery, but it did not assert that Dockview owned the rendered workbench
  groups, tabs, and pane layout.

## Decisions

- Dockview must become the visible workbench layout owner, not only a dependency,
  test double, or unused adapter contract.
- The dashboard still owns product policy. Dockview APIs must stay behind a
  dashboard bridge/registry layer, and raw Dockview panel or group handles must
  not become product-level capabilities.
- Preserve functional parity before adding new tab polish. This ticket is not
  the place to add preview-to-pinned file tabs, richer editor features, or a
  configuration tab.
- Do not keep the custom tab/split engine as a second authoritative layout
  system after Dockview parity is complete. Transitional helpers are acceptable
  only when they feed or adapt Dockview state.
- Browser-level evidence must prove both user-visible behavior and the
  substrate premise: the rendered workbench must be Dockview-backed.

## Phases

### Phase 1: Mount Dockview as the workbench layout owner

Replace the current visible workbench split/tab shell with a Dockview-mounted
workbench surface behind the existing dashboard policy boundary. The first
reviewable result should show that primary/support split groups, active pane
selection, tab labels, and pane bodies are rendered through Dockview rather than
through the custom `WorkbenchEditorGroup`/`WorkbenchTabLane` layout.

Keep the dashboard registry, logical surface keys, and placement policy as the
product-facing layer. If Dockview needs custom headers or component adapters to
preserve the pinned/opened presentation, implement them as Dockview-facing
adapters rather than as a parallel authoritative tab system.

Success means inspection of the render tree and code confirms Dockview is the
layout authority for workbench groups/tabs/panes, while the dashboard still owns
surface identity, duplicate-open focusing, close policy, and placement intent.

### Result (5633cc23) - 2026-05-17

The visible workbench now mounts Dockview as the layout owner under the stable
`data-workbench-layout-owner="dockview"` marker. The retired custom
`WorkbenchEditorGroup`/`WorkbenchTabLane` render path and old split/tab CSS were
removed from the visible workbench authority. Dashboard state still owns surface
identity, placement, selection, close policy, and logical group intent.

The implementation uses a Dockview-compatible flattened tab policy while keeping
pane category metadata available for later tab polish. Raw Dockview handles stay
inside the adapter boundary.

### Phase 2: Preserve file and terminal pane parity

Reattach existing pane contents to the Dockview-backed shell without regressing
the recovered user-facing behavior. Read-only file panes must still open from
the file explorer, load daemon-authorized content, focus duplicates, and render
only under their owning workRoot. Terminal panes must still create live daemon
sessions, use xterm.js, connect through the WebSocket path, keep usable input
fidelity, fill the pane, preserve bottom-row visibility, and terminate the
daemon session on close.

Preserve the current two-group default: durable agent or persistent terminal
surfaces prefer the primary/focused group, while support surfaces such as file
or detail panes prefer the support group. If exact pinned/opened visual rows
cannot be represented cleanly through Dockview, record the closest Dockview
compatible policy and keep the behavior deterministic.

Success means current file-open and terminal browser interactions pass on the
Dockview-backed shell, and the implementation no longer depends on the custom
CSS grid split and tab lane as the authoritative workbench layout.

### Result (5633cc23) - 2026-05-17

Read-only file panes and terminal panes were reattached to the Dockview-backed
shell without regressing browser behavior. Terminal xterm surfaces are kept
stable under Dockview, WebSocket output is streamed into the mounted emulator,
and fallback keyboard routing is scoped to the single actually focused terminal
pane. Follow-up review fixed focus clearing for non-terminal selection and
macOS edit shortcut handling so input is not broadcast or stolen by inactive
terminals.

Verification passed for workbench, workRoot file, terminal, build, and browser
acceptance gates on the final branch head.

### Phase 3: Add substrate assertions and visual acceptance

Extend automated verification so this failure mode cannot recur. Tests must
distinguish between "tabs visually appear" and "Dockview owns the visible
workbench layout." Add focused model/unit coverage for the dashboard bridge and
browser-level evidence against the daemon-served production frontend.

The browser gate should cover, at minimum, owner pairing, opening a workRoot,
opening a read-only file pane, creating and selecting terminal panes, switching
tabs, moving or resizing panes where Dockview exposes the interaction, and
checking that the terminal still fits the visible pane. Include explicit
evidence that the visible workbench is Dockview-backed, such as a stable test
attribute or Dockview DOM marker owned by the dashboard adapter.

Success means `npm run test:browser` or an equivalent recorded browser gate
would fail if the workbench silently returned to a custom non-Dockview tab/split
implementation.

### Result (5633cc23) - 2026-05-17

Browser acceptance now asserts the Dockview owner marker, Dockview DOM beneath
that marker, and absence of the retired `.workbench-splits > .workbench-group`
custom shell. The final lead-run verification passed:

- `npm run test:workbench`
- `npm run test:work-root-files`
- `npm run test:terminals`
- `npm run build`
- `npm run test:browser`
