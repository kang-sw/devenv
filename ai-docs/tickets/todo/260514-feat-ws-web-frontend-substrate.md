---
title: ws web frontend substrate
parent: 260514-epic-ws-web-dashboard-mvp
related-mental-model:
  - developer-environment-tools
---

# ws web frontend substrate

## Background

The dashboard should grow by adding panels, commands, data sources, and event
streams as the user discovers useful workflows. The first frontend slice should
therefore build an extension-ready shell rather than a fixed mock page.

The shell should be server/workspace/instance-aware from the start so local,
WSL, and remote resources can share the same UI model.

Frontend UI implementation delegated through ws named agents should use
`model: "opus"` unless the user overrides that choice for this ticket.

## Prior Art

`ai-docs/ref/design.md` is the initial visual system reference. Preserve the
restrained, square-corner, hairline-driven operational style while adapting the
density and components for dashboard work rather than marketing-page structure.

## Phases

### Phase 1: Add app shell and registries

Create the React/Vite shell with a panel registry, command registry,
server/workspace/instance scope context, toolbar/menu contribution points, and
typed panel metadata.

### Phase 2: Add dock layout and persisted UI state

Add resizable panes, tabbed panel containers, per-workspace layout persistence,
layout reset, duplicate-dashboard affordances, and empty/loading/error states.

### Phase 3: Add design-system primitives

Implement baseline tokens and primitives for square buttons, icon buttons,
inputs, tabs, split panes, lists, toolbars, status rows, and dense information
surfaces.

### Phase 4: Add mock/live data boundary

Create a typed client API boundary with mock providers for fast UI iteration,
live providers for daemon APIs, a common event-stream abstraction, and resource
keys shaped around `serverId`, optional `workspaceId`, and optional
`instanceId`.

### Phase 5: Add server/workspace navigation

Add a left navigation model that groups by server and lists workspaces flat
within each server group. Server groups should expose a creation/add action such
as `[+]`, and workspace rows should expose actions for adding new instances in
that workspace. Worktree workspaces should remain visible as worktree-specific
entries with lineage context instead of being hidden behind a nested
project-only tree. Exact row labels and grouping chrome are TBA.

### Phase 6: Add contribution examples

Add lightweight terminal, agent, and editor panel stubs that prove new panels
and commands can be added without rewriting the app shell.
