# Brief: 260524-feat-ws-dashboard-icon-chrome-refinement

## Intent

Refine the dashboard's icon-first chrome after screenshot review so the left
navigation, workRoot topbar, and Activity ribbon read calmer and denser without
changing command routing, daemon APIs, workbench placement, or main pane bodies.

## Scope Boundary

Implement Phase 1 only: visual density, state treatment, readable labels, and
hover/focus icon chrome for the existing icon-first left navigation, file
explorer-adjacent shared icon buttons, workRoot topbar, overflow menu, and
WorkRoot Activity ribbon. Defer full visual-system redesign, markdown/editor
work, terminal body styling, root picker interaction changes, daemon/API
changes, and persistence changes.

## Caller-Visible Contract

Ready workspace/workRoot rows stay one-line and low-height without ready chips.
Compact resource glyphs do not visually overlap. Non-ready rows remain
visually identifiable through row tone, rail, dots, or compact cues. The
workRoot power control communicates online/offline state through symmetric icon
color rather than a filled blue action background. Topbar chips and overflow
menu labels preserve readable semantic text at normal desktop widths and hide
lower-priority content before truncating into fragments. WorkRoot Activity
ribbon items use bright text for the primary title and muted text for metadata.
Icon buttons are quiet by default and reveal border/highlight treatment on
hover, focus, active, selected, or destructive states while keeping accessible
names, titles, stable dimensions, and `data-command-id` values.

## Contract Instructions

Change `ws-dashboard/frontend/src/App.tsx`,
`ws-dashboard/frontend/src/styles.css`, and browser acceptance assertions only
as needed. Preserve existing dashboard command builders and dispatch paths for
`resource.select`, `workspace.remove`, `workRoot.activation.set`,
`dashboard.refresh`, `terminal.create`, `workbench.openActivity`,
`resource.action.*`, and `workbench.toggle.*`.

Use existing lucide icons and dashboard semantic tokens. Do not introduce new
icon libraries, daemon data, backend fields, persisted preferences, or mock-only
visual wiring. Interpret "glass-like" as quiet default chrome with subtle
hover/focus border or highlight; do not add decorative glass panels, gradients,
or heavy shadows that conflict with `ws-dashboard/frontend/DESIGN.md`.

## Integration Test Instructions

Extend `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` only when
assertions need to reflect the refined visible contract. Required boundary:
browser-level production frontend verification through the existing
`npm run test:browser` gate. The gate must still capture desktop workbench,
file explorer, topbar overflow, and constrained viewport screenshots, and it
must retain evidence that icon-only controls expose accessible names/titles and
stable command ids.

## Implementation Strategy Decisions

- Prefer CSS refinements over React state or data-model changes.
- Remove default ready chips from nav rows; keep unavailable/offline cues low
  height and title/tooltips informative.
- Replace overlapping compact paired glyphs with a non-overlapping fixed-width
  lockup or one clear compact glyph.
- Keep topbar overflow menu text readable by widening the menu, reducing menu
  font slightly, and avoiding ellipsis for short labels.
- Preserve toolbar height by prioritizing status/activity text and hiding
  secondary fragments before high-signal labels break.
- Preserve visible focus rings even when default borders are hidden.

## Rejected Alternatives

- Reverting to broad text buttons or metadata chip lists is out of scope.
- Making Activity, terminal, root picker, editor, or daemon behavior changes is
  out of scope.
- Adding a general menu framework, command palette, theme redesign, or persisted
  chrome preferences is out of scope.
- Decorative glassmorphism is out of scope; only hover-revealed operational
  chrome is intended.

## Approach

- Update shared icon-button CSS first so all icon-only controls get stable
  dimensions, quiet default state, hover/focus reveal, danger state, and
  color-only power variants.
- Update resource-row markup/styles to remove ready chips, keep fixed action
  sizing, and stop compact glyph overlap.
- Update topbar meta/menu styles so readable chip/menu text wins over
  low-priority content.
- Update Activity ribbon styles for inactive rail transparency and muted
  metadata hierarchy.
- Adjust browser assertions or evidence notes for the refined selectors and
  visual contract, then run the frontend build and browser gate.

## Constraints

- Do not alter daemon routes, resource view-model types, Activity read models,
  terminal behavior, Dockview layout persistence, or command payload shapes.
- Do not expose host paths, wsstate paths, session ids, process ids, or cache
  paths through new tooltips or labels.
- Keep rows, buttons, and toolbars stable in size across hover/focus/disabled
  states.
- Keep dashboard UI dark, dense, square, and semantic-token based.

## Out of scope

Main pane body redesign, Activity transcript body typography, terminal body
styling, read-only/markdown editor substrate, translation overlays, save
fan-out, generalized menus, root picker behavior, daemon/API/persistence work,
and full visual design-system replacement.

## Details

Normal ready rows should no longer render `StateBadge` as a chip. If a row is
not ready, a compact state badge or alert chip may appear only when it does not
force the default ready row to two lines. The remove action remains an
icon-only trash button with `workspace.remove` identity and fixed dimensions.
The topbar power button may carry data attributes or classes for online/offline
state as long as its command identity and accessible label remain unchanged.

## Verification Contract

- `cd ws-dashboard/frontend && npm run build`
- `cd ws-dashboard/frontend && npm run test:browser`
- Manual screenshot review of `desktop-workbench.png`, `file-explorer.png`,
  `topbar-overflow.png`, and `narrow-workbench.png` under
  `ws-dashboard/frontend/e2e/.artifacts/`.
- Confirm overflow menu is actually visible in the screenshot, not only present
  in the DOM.
- Confirm icon-only controls retain accessible names/titles and stable
  `data-command-id` routing.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260524-dashboard-icon-first-chrome`,
  `260516-ws-web-dashboard-dark-visual-system`, and
  `260516-ws-web-dashboard-browser-ui-acceptance-gate`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard visible-UI,
  command identity, Activity badge, and browser-gate rules.
- [Must] `ws-dashboard/frontend/DESIGN.md` - dashboard-local dark visual
  vocabulary and constraints.
- [Must] `ws-dashboard/frontend/src/App.tsx` - resource rows, topbar, overflow,
  and Activity pane wiring.
- [Must] `ws-dashboard/frontend/src/styles.css` - semantic tokens and existing
  icon-first chrome styles.
- [Must] `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` - browser
  acceptance gate and screenshot artifacts.
