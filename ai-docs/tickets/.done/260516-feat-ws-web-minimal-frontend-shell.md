---
title: ws web dashboard minimal frontend shell
parent: 260515-epic-ws-web-dashboard-first-visible-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260515-epic-ws-web-dashboard-first-visible-substrate: coordinating first visible substrate epic
  260516-feat-ws-web-resource-view-model-contract: required API and fixture contract
  260514-research-ws-web-dashboard-direction: source research for shell and navigation boundaries
spec:
  - 260516-ws-web-dashboard-protected-frontend-shell
  - 260516-ws-web-dashboard-inspectable-navigation-shell
skeletons:
  phase-1: 83e6e23
related-mental-model:
  - ws-web-dashboard
  - developer-environment-tools
completed: 2026-05-16
---

# ws web dashboard minimal frontend shell

## Background

The first visible substrate needs a real browser shell that can be inspected
with the agreed dashboard resource hierarchy. The frontend should validate
layout, density, status, selection, empty, loading, and error behavior without
pulling terminal, editor, named-agent, or document-viewer feature depth into
the first UI slice.

This ticket depends on the resource view-model API contract so the frontend
does not invent a separate mock shape.

## Decisions

- Build the first frontend as an operational dashboard surface, not a marketing
  page or static mock.
- Use the restrained visual system from `ai-docs/ref/design.md`: practical
  density, square geometry, hairline separators, and clear hierarchy.
- Render the honest data hierarchy while allowing singleton
  `workspace -> workRoot -> mainInstance` chains to appear as compact rows.
- Keep active instances visually distinct from inactive spawn targets.
- Route mouse actions and keyboard actions through command ids. Reserve `^b`
  to mean ctrl plus lowercase `b`.
- Leave full keybinding customization, PTY, editor, document viewer, bookmark
  CRUD, and broad filesystem watcher behavior out of scope.

## Phases

### Phase 1: Frontend package and protected asset serving

Introduce the React, TypeScript, and Vite frontend package and wire the daemon
to serve built assets behind the existing owner-auth boundary. Keep the
frontend build and daemon route layout simple enough that existing daemon auth
tests can verify unauthenticated rejection and paired-owner success.

Success criteria:

- The daemon can serve the built dashboard shell only after owner
  authentication.
- Development and production entrypoints are documented enough for later
  dashboard slices to run the shell locally.
- Static serving does not create an unauthenticated route beside `/pair`.

### Result (48c8a0c) - 2026-05-16

Implemented the protected frontend shell foundation. The repository now has a
React, TypeScript, and Vite package under `ws-dashboard/frontend/` with
`dev`, `build`, and `preview` scripts, lockfile-pinned dependencies, and a
minimal buildable shell. The daemon serves `ServeConfig.static_dir/index.html`
at `/` and `ServeConfig.static_dir/assets/*` at `/assets/*`, both under the
existing owner-auth protected router.

Static serving preserves `/pair` as the only unauthenticated browser route and
keeps the no-static-dir placeholder route for daemon development. Route tests
cover unauthenticated rejection and paired-cookie success for the built shell
fixture. Phase 2 navigation/detail/resource rendering remains unimplemented.

Verification: `npm run build`, `cargo test -p ws-dashboard-daemon --test
routes`, and `cargo test --workspace` passed. Correctness, fit, and test
reviews were clean.

### Phase 2: Inspectable navigation and detail shell

Render the first dashboard shell from the resource view-model API. The shell
should include a left navigation region, central instance/detail region, and
reserved right viewer region without implementing the deferred viewer feature.
It should show server, workspace, workRoot, main-instance, and sub-instance
state; loading, empty, stale, and error states; compact singleton rows; and a
small command registry behind visible mouse actions.

Success criteria:

- A human can visually inspect hierarchy, selection, status, and density from
  deterministic fixture data.
- UI text and controls fit at desktop and narrow browser sizes.
- Mouse actions use command ids so future keyboard bindings can call the same
  actions.

### Result (f550788) - 2026-05-16

Implemented the first inspectable dashboard browser shell over the authenticated
resource API. The React app now fetches `/api/dashboard/resources`, renders the
full `server -> workspace -> workRoot -> mainInstance -> subInstance`
hierarchy in the left navigation, displays selected resource detail in the
center panel, and reserves a right viewer panel without implementing viewer
feature depth.

The shell renders loading, empty, stale, error, compact singleton, selected,
disabled-action, and post-load refresh-failure states. Mouse-triggered row and
action controls carry command ids so a future keyboard layer can dispatch the
same command path instead of duplicating interaction behavior.

Verification: `npm run build`, `cargo test --workspace`, authenticated daemon
smoke checks for `/` and `/api/dashboard/resources`, and Playwright desktop and
mobile screenshots passed. Review found a post-load refresh failure visibility
bug; the final commit fixes it with an inline refresh-failure notice while
keeping stale data visible.
