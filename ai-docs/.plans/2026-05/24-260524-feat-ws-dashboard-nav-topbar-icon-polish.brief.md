# Brief: 260524-feat-ws-dashboard-nav-topbar-icon-polish

## Intent

Make the dashboard's left navigation, file explorer, and workRoot topbar read as
a calmer workbench chrome by replacing low-value text/chip clutter with
conventional icons, compact state cues, and a small overflow menu while
preserving the existing command model and main pane bodies.

## Scope Boundary

Implement Phase 1 only: icon-first left navigation and topbar polish. Leave
Activity transcript bodies, terminal bodies, read-only file pane bodies, root
picker interactions, markdown/editor work, file operations, keyboard shortcut
editing, and dashboard-wide React Aria migration out of scope.

## Caller-Visible Contract

The browser shell keeps the same resource selection, workRoot activation,
dashboard refresh, terminal creation, workspace removal, file explorer refresh,
file selection/opening, and placeholder workbench toggle behavior, but presents
the surrounding chrome with:

- conventional icon-first controls with accessible names and tooltips;
- visibly distinct left-column regions for open-workRoot, resource navigation,
  file explorer header, and file explorer body;
- compact resource rows that show resource type and state through icons, row
  tone, and high-signal badges rather than long metadata chip lists;
- generic file/folder icons in the file explorer while retaining existing
  disclosure/indent affordances;
- workRoot topbar layout of `power button | breadcrumb + high-signal chips |
  primary action icons | more menu`;
- overflow access for low-value or placeholder workbench toggles without
  removing their command ids.

## Contract Instructions

Change the React frontend only unless dependency metadata is needed.

- `ws-dashboard/frontend/src/App.tsx`
  - Import a conventional icon source, preferably `lucide-react`, for common
    chrome icons. Do not hand-roll generic refresh, folder, file, power, trash,
    terminal, more-menu, workspace, root, or activity icons when the library has
    a clear match.
  - Add a small icon button helper only if it reduces duplication while keeping
    each button's `data-command-id`, click handler, disabled state, `title`, and
    accessible name explicit.
  - Replace resource row text eyebrows such as `compact workRoot`,
    `workspace`, and `workRoot` with icon presentation. Compact workspace/root
    rows should communicate compression with paired icons.
  - Keep row selection routed through `resource.select`; keep workspace removal
    routed through `workspace.remove` and its existing confirmation behavior.
  - Reduce visible resource-row metadata chips to high-signal state only.
    Preserve `kind`, `availability`, `activation`, and other debug metadata in
    row titles or other low-visual-weight affordances.
  - Add file/folder icons to file explorer rows. Keep the existing tree
    disclosure/indent and row command ids.
  - Rework `WorkRootToolbar` so activation is a power icon at the left of the
    toolbar, breadcrumb/high-signal chips stay in the center, frequent actions
    are icon buttons, and low-value toggles remain command-routed behind a more
    menu.
  - Do not change Activity pane, terminal pane, read-only file pane, Dockview
    layout, root picker route semantics, resource API calls, or backend routes.
- `ws-dashboard/frontend/src/styles.css`
  - Extend the existing dashboard visual building blocks for icon buttons,
    compact resource rows, row tones, section backgrounds, file icons, and
    toolbar overflow.
  - Preserve dense square operational style, single-line toolbar metadata, file
    explorer scroll containment, terminal fit boundaries, and narrow viewport
    behavior.
- `ws-dashboard/frontend/package.json` and `package-lock.json`
  - Add `lucide-react` if it is not already available. Do not add a second UI
    framework for this pass.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`
  - Update browser assertions that currently expect old button text or chip
    text when the visible label becomes an accessible name/title. Preserve
    existing behavioral assertions for command routing, toolbar height, file
    explorer containment, Activity pane behavior, and terminal fit.

## Integration Test Instructions

Required boundary: browser UI acceptance plus frontend build/type verification.

- Run `cd ws-dashboard/frontend && npm run build`.
- Run `cd ws-dashboard/frontend && npm run test:browser`.
- If command routing tests fail because visible chrome changed, update only the
  affected frontend tests while preserving the existing command ids and payload
  assertions.
- Capture browser artifacts already produced by the acceptance gate and ensure
  desktop plus constrained viewport evidence covers left nav, file explorer,
  topbar, and the overflow menu state.

## Implementation Strategy Decisions

- Use icons as presentation only. Command ids, payload shapes, route identity,
  resource ids, persistence formats, and backend behavior remain unchanged.
- Prefer `lucide-react` over custom SVG for conventional symbols.
- Keep text where the icon alone is not conventional enough, but make common
  workbench actions icon-first.
- Demote low-value metadata visually instead of deleting it.
- Keep main pane bodies stable.

## Rejected Alternatives

- Do not treat React Aria as a visual styling solution for this pass.
- Do not replace Dockview, xterm, Activity Console, or the root picker.
- Do not turn the file explorer into a file manager or add file-type-specific
  icon theming in this phase.
- Do not use playful or brand-like symbols where operational symbols fit better.

## Approach

- Add the icon dependency and imports.
- Create small chrome helpers/classes for icon buttons and metadata titles.
- Update resource rows, file explorer rows, and `WorkRootToolbar` markup.
- Adjust CSS tokens/classes so section boundaries and icon controls read clearly
  without changing main pane bodies.
- Update browser assertions for accessible icon controls and overflow behavior.
- Run build and browser acceptance, then tune visible chrome from screenshots.

## Constraints

- Every icon-only action must have an accessible name and tooltip/title.
- Metadata row and toolbar must not wrap into extra rows at accepted widths.
- File explorer overflow must stay inside `.file-explorer-body`.
- Terminal/xterm fit and Activity transcript behavior must remain stable.
- Host paths, daemon-private paths, cache paths, session ids, and pairing data
  must not become visible through tooltips or metadata surfaces.

## Out of scope

- Markdown document UI, edit/read-only mode split, translation overlays,
  mention/pathref copying, save fan-out, and dedicated agents UI reuse.
- Full visual design system replacement.
- Generalized command palette, keyboard shortcut editor, or global menu system.
- Root picker UX changes beyond shared visual primitives if unavoidable.

## Details

The target topbar structure is:

```text
power button | breadcrumb + ready/activity chips | primary action icons | more menu
```

Primary icon actions should include open root, refresh, and new terminal when
enabled. Placeholder toggles such as viewer, task, diagnostics, events, and
layout should remain reachable through the more menu using their existing
`workbench.toggle.*` command ids.

## Verification Contract

- `npm run build` passes in `ws-dashboard/frontend`.
- `npm run test:browser` passes in `ws-dashboard/frontend`.
- Browser screenshots and evidence show icon-first left nav, file explorer
  icons, topbar power/action/more structure, and constrained viewport stability.
- The implementation commit references:
  - `260516-ws-web-dashboard-dark-visual-system`
  - `260524-dashboard-icon-first-chrome`
  - `260516-ws-web-dashboard-inspectable-navigation-shell`
  - `260516-ws-web-dashboard-workroot-file-explorer`
  - `260516-ws-web-dashboard-browser-ui-acceptance-gate`

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260524-dashboard-icon-first-chrome`,
  `260516-ws-web-dashboard-dark-visual-system`,
  `260516-ws-web-dashboard-inspectable-navigation-shell`,
  `260516-ws-web-dashboard-workroot-file-explorer`, and
  `260516-ws-web-dashboard-browser-ui-acceptance-gate`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard UI rules,
  command routing, file explorer, topbar, visual system, and browser gate.
- [Must] `ws-dashboard/frontend/src/App.tsx` - visible resource navigation,
  file explorer, topbar, command wiring, and workbench actions.
- [Must] `ws-dashboard/frontend/src/styles.css` - dashboard visual primitives
  and responsive chrome constraints.
- [Must] `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` - browser
  acceptance evidence for toolbar, file explorer, Activity, and terminal fit.
- [Maybe] `ws-dashboard/frontend/DESIGN.md` - dashboard-local design vocabulary.
- [Maybe] `ws-dashboard/frontend/src/commands.ts` - consult only if command ids
  need test updates; command shapes should not change.
