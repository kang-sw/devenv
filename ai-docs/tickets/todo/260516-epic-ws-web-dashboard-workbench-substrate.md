---
title: ws web dashboard workbench substrate
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260514-research-ws-web-dashboard-direction: direction research and deferred substrate capture
  260515-epic-ws-web-dashboard-first-visible-substrate: completed prerequisite visible shell and resource substrate
related-mental-model:
  - ws-web-dashboard
---

# ws web dashboard workbench substrate

## Scope

Turn the first visible dashboard shell into a dark-first, constrained workbench
substrate that can host later terminal, agent, editor, document, output, event,
and inspector surfaces without becoming a full IDE platform.

The milestone should establish:

- A dashboard-specific dark-first frontend `DESIGN.md`-style guide under
  `ws-dashboard/frontend/`, derived from `ai-docs/ref/design.md` as a
  Carbon-inspired geometry, density, and component reference rather than as a
  default light palette.
- A semantic frontend theme/token baseline for the existing shell.
- Stable browser entry after pairing: the startup pairing URL remains a
  one-time entrypoint, but successful pairing redirects to a token-free stable
  app URL backed by the owner session cookie.
- Stable route identity direction using explicit
  `/servers/:serverId/...` segments instead of hiding server identity inside
  workspace, workRoot, or instance ids.
- A constrained workRoot workbench layout spike and substrate. The dashboard
  should use a `left nav | workRoot workbench` information architecture rather
  than a fixed `left | center | sub/right/bottom` region hierarchy.
- A split-group workbench model. Within an opened workRoot, sibling split
  groups own their local tab selection; each group can have a pinned row for
  durable surfaces and an opened row for transient or support surfaces.
- A placement registry that keeps product policy outside the layout library:
  the left nav selects server/workspace/workRoot locations; agent and
  persistent terminal surfaces default to pinned rows; editor, viewer, diff,
  diagnostics, logs/events, task view, and inspector surfaces default to
  opened rows; file-open actions should prefer the second or later split group
  so the first group can preserve the active agent view.
- A region-constrained layout library spike. Dockview remains the first
  candidate because tabbed groups, serialization, theming, and keyboard focus
  navigation match the desired VS Code-editor-group-like shell; keep FlexLayout
  as the comparison fallback if Dockview policy code becomes noisy.
- A terminal/agent resize policy proving that visual layout changes do not
  continuously mutate PTY/TUI logical columns. Logical terminal width changes
  should happen through explicit presets, committed resize, or a later
  user-invoked fit command.

## Non-Scope

- Full VS Code, Theia, OpenVSCode, or extension-host parity.
- Arbitrary user-defined docking for every panel in every region.
- Browser ownership of ws MCP, named-agent, harness, or process authority.
- Generic file-manager behavior such as delete, rename, move, copy, or
  recursive folder deletion.
- Final editor, terminal, agent, or document-viewer feature depth beyond the
  substrate needed to host those panes.
- Public deployment hardening beyond preserving the existing owner-auth route
  boundary.

## Child Tickets

- `260516-feat-ws-web-dark-visual-system` - done; created
  `ws-dashboard/frontend/DESIGN.md`, define semantic theme tokens, convert the
  existing shell away from hardcoded light colors, and verify desktop/mobile
  screenshots.
- `260516-feat-ws-web-stable-pairing-routes` - done; redirected successful
  `/pair?token=...` exchanges to a token-free app URL, keep
  invalid/reused/expired token failures non-redirecting, and introduce the
  frontend route shape for `/servers/:serverId/...`.
- `260516-research-ws-web-workbench-layout-spike` - done; compared Dockview against
  FlexLayout on workRoot-scoped sibling split groups, group-local
  pinned/opened rows, placement registry, layout serialization, keyboard focus
  movement, constrained floating/popout policy, and stable terminal logical
  width policy.
- `260516-feat-ws-web-workbench-substrate` - adopt the selected layout library
  behind a dashboard-owned workbench adapter and render the current resource
  shell through the `left nav | workRoot workbench` model.

## Cross-Child Decisions

- Treat the workbench as a ws dashboard control plane, not an IDE clone.
  CodeMirror 6 and xterm.js remain pane contents; the layout library must not
  own dashboard resources, routes, auth, runtime sessions, or command semantics.
- The left nav should stop at server/workspace/workRoot by default. It may show
  compact status badges, but it should not expand main instances or sub
  instances as its normal hierarchy.
- A workRoot is the workbench container. Main instances are workRoot-local
  durable surfaces, not the top-level navigation unit. Sub instances are
  view-only projections attached to a main instance through badges,
  popovers, cards, or drawers rather than independent top-level panels.
- The old `center/sub area` model is superseded by workRoot-scoped sibling
  split groups. The default preset should be two groups, side-by-side on wide
  screens and stacked on narrow screens, while preserving a model that can
  later support free splitting.
- Each split group owns a pinned row and an opened row. Pinned rows are for
  durable surfaces such as agent and persistent terminal views. Opened rows are
  for editor, viewer, diff, diagnostics, logs/events, task view, and inspector
  surfaces.
- WorkRoot-level utility toggles belong in a thin global/workRoot combined bar,
  not in split-group pinned rows. This bar should cover breadcrumbs/status and
  viewer, task view, diagnostics/events, and layout menu actions.
- Dockview is the leading candidate, but only as a constrained workbench layout
  skeleton. Use tabbed groups or equivalent split groups for local selection;
  keep fixed global/workRoot controls outside the layout library.
- Keep placement policy in a dashboard-owned panel registry and adapter layer.
  Validate all programmatic panel creation, drag/drop, move, and restored
  layout state against that registry.
- Opening a file from the left nav should prefer the second or later split
  group. Agent and persistent terminal views should default to the first or
  focused group. If a surface is already open, focus it instead of duplicating
  it unless the user explicitly asks for another view.
- Disable or intercept broad floating, popout, and arbitrary docking by default
  until a specific child ticket justifies a wider affordance.
- Keep layout persistence separate from resource identity. Layout JSON stores
  panel arrangement only; daemon APIs and `/servers/:serverId/...` routes keep
  authoritative resource identity.
- A frontend panel is an attachment, not a backend instance. Closing a panel
  detaches the view by default; explicit terminate or close-resource commands
  end daemon-owned terminal or agent lifecycle.
- Terminal panes should behave like daemon-owned, tmux-like sessions. Agent
  surfaces should remain a higher-level abstraction where PTY is only one
  possible interface type, not the definition of an agent panel.
- Long-running tasks should aggregate into a workRoot-scoped task view and
  main-instance-local badges/popovers. Individual tasks should not become
  top-level split-group tabs by default.
- Preserve `ctrl+b` as ctrl plus lowercase `b` for the future dashboard prefix
  model. Keyboard behavior should compose panel focus commands, fuzzy jump, and
  pane-local editor/terminal bindings without forcing one global mode into all
  controls.
- PTY/TUI panes must not resize their logical columns continuously during
  visual layout drag. This is a dashboard terminal policy, not a layout-library
  responsibility.

## Completion Criteria

- Done: child tickets deliver a dark-first shell, token-free refresh-safe app
  entry after pairing, explicit server-scoped route identity, and a constrained
  workbench substrate that can host future editor, terminal, agent, viewer,
  output, and inspector panes.
- Dropped: a full IDE platform such as Theia/OpenVSCode replaces the custom
  dashboard workbench direction, or enforcing the required placement and PTY
  stability policies on the selected layout library would require unacceptable
  complexity.
- Deferred: live PTY implementation, named-agent controls, full editor
  capability, document viewer depth, root explorer polish, bookmarks, linked
  server federation, and user-facing keybinding customization belong to later
  child tickets or epics after this substrate is proven.
