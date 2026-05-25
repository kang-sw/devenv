---
title: Research ws dashboard visual design system refresh
related:
  260514-epic-ws-web-dashboard-mvp: dashboard MVP board that needs a coherent visual quality pass
  260524-research-ws-dashboard-react-aria-ui-primitives: primitive adoption research showed accessibility primitives do not solve visual design quality
  260524-feat-ws-dashboard-document-viewer-editor-substrate: future document/editor surfaces should not inherit the current rough visual language
related-mental-model:
  - ws-web-dashboard
---

# Research ws dashboard visual design system refresh

## Background

The root picker React Aria pilot improved interaction mechanics, accessibility
primitives, focus behavior, and keyboard collection handling, but it did not
make the dashboard visually polished. The dashboard still reads as an
implementation-shaped operational shell rather than a coherent product UI.

This feedback should be captured separately from React Aria adoption. React
Aria can supply accessible behavior primitives, but the dashboard's visual
quality depends on layout density, typography, spacing, borders, color roles,
interaction states, pane hierarchy, and a domain-appropriate component
vocabulary.

## Questions

- What visual standard should the dashboard target: dense operational control
  plane, IDE-like workbench, or another explicitly described direction?
- Which current surfaces are most damaging to perceived quality: left
  navigation, root picker, Dockview panes/tabs, Activity Console, file explorer,
  terminal chrome, editor/read-only views, or command/status surfaces?
- Which styling problems are systemic token/design-system issues versus
  isolated component CSS defects?
- Should the dashboard keep the current dark operational vocabulary and refine
  it, or define a new dashboard-local design guide before further UI feature
  work?
- What screenshot/browser evidence should gate future visual refresh work
  across desktop and constrained viewports?

## Current Hypothesis

- Keep React Aria, Dockview, and xterm as behavior/layout primitives where they
  fit, but do not treat any library as the visual design system.
- Define a dashboard-local visual guide before broad polishing: typography
  scale, spacing rhythm, border/shadow policy, focus/hover/selected states,
  list/table density, modal layout, sidebar hierarchy, and pane chrome rules.
- Prioritize operational clarity and scanability over marketing-style layout,
  large cards, decorative gradients, or oversized hero-like typography.
- Start with one coherent vertical slice that includes left navigation, root
  picker, workbench tabs, and a read-only/editor pane so future document UI has
  a better base.

## Evidence To Collect

- Current desktop and constrained viewport screenshots for the main dashboard
  surfaces after the React Aria root picker pilot.
- A short inventory of repeated visual problems in `frontend/src/styles.css`
  and component-specific CSS usage.
- Candidate visual direction references or mockups that match a personal
  developer control plane rather than a generic admin template.
- Browser-level before/after evidence for any future visual refresh ticket.

## Non-Goals

- Replacing React Aria, Dockview, or xterm solely for visual reasons.
- Redesigning dashboard behavior, command semantics, resource identity, or
  backend contracts.
- Landing a broad visual rewrite without a concrete visual standard and browser
  evidence.
