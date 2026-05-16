---
title: ws web dashboard dark visual system
completed: 2026-05-16
parent: 260516-epic-ws-web-dashboard-workbench-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workbench-substrate: containing workbench substrate epic
  260514-research-ws-web-dashboard-direction: visual direction and dashboard UX research
spec:
  - 260516-ws-web-dashboard-dark-visual-system
plans:
  phase-1: 2026-05/16-260516-feat-ws-web-dark-visual-system-phase-1.brief
  phase-2: 2026-05/16-260516-feat-ws-web-dark-visual-system-phase-2.brief
related-mental-model:
  - ws-web-dashboard
---

# ws web dashboard dark visual system

## Background

The first visible dashboard shell is inspectable but still mostly a substrate
for information architecture. Before larger workbench surfaces are added, the
frontend should establish a dashboard-specific dark-first visual system so
later layout, editor, terminal, viewer, and utility panels share tokens,
density, and component treatment.

Use `ai-docs/ref/design.md` as a Carbon-inspired reference for density, square
corners, hairlines, and restrained operational UI. Do not apply that reference
as a light-mode palette by default.

## Decisions

- Create dashboard-specific guidance under `ws-dashboard/frontend/`, not a
  broad repo-level design system.
- Prefer semantic tokens over hardcoded component colors.
- Keep the first pass focused on the existing shell and reusable primitives;
  do not redesign final workbench content that does not exist yet.
- Verify with desktop and mobile screenshots so the dark shell is inspectable
  before deeper feature work depends on it.

## Phases

### Phase 1: Theme Guide And Tokens

Create `ws-dashboard/frontend/DESIGN.md` with the dashboard's dark-first
visual rules, including semantic color tokens, density guidance, border and
hairline treatment, typography scale, focus/selection states, and component
constraints for operational UI.

Add or normalize the frontend token layer so application components consume
semantic variables rather than scattered literal colors.

### Result (7af4d45) - 2026-05-16

Added `ws-dashboard/frontend/DESIGN.md` as the dashboard-local dark-first
visual guide and normalized `ws-dashboard/frontend/src/styles.css` around
semantic CSS variables for surfaces, text, borders, actions, focus, density,
and state treatment.

The implementation stayed within Phase 1: resource API behavior and command
identifiers were unchanged, and full screenshot verification remains Phase 2.
Verified with `cd ws-dashboard/frontend && npm run build`. Fit and test review
reported clean.

### Phase 2: Shell Reskin And Visual Verification

Convert the current protected frontend shell to the new dark token baseline.
Preserve the existing resource API behavior, loading/error/stale states, and
command ids while making the shell visibly consistent with the guide.

Verify the updated shell with build checks and screenshot inspection across at
least one desktop and one narrow viewport.

### Result (7c3856d) - 2026-05-16

Aligned the protected frontend shell's root and page surfaces with the dark
semantic baseline, including dark UA color-scheme and page canvas defaults.
The implementation preserved resource API behavior, command ids, hierarchy
rendering, and loading/error/stale component behavior.

Verified with `cd ws-dashboard/frontend && npm run build`. A delegated visual
verifier inspected desktop `1440x960`, narrow `390x844`, and tall narrow
`390x1800` screenshots from a frontend-only static build server with fixture
data. The verifier reported a nonblank coherent dark shell, readable text
without visible overlap, inspectable navigation/detail/viewer areas, and no
blocking visual findings. Fit and test review reported clean.
