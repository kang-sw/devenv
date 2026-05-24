---
title: Refine dashboard icon chrome density, states, and hover treatment
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260524-feat-ws-dashboard-nav-topbar-icon-polish: first icon-first chrome pass; this ticket refines visual density and state treatment after screenshot review
  260524-feat-ws-dashboard-visual-building-blocks-first-pass: reusable CSS primitive layer this pass should continue to consume
  260524-research-ws-dashboard-visual-design-system-refresh: broader visual-system research remains separate from this tactical refinement
spec:
  - 260524-dashboard-icon-first-chrome
  - 260516-ws-web-dashboard-dark-visual-system
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
related-mental-model:
  - ws-web-dashboard
---

# Refine dashboard icon chrome density, states, and hover treatment

## Background

The first icon-first chrome pass reduced the dashboard's text-heavy navigation
and topbar, but screenshot review found several second-order visual issues:
compact resource glyphs overlap, ready chips still thicken workspace rows,
icon buttons read as permanently boxed controls instead of quiet workbench
chrome, some topbar/menu text truncates too aggressively, and Activity ribbon
rows need clearer inactive/active hierarchy.

This ticket captures a focused refinement pass for the next session. It should
keep the existing icon-first direction and command model, but tune density,
state color, text preservation, and hover/focus treatment so the UI feels less
heavy while remaining accessible.

## Decisions

- Keep the icon-first chrome direction from the previous ticket. Do not revert
  to broad text buttons or metadata chip lists.
- Treat this as a visual refinement pass over the existing frontend behavior.
  Command ids, payloads, daemon routes, resource ids, Activity read models,
  terminal behavior, file explorer data, root picker behavior, and Dockview
  layout persistence remain unchanged.
- Prefer "quiet by default, visible on hover/focus/active" control chrome:
  icons should not look like always-on boxed buttons unless selected, active,
  destructive, or focus-visible.
- Preserve readable text where text is the point of the UI, especially menu
  labels and high-signal topbar chips.

## Phases

### Phase 1: Refine icon chrome density and state presentation

Polish the visible chrome introduced by the icon-first pass without changing
the dashboard's behavior or main pane bodies.

The implementation should:

- Fix compact workspace/workRoot glyph rendering. The current paired icon
  treatment visually overlaps; replace it with either one clear compact glyph
  or a fixed-width parent/child lockup where the two symbols do not collide.
- Remove ready chips from workspace navigation rows. Ready state should be
  conveyed through row tone, state rail, small status dot, or another low-height
  indicator instead of a chip that creates a second row.
- Keep non-ready states visually clear without chip clutter. Offline and
  unavailable rows may use muted/error/degraded row tone and, only when needed,
  a compact text cue that does not thicken the default ready row.
- Keep workspace remove/trash as a fixed-size icon-only action. It must not
  stretch vertically or horizontally with row height, and it must keep the
  existing `workspace.remove` command path and confirmation behavior.
- Change the workRoot topbar power button to color-only state treatment:
  online and offline states should be symmetric through icon color, such as
  green and red/muted red, without a filled blue action background.
- Preserve the topbar overflow menu direction, but prevent menu labels from
  truncating for short commands such as refresh workspace and remove
  workspace. Increase menu width or layout space as needed, and reduce menu
  font size by roughly ten percent.
- Preserve high-signal topbar chip text. If Activity/status chips remain
  visible, labels like agent counts and failed counts should keep their
  semantic units instead of rendering as fragments such as `46 a...` or
  `2 f...`. Hide lower-priority content before truncating high-signal chip
  labels into unreadable fragments.
- Fix WorkRoot Activity ribbon inactive rail rendering. The left highlight rail
  may remain as a selected/active affordance, but its inactive alpha should be
  zero so it does not obscure the normal border.
- Tune WorkRoot Activity ribbon text hierarchy. In the three-line activity
  label, keep the primary/title line bright, but render the upper and lower
  metadata lines in secondary or tertiary gray so every line does not compete
  at full white contrast.
- Move dashboard icon buttons toward a glass-like hover treatment:
  default controls should be transparent or very low-emphasis with no visible
  permanent border; hover/focus should reveal a subtle glossy border or
  highlight; active/selected/destructive states may use restrained color but
  should avoid heavy filled surfaces unless there is a specific selected state.
- Preserve focus-visible rings clearly even when default borders are hidden.
- Keep icon button dimensions stable so hover states, row labels, and row
  actions do not resize or shift layout.

Completion means screenshot review shows a calmer dashboard chrome: compact
rows stay one-line and low-height, ready state no longer appears as a chip,
power state reads through icon color rather than fill, overflow/menu labels are
readable, topbar chips preserve meaningful text, Activity ribbon hierarchy is
less visually loud, and icon buttons reveal chrome primarily on hover/focus.

Deferred scope:

- Full visual design system redesign or theme replacement.
- Main pane body redesign, Activity transcript body typography, terminal body
  styling, read-only/markdown editor work, translation overlay work, or save
  fan-out.
- New command palette, keyboard shortcut editor, or generalized menu framework.
- Root picker interaction changes beyond incidental shared icon-button styling.
- Daemon/API/persistence changes.

Verification should include:

- `cd ws-dashboard/frontend && npm run build`
- `cd ws-dashboard/frontend && npm run test:browser`
- Browser screenshots covering desktop workbench, file explorer, topbar
  overflow menu, and constrained viewport.
- Manual screenshot review that the overflow menu is actually visible, not only
  present in the DOM.
- Evidence that icon-only controls retain accessible names/titles and stable
  `data-command-id` routing.
