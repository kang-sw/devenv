---
title: Add dashboard visual building blocks first pass
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260524-research-ws-dashboard-visual-design-system-refresh: research capture that identified visual-system quality as separate from behavior primitives
  260524-research-ws-dashboard-react-aria-ui-primitives: React Aria primitive research remains behavior-focused, not a visual styling answer
  260524-feat-ws-dashboard-document-viewer-editor-substrate: future markdown and document panes should consume the same visual building blocks
spec:
  - 260516-ws-web-dashboard-dark-visual-system
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
related-mental-model:
  - ws-web-dashboard
---

# Add dashboard visual building blocks first pass

## Background

The dashboard now has a dark token baseline in `ws-dashboard/frontend/DESIGN.md`
and `frontend/src/styles.css`, but the visible UI still reads as an
implementation-shaped console rather than a coherent daily workbench. The
problem is not only color tokens: repeated components such as nav rows, toolbars,
chips, buttons, tabs, transcript blocks, and document panes do not yet share a
clear visual grammar.

This ticket turns the visual refresh research into an actionable first pass:
define reusable dashboard-local style building blocks and apply them to the
current high-impact surfaces without changing behavior.

## Decisions

- Target a dense IDE/workbench-like operational control plane. The dashboard
  should be calm, compact, and scannable for long-running sessions, not a
  marketing page, generic admin template, or card-heavy layout.
- Treat React Aria, Dockview, and xterm as behavior/layout primitives that
  remain in place. The visual layer should wrap and normalize them instead of
  replacing them for aesthetic reasons.
- Preserve the existing dark-first, square-corner, hairline-driven direction.
  The first pass may refine tokens and component rules, but it must not switch
  to rounded cards, gradients, decorative shadows, light-theme defaults, or a
  one-note accent-color palette.
- Establish reusable building blocks before polishing isolated components:
  frame, panel, pane, toolbar, action button, icon button, segmented control,
  chip, badge, state dot, list row, resource row, detail row, tab, transcript
  block, code block, document surface, empty/loading/error/stale state.
- Prefer progressive CSS/class consolidation over broad React rewrites. DOM
  changes are acceptable only when they clarify component roles or remove
  duplicated chrome without touching command behavior.
- Let this pass prepare the visual base for the dedicated markdown/document UI,
  but do not implement document rendering, translation overlays, editing, or
  save behavior in this ticket.

## Constraints

- Do not change command ids, command payload identity, daemon routes, resource
  ids, workRoot activation behavior, file APIs, Activity APIs, terminal
  lifecycle, or Dockview layout persistence semantics.
- Do not replace React Aria, Dockview, xterm, or the current workbench adapter
  as part of this visual pass.
- Do not let visual chrome around xterm break terminal fit, focus, input, or
  resize behavior.
- Keep host paths and daemon-private details out of newly introduced visual
  labels, tooltips, screenshots, and test evidence.
- Keep `DESIGN.md`, semantic tokens, and component CSS synchronized so future
  UI work has a recoverable visual contract.

## Phases

### Phase 1: Visual building blocks and current-surface first pass

Define the dashboard-local visual building blocks and apply them to the current
high-impact UI surfaces.

The implementation should:

- Update `ws-dashboard/frontend/DESIGN.md` with the reusable building block
  vocabulary, component roles, and composition rules that future UI surfaces
  should follow.
- Refine `frontend/src/styles.css` tokens only where needed to support the
  building blocks. Keep semantic names and avoid raw color literals inside
  feature components when a token exists.
- Introduce or consolidate primitive/component CSS classes for panels, panes,
  toolbars, buttons, chips, state indicators, list rows, tabs, transcript
  blocks, document/code surfaces, and state surfaces.
- Apply the first pass to the left navigation, open-workRoot area, workbench
  toolbar, Dockview tab chrome, Activity ribbon, transcript blocks, read-only
  text/document pane, and the most visible empty/loading/error states.
- Treat the root picker as a consumer of the new visual language when the
  change is low-risk, but do not reopen its interaction model or backend
  behavior.
- Preserve responsive behavior at desktop and constrained widths, including
  clipped metadata rows and stable toolbar heights.

Completion means the dashboard has a visible, reusable visual grammar that the
next document/editor work can consume, even if a later ticket continues deeper
surface-by-surface polish.

Deferred scope:

- Dedicated markdown rendering, paragraph actions, translation overlays,
  pathref copying, edit mode, and save fan-out.
- Broad root picker interaction changes, file-manager operations, or picker
  persistence changes.
- Terminal UX redesign beyond preserving a visually coherent pane boundary.
- Dashboard-wide React Aria adoption or removal.
- New backend APIs, command model changes, and resource model changes.

Verification should include:

- Frontend build/type verification appropriate for the touched files.
- Browser-level visual/interaction verification against the daemon-served
  production frontend at desktop and constrained viewports.
- Screenshot or equivalent browser evidence that covers the left nav, workbench
  toolbar/tabs, Activity Console, and read-only pane after the first pass.
- Existing browser acceptance coverage must still pass or any failure must be
  explained with a concrete visual-only limitation.
