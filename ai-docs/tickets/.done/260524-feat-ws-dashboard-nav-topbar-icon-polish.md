---
title: Polish dashboard navigation and topbar with icon-first chrome
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260524-feat-ws-dashboard-visual-building-blocks-first-pass: completed visual primitive layer this ticket should consume rather than replace
  260524-research-ws-dashboard-visual-design-system-refresh: research capture for dashboard visual quality and dense workbench direction
  260524-feat-ws-dashboard-document-viewer-editor-substrate: later document/editor work should inherit the improved chrome instead of solving global navigation polish
spec:
  - 260516-ws-web-dashboard-dark-visual-system
  - 260524-dashboard-icon-first-chrome
  - 260516-ws-web-dashboard-inspectable-navigation-shell
  - 260516-ws-web-dashboard-workroot-file-explorer
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-24
---

# Polish dashboard navigation and topbar with icon-first chrome

## Background

The first visual building-block pass created shared CSS vocabulary, but it did
not materially change the visible information structure. The dashboard still
shows too many text buttons and metadata chips in the left navigation and
topbar. The result remains visually noisy: rows mix resource identity,
debug-style metadata, state, and actions at the same weight, while the topbar
looks like a linear button array rather than an IDE/workbench command surface.

This ticket applies a stronger, caller-visible polish pass to the dashboard
chrome. It should reduce text clutter, introduce conventional icons, move
secondary or inactive commands behind an overflow menu, and make resource and
state hierarchy visible without changing dashboard behavior.

## Decisions

- Use an icon-first UI grammar for navigation and toolbar chrome. Prefer a
  conventional icon when the action or resource type has a widely understood
  symbol, such as refresh, folder, file, power, trash/remove, terminal, more
  menu, activity/agent, workspace, or root. Use text only when no conventional
  symbol exists or when text is the primary command label.
- Add an icon source appropriate for the React frontend, with `lucide-react`
  as the preferred candidate unless implementation finds a concrete better fit
  with the existing stack. Do not hand-roll general-purpose SVG icons when a
  library icon exists.
- Treat icons as chrome presentation only. Command ids, command dispatch,
  route identity, resource ids, backend routes, and persistence formats must
  stay unchanged.
- Preserve the dense operational workbench direction: square controls, tight
  spacing, hairline separators, restrained state color, and no rounded
  card-heavy redesign.
- Keep the main workbench panel bodies out of scope. This pass may touch their
  tab/top chrome if needed, but not Activity transcript semantics, terminal
  body behavior, read-only file rendering semantics, or future markdown
  document UI.

## Constraints

- Do not change daemon APIs, command payloads, route names, workRoot activation
  behavior, Activity read models, file explorer data shape, terminal lifecycle,
  Dockview layout persistence, or root picker backend behavior.
- Do not remove existing commands; hide low-value or placeholder controls
  behind a menu only when they remain reachable and the command dispatch path is
  preserved.
- Every icon-only action must have an accessible name and tooltip/title so the
  UI remains discoverable.
- Keep debug-oriented metadata available through tooltips, titles, or detail
  surfaces; do not surface host paths or daemon-private paths as part of the
  visual polish.
- Preserve responsive toolbar behavior at desktop and constrained widths. The
  metadata row must not wrap into extra toolbar rows, and icon controls must not
  overlap.
- Preserve terminal/xterm fit and file explorer scroll containment.

## Phases

### Phase 1: Icon-first left navigation and topbar polish

Rework the left navigation and topbar chrome so they are visibly calmer and
more workbench-like while preserving current behavior.

The implementation should:

- Introduce an icon library or icon adapter for frontend chrome, preferably
  `lucide-react`, and use conventional icons for common actions/resources.
- Replace text-heavy resource eyebrows such as `compact workRoot`,
  `workspace`, and `workRoot` with icons. Compact workRoot rows should express
  the compressed `workspace -> workRoot` meaning with a paired icon treatment
  instead of a long text eyebrow.
- Replace the oversized workspace `Remove` text button with a compact icon
  action, such as trash/remove, with a tooltip/accessibility label. Keep the
  existing `workspace.remove` command path and confirmation behavior.
- Reduce left resource-row chips. Keep only the most important state, such as
  ready/available, as an inline badge when helpful. Move `kind`,
  `availability`, `activation`, and other debug-style metadata into titles,
  tooltips, or an equivalent low-visual-weight surface.
- Express important non-ready states through row tone instead of chip lists:
  unavailable or inaccessible rows should get a muted error/degraded treatment;
  offline rows should read disabled or subdued; active/ready rows should remain
  compact and clear.
- When a workspace has multiple workRoots and the root anchor needs visual
  distinction, prefer an operational root/home/source icon rather than a playful
  crown-like symbol.
- Clarify the left-column section backgrounds. The open-workRoot area,
  workspace/resource nav, file explorer header, and file explorer body should
  read as distinct bands or regions through subtle background differences and
  hairlines, not as one same-colored block.
- Add file and folder icons to the file explorer. Keep the existing disclosure
  bullet/indent affordance for expandable folders because it helps hierarchy.
  Generic file and folder icons are sufficient for this phase; file-type-specific
  icons may be deferred.
- Rework the topbar into this structure:

  ```text
  power button | breadcrumb + ready/activity chips | primary action icons | more menu
  ```

- Move online/offline activation to a conventional power icon near the left
  side of the topbar. Preserve `workRoot.activation.set` command behavior.
- Keep only high-signal topbar chips, primarily ready/status and Activity/agent
  summary. Remove or demote `kind`, `availability`, `activation`, and `last
  command` chips unless they are shown through a tooltip or overflow details.
- Convert frequent topbar actions such as open root, refresh, and new terminal
  to icon buttons with accessible labels. Use text only where the icon alone is
  not conventional enough.
- Move low-value, placeholder, or currently non-functional workbench toggles
  such as viewer, task, diagnostics, events, and layout behind a `more` menu
  while preserving reachability and command dispatch.
- Keep the main workbench pane bodies, Activity transcript body, terminal body,
  and read-only document body visually and behaviorally stable except where
  necessary to keep chrome alignment intact.

Completion means the desktop dashboard visibly reduces text/chip clutter in
the left navigation and topbar, uses conventional icon grammar for common
actions, and keeps every existing behavior reachable through the same command
model.

Deferred scope:

- Full document/editor body redesign, markdown rendering, edit mode, translation
  overlays, mention/pathref actions, and save fan-out.
- Activity transcript readability redesign beyond topbar/nav chrome concerns.
- File-type-specific icon theming beyond generic file/folder icons.
- Root picker interaction changes or file-manager operations.
- Keyboard shortcut editor, command palette, or generalized menu framework
  beyond the small topbar overflow needed for this pass.
- Dashboard-wide React Aria migration or replacement.

Verification should include:

- Frontend build/type verification.
- Command-model tests if command dispatch wiring changes.
- Browser-level verification through the daemon-served production frontend.
- Desktop and constrained viewport screenshots covering left nav, file explorer,
  topbar, and overflow menu behavior.
- Assertions or evidence that icon-only controls have accessible names and
  remain command-routed.
- Existing browser acceptance must continue to pass, including toolbar
  single-line metadata behavior, file explorer containment, terminal fit, and
  Activity pane behavior.

### Result (a1cf23cf) - 2026-05-24

Implemented the icon-first dashboard chrome polish for the selected Phase 1
scope. The frontend now uses `lucide-react` icons for common dashboard chrome,
compresses left navigation resource rows with icon presentation and state
tones, replaces the workspace remove text button with an accessible icon
action, adds generic file/folder icons to the workRoot file explorer, and
reworks the workRoot topbar into power, breadcrumb/status, primary icon actions,
and a visible overflow menu for placeholder workbench toggles.

Command routing and backend behavior stayed stable: resource selection,
workspace removal, activation, refresh, terminal creation, file explorer
actions, Activity entrypoint, and workbench toggle commands continue to use
their existing command ids and payload boundaries. Main pane bodies, terminal
behavior, Activity transcript semantics, read-only file rendering semantics,
root picker behavior, and daemon APIs stayed out of scope.

Verification:

- `cd ws-dashboard/frontend && npm run build`
- `cd ws-dashboard/frontend && npm run test:browser`
- Browser artifacts refreshed under `ws-dashboard/frontend/e2e/.artifacts/`,
  including `desktop-workbench.png`, `file-explorer.png`,
  `narrow-workbench.png`, and `topbar-overflow.png`.

Notes:

- Screenshot review found that the overflow menu was initially present in the
  DOM but clipped by toolbar action overflow. Follow-up commit `a1cf23cf`
  made the menu screenshot-visible and reran the browser gate.
  `8fb1b2ed` contains the main implementation.
