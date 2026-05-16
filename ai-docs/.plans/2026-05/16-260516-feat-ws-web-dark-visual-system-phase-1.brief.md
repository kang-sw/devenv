# Brief: 260516-feat-ws-web-dark-visual-system Phase 1

## Intent

Establish the ws dashboard frontend's dark-first visual system contract before
larger workbench surfaces are implemented. This phase creates the frontend
design guide and introduces a semantic token layer that later components can
consume without hardcoding light-mode values throughout the component tree.

## Approach

- Add `ws-dashboard/frontend/DESIGN.md` with dashboard-specific dark-first
  visual rules.
- Derive the rules from `ai-docs/ref/design.md` as a Carbon-inspired reference
  for density, square corners, hairlines, restrained operational UI, and
  practical component states.
- Normalize `ws-dashboard/frontend/src/styles.css` so it exposes semantic
  custom properties for color, typography, spacing, borders, focus, and state
  treatment.
- Keep the current shell behavior and layout intact. This phase defines tokens
  and guidance; the full shell reskin and screenshot pass belongs to Phase 2.

## Constraints

- Scope is Phase 1 only.
- Do not redesign the dashboard shell, change resource API behavior, or alter
  command ids.
- Do not implement the future workbench split-group UI.
- Do not apply `ai-docs/ref/design.md` as a light palette.
- Keep the visual system dashboard-local under `ws-dashboard/frontend/`.
- Prefer semantic token names that can support later terminal, agent, editor,
  viewer, diagnostics, and task surfaces.

## Out of scope

- Desktop/mobile screenshot verification.
- Complete shell reskin.
- Dockview/FlexLayout workbench substrate.
- Pairing redirect or route identity changes.
- Live terminal, agent, editor, viewer, task, or diagnostics implementation.

## Details

`DESIGN.md` should cover:

- product intent and dark-first stance;
- token vocabulary for canvas, panels, borders, text, action, state, and focus;
- typography and density guidance suitable for operational UI;
- component rules for buttons, headers, navigation rows, badges, notices, and
  future split-group rows;
- constraints against rounded card-heavy marketing layouts, gradients, shadows,
  and light-mode defaults.

`styles.css` should expose semantic CSS variables at `:root`. Existing rules
may continue to use the current light visual treatment only where necessary for
Phase 2, but token definitions should be dark-first and ready for component
adoption.

Verification should at least run the frontend build command after token changes.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-dark-visual-system` planned behavior for this
  feature.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard frontend
  ownership, auth boundary, and browser shell contracts.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-dark-visual-system.md` -
  Phase 1 scope and acceptance boundary.
- [Must] `ai-docs/ref/design.md` - Carbon-inspired density, square-corner,
  hairline, typography, and restrained UI reference.
- [Must] `ws-dashboard/frontend/src/styles.css` - existing frontend style
  entrypoint and token target.
- [Maybe] `ws-dashboard/frontend/src/App.tsx` - consult only to verify class
  names and avoid changing shell behavior.
- [Maybe] `ws-dashboard/frontend/package.json` - frontend build command.
