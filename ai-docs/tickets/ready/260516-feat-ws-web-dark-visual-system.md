---
title: ws web dashboard dark visual system
parent: 260516-epic-ws-web-dashboard-workbench-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workbench-substrate: containing workbench substrate epic
  260514-research-ws-web-dashboard-direction: visual direction and dashboard UX research
spec:
  - 260516-ws-web-dashboard-dark-visual-system
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

### Phase 2: Shell Reskin And Visual Verification

Convert the current protected frontend shell to the new dark token baseline.
Preserve the existing resource API behavior, loading/error/stale states, and
command ids while making the shell visibly consistent with the guide.

Verify the updated shell with build checks and screenshot inspection across at
least one desktop and one narrow viewport.
