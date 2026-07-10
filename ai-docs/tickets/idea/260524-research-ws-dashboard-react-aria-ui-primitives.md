---
title: Research ws dashboard React Aria UI primitives
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
related:
  260524-feat-ws-dashboard-react-aria-root-picker-pilot: root picker pilot should provide the first concrete evidence before broader adoption
  260524-feat-ws-dashboard-document-viewer-editor-substrate: future document/editor surfaces may need shared dialog, toggle, menu, tooltip, and collection primitives
related-mental-model:
  - ws-web-dashboard
---

# Research ws dashboard React Aria UI primitives

## Background

The dashboard has started to need richer browser interaction primitives than
locally authored markup can comfortably provide. The root picker modal proved
the backend and command contracts, but its local UI feels awkward and lacks the
interaction quality expected from a folder picker. Future document/editor
surfaces will add mode switches, block actions, translation overlays, menus,
tooltips, dialogs, and possibly richer lists.

React Aria Components is a plausible candidate because it offers accessible
modal/dialog and collection primitives while leaving visual styling under the
dashboard's existing token system. This research should not treat maintainer
preference for a better UI as evidence that React Aria should replace all local
UI. The root picker pilot should provide concrete integration evidence first.

## Questions

- Which React Aria primitives fit dashboard surfaces without weakening the
  dashboard command model?
- Does React Aria Modal/Dialog integrate cleanly with Dockview, xterm focus, and
  the daemon-served browser acceptance gate?
- Does React Aria GridList provide the right semantics for details-style folder
  rows with selection, row actions, keyboard navigation, and future metadata
  columns?
- What is the package and bundle impact of adding React Aria Components to the
  current React 19/Vite frontend?
- Which primitives, if any, should remain local or use a smaller library such as
  Radix UI instead?
- What conventions should govern controlled React Aria state, command dispatch,
  path-bearing UI keys, and path-free loggable command payloads?

## Current Hypothesis

- Keep `commands.ts` as the authoritative dashboard action contract.
- Treat React Aria as a controlled UI primitive layer: dashboard state flows
  into React Aria props, and React Aria events are adapted into dashboard
  commands.
- Permit host paths as local UI keys or authenticated request arguments, but
  never as loggable dashboard command payload fields.
- Prefer a narrow root picker pilot before declaring React Aria a dashboard-wide
  standard.
- Avoid broad React Aria plus Radix overlap unless the ownership split is
  explicit and evidenced.

## Evidence To Collect

- Package/version and bundle impact after adding the candidate React Aria
  package.
- Browser evidence for modal open/close focus, focus restore to opener, Escape
  handling, GridList keyboard selection/action, Dockview focus preservation, and
  xterm focus preservation.
- Implementation evidence that root picker UI keys containing paths do not leak
  into command payloads, command logs, persisted UI state, or browser-visible
  resource identities.
- Visual evidence that the dashboard token system can style React Aria
  primitives into a compact operational UI without a one-off visual subsystem.

## Non-Goals

- Replacing Dockview.
- Replacing xterm or terminal raw input handling.
- Redesigning the whole dashboard visual system in one pass.
- Converting every existing dashboard control to React Aria before a pilot has
  produced evidence.
- Changing backend root picker, open-workRoot, or workRoot resource contracts.
