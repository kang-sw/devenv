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
- A region-constrained workbench layout spike and substrate. Dockview is the
  first candidate because edge groups, tabbed groups, serialization, theming,
  and keyboard focus navigation match the desired VS Code-inspired shell; keep
  FlexLayout as the comparison fallback if Dockview policy code becomes noisy.
- A panel placement registry that keeps product policy outside the layout
  library: resource/file navigation belongs in the left region, editor and
  document surfaces in the main region, terminal and agent TUI surfaces in the
  bottom or main regions, and inspector/viewer/diagnostics surfaces in the
  right or bottom regions.
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

- Planned: dark-first frontend visual system baseline. Create
  `ws-dashboard/frontend/DESIGN.md`, define semantic theme tokens, convert the
  existing shell away from hardcoded light colors, and verify desktop/mobile
  screenshots.
- Planned: stable pairing redirect and route identity basis. Redirect
  successful `/pair?token=...` exchanges to `/`, keep invalid/reused/expired
  token failures non-redirecting, and introduce the frontend route shape for
  `/servers/:serverId/...`.
- Planned: constrained workbench layout spike. Compare Dockview against
  FlexLayout on the actual dashboard requirements: left/main/bottom/right
  regions, placement registry, layout serialization, keyboard focus movement,
  disabled or intercepted unconstrained floating/popout behavior, and stable
  terminal logical width policy.
- Planned: workbench substrate implementation. Adopt the selected layout
  library behind a dashboard-owned workbench adapter and render the current
  resource shell through the constrained regions.

## Cross-Child Decisions

- Treat the workbench as a ws dashboard control plane, not an IDE clone.
  CodeMirror 6 and xterm.js remain pane contents; the layout library must not
  own dashboard resources, routes, auth, runtime sessions, or command semantics.
- Dockview is the leading candidate, but only as a constrained workbench layout
  skeleton. Use edge groups or equivalent fixed regions for left, bottom, and
  optional right surfaces; keep the central region as the primary workspace.
- Keep placement policy in a dashboard-owned panel registry and adapter layer.
  Validate all programmatic panel creation, drag/drop, move, and restored
  layout state against that registry.
- Disable or intercept broad floating, popout, and arbitrary docking by default
  until a specific child ticket justifies a wider affordance.
- Keep layout persistence separate from resource identity. Layout JSON stores
  panel arrangement only; daemon APIs and `/servers/:serverId/...` routes keep
  authoritative resource identity.
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
