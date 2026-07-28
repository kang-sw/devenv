---
title: Dashboard-local design guide (tokens + primitives), seeded by the git widget
sage-review-design: required
parent: 260725-epic-ws-dashboard-git-panel
related:
  260524-research-ws-dashboard-visual-design-system-refresh: origin research whose hypothesis this actions; absorb once the guide lands
related-mental-model:
  - ws-web-dashboard
---

# Dashboard-local design guide (tokens + primitives), seeded by the git widget

## Background

The dashboard's current visual language reads as an implementation-shaped
operational shell, not a coherent product UI, and the only design reference in
the repo (`ai-docs/ref/design.md`) is an **IBM Carbon marketing-site
extraction** — white canvas, hero headlines, CTA banners — wrong for a dark
operational workbench. The owner wants to establish a real dashboard-local design
grammar now, using the interaction-heavy git widget as the first coherent
vertical slice to define and prove it.

This actions the hypothesis already recorded in
`260524-research-ws-dashboard-visual-design-system-refresh` ("define a
dashboard-local visual guide before broad polishing; start with one coherent
vertical slice"). That research is the origin; this ticket produces the artifact.

## Decisions

- **Owned by the git-panel epic**, not `260710`, because the owner chose to birth
  the standard from the git widget (the diff view especially). The guide is
  nonetheless dashboard-wide in intent.
- **Replace `ai-docs/ref/design.md`** with a dashboard-local design guide. The
  Carbon-marketing doc is retired (preserve historically if useful, but it is not
  the dashboard standard).
- **Scope of the guide:** typography scale, spacing rhythm, color roles (dark
  operational palette), border/elevation policy, focus/hover/selected interaction
  states, list/table density, pane/tab chrome, and the handful of primitives the
  git widget needs (rows, gutters, badges, toolbar controls). Deliberately
  operational-clarity-first, not marketing-style.
- **Seeded, not big-bang.** The guide starts from the concrete needs of the git
  tab / log graph / diff view and grows; it does not attempt to restyle every
  existing surface (that would collide with `260710`'s polishing backlog).

## Constraints

- Do not treat React Aria / Dockview / xterm as the design system (per
  `260524-research` non-goals); they are behavior/layout primitives.
- The guide must cover the dark operational vocabulary the dashboard already uses;
  it refines rather than importing a light marketing palette.
- Existing tokens live in `frontend/src/styles.css` (`--ws-color-*` etc.);
  reconcile the guide with (and clean up) that surface rather than forking a
  parallel token set.

## Phases

### Phase 1: Author the dashboard-local design guide seeded by the git widget

Produce the guide doc (replacing `ref/design.md`) capturing tokens + primitives,
derived from the git tab / log graph / diff view components as they are built.
Reconcile with `frontend/src/styles.css` tokens. Establish the primitives the
sibling git children consume (row, gutter, badge, toolbar control, tab chrome).

Verification boundary: the guide exists as the repo's design reference (old
Carbon doc retired), and the git-panel components reference its tokens/primitives
rather than ad-hoc CSS. Coordinate with `260524-research` closure.

## Spec Impact

Target spec area: none in the workflow spec set — this is a downstream
ws-dashboard documentation/visual artifact, not a workflow-system contract. The
mental-model `ws-web-dashboard` may reference the new guide on landing.

Contract-first spec: no.
