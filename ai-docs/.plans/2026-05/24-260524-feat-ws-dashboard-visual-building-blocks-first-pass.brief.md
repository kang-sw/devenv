# Brief: 260524-feat-ws-dashboard-visual-building-blocks-first-pass

## Intent

Make the dashboard read as a coherent dense workbench by defining reusable
dashboard-local visual building blocks and applying a first pass to the current
high-impact browser surfaces.

## Scope Boundary

Implement Phase 1 only. The pass may update `ws-dashboard/frontend/DESIGN.md`,
`ws-dashboard/frontend/src/styles.css`, and minimal React class composition in
the current frontend components. It must not change dashboard behavior,
commands, daemon APIs, resource identity, Activity data flow, terminal lifecycle,
or editor/document backend semantics.

## Caller-Visible Contract

The visible dashboard should keep the same controls and data, but use a clearer
dark operational visual grammar for:

- left navigation and open-workRoot chrome;
- workbench toolbar and Dockview tab chrome;
- Activity ribbon and transcript blocks;
- read-only text/document pane;
- common chips, buttons, rows, state surfaces, and empty/loading/error states.

## Contract Instructions

Keep React Aria, Dockview, and xterm in place. Preserve command ids and existing
click/input routes. Treat CSS primitives as dashboard-local visual vocabulary,
not as a new external design-system package.

Use or introduce reusable classes only when they reduce repeated visual rules or
make component roles clearer. Do not create broad DOM rewrites. Do not add
rounded cards, decorative gradients, heavy shadows, light-theme colors, or a
single-hue palette. Keep terminal fit and xterm full-bleed behavior intact.

## Integration Test Instructions

Run the frontend build and browser acceptance gate:

- `cd ws-dashboard/frontend && npm run build`
- `cd ws-dashboard/frontend && npm run test:browser`

Capture browser-level visual evidence at desktop and constrained viewports. The
existing Playwright gate is acceptable when it exercises the daemon-served
production frontend and screenshots cover the changed workbench surfaces.

## Implementation Strategy Decisions

- Refine `DESIGN.md` from token vocabulary into component building-block
  vocabulary.
- Keep most implementation in CSS, with minimal React class additions where a
  shared primitive needs to apply to an existing element.
- Use existing semantic tokens first; extend tokens only when a repeated visual
  role lacks one.
- Treat the root picker as optional consumer polish only; do not reopen picker
  interaction design.

## Rejected Alternatives

- Replacing React Aria, Dockview, or xterm for visual reasons.
- Redesigning dashboard behavior, command semantics, or backend routes.
- Implementing markdown rendering, translation overlays, edit mode, or document
  save fan-out in this pass.
- A broad visual rewrite without browser evidence.

## Approach

- Add building-block rules to `DESIGN.md`.
- Normalize semantic token names that are already implicitly used by existing
  CSS.
- Consolidate button, chip, row, pane, state, tab, transcript, document, and
  code-surface CSS.
- Apply the visual language to left nav, workbench chrome, Activity Console,
  read-only pane, and common state surfaces.
- Build and run the browser gate before closeout.

## Constraints

- No command id, API, route, resource id, Activity projection, terminal PTY, or
  layout persistence changes.
- No host paths or daemon-private details in newly introduced visible labels or
  evidence.
- Preserve responsive clipping and stable toolbar heights at constrained widths.
- Keep `DESIGN.md` and `styles.css` synchronized.

## Out of scope

- Dedicated markdown/document substrate.
- Translation, mention, pathref, edit, and save behavior.
- New backend or command model work.
- Dashboard-wide React Aria adoption.

## Details

Known relevant surfaces:

- `ws-dashboard/frontend/DESIGN.md`
- `ws-dashboard/frontend/src/styles.css`
- `ws-dashboard/frontend/src/App.tsx`
- `ws-dashboard/frontend/src/ActivityConsole.tsx`
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`

## Verification Contract

Required commands:

- `npm run build` from `ws-dashboard/frontend`
- `npm run test:browser` from `ws-dashboard/frontend`

Record whether browser evidence covers desktop and constrained viewport states.

## References

- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard UI, command,
  visual-system, Activity, read-only pane, terminal, and browser-gate rules.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - dark visual system and
  browser UI acceptance spec anchors.
- [Must] `ai-docs/tickets/ready/260524-feat-ws-dashboard-visual-building-blocks-first-pass.md`
  - selected phase scope and exclusions.
