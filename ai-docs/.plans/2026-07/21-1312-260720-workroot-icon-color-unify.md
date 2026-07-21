# Plan: 260720-idea-dashboard-workroot-icon-color-shape-inconsistency — Phase 1: unify base-root-with-worktrees icon color to single-root action color

## Relevant Ticket Contract
- Decided Direction (2026-07-21): keep the current icon **set** unchanged (no shape/glyph changes); only change the icon **color** of the `workspace`/`workRoot` main glyph (currently `.resource-row-icon`, `color: var(--ws-color-text-tertiary)`, `styles.css:2623-2630`) to match the `compactWorkRoot` case's color (`.resource-row-icon-compact`, `color: var(--ws-color-action)` = `#78a9ff`, `styles.css:2632-2634`, token defined `styles.css:37`).
- Reuse the existing `--ws-color-action` token as-is; do not invent a new blue value or new token.
- Success criterion: `workspace`/`workRoot` presentation main glyph and `compactWorkRoot` main glyph render in the same color; no other icon/color behavior changes.

## Out of Scope
- Icon shape/glyph selection in `ResourceGlyph` (`App.tsx:1542-1564`) — unchanged (`FolderOpen` for compact, `BriefcaseBusiness` for workspace, `FolderGit2` for workRoot stay as-is).
- `WorkRootKindIcon` badge (`App.tsx:1567-1575`, mounted `App.tsx:9615-9616`) and its color (`.resource-kind-glyph`, `var(--ws-color-text-disabled)`, `styles.css:2636-2638`) — unchanged.
- `resourceRowTone` border-left-color behavior (`App.tsx:9907-9923`, `.resource-row-ready` / `.resource-row-muted` / `.resource-row-error`, `styles.css:2598-2610`) — unchanged.
- Badge-not-shown-on-workspace-row inconsistency, main-glyph-ignores-`kind` inconsistency, and the broader three-competing-color-channels design question — explicitly deferred by the ticket's `## Non-Goals` to whoever owns `260524-research-ws-dashboard-visual-design-system-refresh`.

## Codebase Findings
- `ws-dashboard/frontend/src/styles.css#L2623-2630` — the single rule to edit. Combined selector `.resource-row-icon, .resource-kind-glyph { display: inline-flex; min-width: 0; align-items: center; justify-content: center; color: var(--ws-color-text-tertiary); }`. This sets the default color for both the main glyph wrapper (`.resource-row-icon`, used by `workspace` and `workRoot` presentations, `App.tsx:1561`) and the kind-badge wrapper (`.resource-kind-glyph`, `App.tsx:9615`).
- `ws-dashboard/frontend/src/styles.css#L2636-2638` — a later, equal-specificity rule `.resource-kind-glyph { color: var(--ws-color-text-disabled); }` that already overrides the shared rule above for the badge, by source order (both rules have specificity 0,1,0 for a `.resource-kind-glyph` element; later wins). This means changing the `color` value inside the L2623-2630 rule affects only `.resource-row-icon` in practice — `.resource-kind-glyph` keeps resolving to `--ws-color-text-disabled` regardless, so the badge color is unaffected by this edit. Confirms the fix is a single, contained CSS value change with no risk to the badge.
- `ws-dashboard/frontend/src/styles.css#L2632-2634` — `.resource-row-icon-compact { color: var(--ws-color-action); }`. Applied together with `.resource-row-icon` on the compact case only (`App.tsx:1551`: `className="resource-row-icon resource-row-icon-compact"`). After the L2623-2630 edit, this rule becomes a harmless redundant duplicate (both rules will state the same color for the compact case) — leaving it in place is safe and lowest-risk; removing it is optional cleanup, not required by the ticket's narrow scope.
- `ws-dashboard/frontend/src/styles.css#L37` — token definition `--ws-color-action: #78a9ff;`. Reuse this token reference; do not add a new token or hardcode the hex value.
- `ws-dashboard/frontend/src/App.tsx#L1543-1565` (`ResourceGlyph`) — confirms `.resource-row-icon` (no `-compact` modifier) is the class applied for both `workspace` (`App.tsx:1559`, `BriefcaseBusiness`) and `workRoot` (`App.tsx:1559`, `FolderGit2`) presentations. No `App.tsx` change is needed for Phase 1; this is a pure CSS-value edit.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L963-964` — existing e2e assertions check `.resource-row-icon-compact` visibility and `.resource-row-icon svg` count, not color values. This edit does not touch selector/DOM structure, so these assertions are unaffected. No test currently asserts on the tertiary/action color values themselves — no test file needs updating for this phase.

## Implementation Plan
1. In `ws-dashboard/frontend/src/styles.css`, inside the `.resource-row-icon, .resource-kind-glyph` rule (`L2623-2630`), change the `color` declaration from `var(--ws-color-text-tertiary)` to `var(--ws-color-action)`. This is the only line that needs to change.
2. Leave `.resource-row-icon-compact` (`L2632-2634`) and `.resource-kind-glyph` (`L2636-2638`) rules exactly as they are — they remain correct (now redundant-but-harmless for the compact case, and still authoritative for the badge case respectively).
3. Do not touch `App.tsx` (`ResourceGlyph`, `WorkRootKindIcon`, `resourceRowTone`) — no shape, badge, or border-tone logic changes per the ticket's decided direction.

## Verification Plan
- Manual/visual: run the dashboard dev server (`npm run dev` in `ws-dashboard/frontend`) and confirm a single-root workspace (`compactWorkRoot`) and a multi-root workspace (`workspace` parent + `workRoot` children) now show the same blue (`#78a9ff`) main-glyph color, with icon shapes and kind badges unchanged.
- `cd ws-dashboard/frontend && npm run build` — sanity check (`tsc -b && vite build`); this is a CSS-only change so no type errors are expected, but the build is cheap insurance the file still parses/bundles cleanly.
- Optional: `cd ws-dashboard/frontend && npm run test:browser` (builds, builds the daemon, runs Playwright including `e2e/dashboard-acceptance.spec.ts`) if a fuller regression pass is wanted; not required since no assertion in that spec checks the changed color value.

## Escalations
- None.
