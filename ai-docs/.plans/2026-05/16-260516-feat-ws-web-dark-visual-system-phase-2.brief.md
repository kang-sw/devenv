# Brief: 260516-feat-ws-web-dark-visual-system Phase 2

## Intent

Finish the dark-first visual system child by ensuring the protected dashboard
shell uses the Phase 1 semantic token baseline and by collecting build plus
desktop/narrow screenshot verification evidence.

## Approach

- Inspect the existing frontend shell against `ws-dashboard/frontend/DESIGN.md`
  and `src/styles.css`.
- Make only the minimal shell/style adjustments needed for a coherent dark
  token baseline.
- Preserve the existing resource API fetch behavior, loading/error/stale
  states, command ids, and visible resource hierarchy semantics.
- Verify with `npm run build`.
- Run an inspectable browser screenshot check for one desktop viewport and one
  narrow viewport. Use fixture-backed or daemon-served dashboard data already
  available in the app; do not introduce new product behavior to make
  screenshots pass.

## Constraints

- Scope is Phase 2 only.
- Do not change daemon auth, routes, resource API contracts, or command ids.
- Do not implement workbench split groups, stable pairing routes, or live
  terminal/agent/editor/viewer behavior.
- Keep screenshots as verification artifacts; do not commit generated images
  unless a repository convention explicitly requires it.
- Prefer delegating screenshot inspection and reporting pass/fail plus concise
  observations rather than dumping image details into the main context.

## Out of scope

- Adding a screenshot test framework as a permanent dependency.
- Pixel-perfect final product polish.
- Free split manipulation UI.
- Stable token-free pairing route changes.

## Details

The screenshot verifier should check:

- the shell renders nonblank at desktop and narrow widths;
- dark canvas/panels/tokens are visible and coherent;
- text remains readable and does not overlap its containers;
- loading/error/stale visual affordances remain represented where visible;
- left navigation, detail area, and viewer/reserved area remain inspectable;
- no obvious light-mode panel remains from hardcoded component colors.

Accept manual Playwright or browser automation output as verification evidence
if it records viewport sizes and pass/fail observations.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-dark-visual-system`,
  `260516-ws-web-dashboard-protected-frontend-shell`,
  `260516-ws-web-dashboard-resource-view-model-contract`, and
  `260516-ws-web-dashboard-inspectable-navigation-shell`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard frontend
  ownership, protected shell, dark token contract, and resource shell coupling.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-dark-visual-system.md` -
  Phase 2 scope.
- [Must] `ws-dashboard/frontend/DESIGN.md` - visual-system guide.
- [Must] `ws-dashboard/frontend/src/styles.css` - current tokenized styling.
- [Maybe] `ws-dashboard/frontend/src/App.tsx` - class names, command ids, and
  resource shell behavior.
- [Maybe] `ws-dashboard/dev.sh` - build/run helper for daemon-served screenshots.
