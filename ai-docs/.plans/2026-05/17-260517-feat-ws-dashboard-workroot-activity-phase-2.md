# Survey: 17-260517-feat-ws-dashboard-workroot-activity-phase-2

## Reusable Components
- `ws-dashboard/frontend/src/workRootActivity.ts#L3-L64` — `WorkRootActivityView` and `fetchWorkRootActivity`: Phase 2 should consume this helper for selected workRoot activity; it already keeps browser code on the protected API route instead of ws cache files.
- `ws-dashboard/frontend/src/App.tsx#L977-L1123` — `WorkbenchShell`: owns selected-workRoot scoped workbench state/effects and computes the active `workbenchModel`; this is the closest place to hold activity fetch state keyed by `workbenchModel.root.id`.
- `ws-dashboard/frontend/src/App.tsx#L1596-L1608` — `WorkbenchToolbar` call site: passes the selected `root` and workbench actions into the top bar; activity badge data can be passed here without changing daemon/resource contracts.
- `ws-dashboard/frontend/src/App.tsx#L1685-L1781` — `WorkbenchToolbar`: renders breadcrumb, `.workbench-toolbar-meta`, and action buttons; the activity badge belongs in the existing meta row near `StateBadge`, kind, status, and last-command chips.
- `ws-dashboard/frontend/src/App.tsx#L2918-L2928` — `StateBadge`: compact status-chip pattern using existing dot/chip styling; useful for loading/error/unavailable activity badge states without long diagnostics.
- `ws-dashboard/frontend/src/styles.css#L494-L515` — `.meta-chip` / `.state-badge`: existing compact chip styling with max width, ellipsis, and nowrap text.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L366-L401` — owner pairing and real workRoot open flow: browser acceptance already opens a selected workRoot before layout assertions.

## Existing Patterns
- Selected resource flow: see `ws-dashboard/frontend/src/App.tsx#L220-L225` and `ws-dashboard/frontend/src/App.tsx#L2956-L2991` — selection resolves to a `WorkbenchSelection` even when a workspace/root/main instance is selected.
- Async route fetch state: see `ws-dashboard/frontend/src/App.tsx#L146-L168` — resource loading uses local loading/error state and bounded error strings.
- WorkRoot-scoped side effects: see `ws-dashboard/frontend/src/App.tsx#L1124-L1150` — terminal listing reacts to `workbenchModel?.root.id` and ignores failures without breaking the workbench.
- Toolbar metadata row: see `ws-dashboard/frontend/src/App.tsx#L1711-L1724` — existing top bar has one breadcrumb row and one `.workbench-toolbar-meta` row; adding a badge should not add a third row.
- Current toolbar density CSS: see `ws-dashboard/frontend/src/styles.css#L761-L820` — toolbar is a two-column grid with `min-height: 64px`; meta/actions currently wrap, so Phase 2 must check/adjust no-wrap behavior for the covered browser widths.
- Responsive toolbar behavior: see `ws-dashboard/frontend/src/styles.css#L1085-L1104` — below 960px toolbar becomes one column and below 560px toolbar children stack vertically; browser assertions should target the covered viewports in the gate.
- Browser viewport containment checks: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L403-L434` and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L610-L646` — existing Playwright tests measure document scroll/bounding boxes after UI changes.
- Browser artifact/evidence pattern: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L37-L40` and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L155-L174` — gate records concise notes and regenerable artifacts outside tracked source.

## Relevant Interfaces
- `ws-dashboard/frontend/src/workRootActivity.ts#L3-L9` — `WorkRootActivitySummary`: source counts for compact label formatting (`total`, `active`, `blocked`, `failed`, `unavailable`).
- `ws-dashboard/frontend/src/workRootActivity.ts#L39-L46` — `WorkRootActivityView`: status vocabulary includes `ok`, `unavailable`, and `degraded`; badge state should stay summary-level and not render row diagnostics.
- `ws-dashboard/frontend/src/workRootActivity.test.ts#L27-L74` — existing route/helper test file: can host pure formatting tests while keeping `npm run test:work-root-activity` as the Phase 2 unit gate.
- `ws-dashboard/frontend/package.json#L6-L17` — verification scripts: `test:work-root-activity`, `build`, and `test:browser` are already defined; `test:browser` builds production frontend and daemon before Playwright.
- `ws-dashboard/frontend/tsconfig.route-tests.json#L1-L26` — route-test compilation includes `workRootActivity.ts` and `workRootActivity.test.ts`; helper formatting must stay inside included files or update this list.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L209-L234` — reusable helpers for display names and Dockview assertion; additional toolbar assertions can sit near the open-workRoot/topbar steps.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L357-L364` — `documentScrolls`: existing helper for proving top-level viewport containment.

## Constraints
- `ai-docs/spec/ws-web-dashboard/index.md#L219-L232` — Phase 2 consumes the Phase 1 read-only projection; no browser ws cache access, agent controls, or running-command activity.
- `ai-docs/spec/ws-web-dashboard/index.md#L234-L242` — badge must live in the existing badge row and must not add toolbar height; constrained widths should compact/truncate/hide secondary text instead of wrapping.
- `ai-docs/spec/ws-web-dashboard/index.md#L271-L328` — visible frontend changes require `npm run test:browser` against daemon-served production frontend; pure TypeScript tests and Vite build alone are insufficient.
- `ai-docs/mental-model/ws-web-dashboard.md#L10-L12` — any visible dashboard UI change needs browser-level visual/interaction verification.
- `ai-docs/mental-model/ws-web-dashboard.md#L75-L80` — the browser shell should consume daemon APIs as source of truth and not make route params/resource ids authoritative.
- `ai-docs/mental-model/ws-web-dashboard.md#L154-L156` — new dashboard UI should use semantic dark tokens/square operational style, not raw decorative styling.
- `ws-dashboard/frontend/src/styles.css#L494-L510` — chips already use ellipsis and `white-space: nowrap`; activity text must remain short enough that these constraints are effective.

## Verification Commands
- `cd ws-dashboard/frontend && npm run test:work-root-activity`
- `cd ws-dashboard/frontend && npm run build`
- `cd ws-dashboard/frontend && npm run test:browser`
- `git diff --check`

## Opinion
- The smallest Phase 2 surface is frontend-only: add selected-workRoot activity fetch state in `WorkbenchShell`, pass a compact view model into `WorkbenchToolbar`, and test the label formatter through the existing `workRootActivity.test.ts` script.
- Playwright will likely need deterministic activity data. Since the default spawned fixture workRoot may have no named agents, route interception for `/api/dashboard/work-roots/*/activity` or fixture wsstate setup is needed to make a non-empty badge assertion stable.
- The current CSS allows `.workbench-toolbar-meta` and actions to wrap; preserving height under the browser gate likely requires explicit no-wrap/overflow behavior for the toolbar meta row and a measured before/after or bounded-height assertion in Playwright.
