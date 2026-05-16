---
title: ws web dashboard direction research
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260514-feat-ws-web-daemon-foundation: first retained implementation child
  260515-epic-ws-web-dashboard-first-visible-substrate: closed first visible substrate milestone board
  260516-feat-ws-web-resource-view-model-contract: recreated first visible substrate child
  260427-chore-claude-dash-windows: prior PTY dashboard surface and Windows stability motivation
  260513-research-streamable-http-mcp-transport: adjacent daemon and remote transport research
  260513-feat-async-exec-output-reader: adjacent persisted process output and reader-agent pattern
related-mental-model:
  - developer-environment-tools
  - named-agent-runtime
  - mcp-runtime
  - plugin-runtime
---

# ws web dashboard direction research

## Background

The ws web dashboard is closer to a next-generation personal devenv UI than a
separate multi-user product. Keeping the MVP in this repository remains useful
while its core risks are wsstate, named-agent lifecycle, workRoot scoping,
harness behavior, and local/WSL/remote daemon boundaries.

The dashboard ticket set was previously split into several provisional
implementation children for frontend, workspace, terminal, agent, editor,
server-link, and remote-hardening substrates. Those children carried useful
idea-level detail but created active-ticket bloat before the design was stable.
This research ticket absorbs that provisional detail so later child tickets can
be recreated only when their scope is ready for implementation.

## Absorbed Provisional Children

This ticket absorbs the intent from these deleted provisional todo tickets:

- `260514-feat-ws-web-frontend-substrate`
- `260514-feat-ws-web-workspace-substrate`
- `260514-feat-ws-web-terminal-substrate`
- `260514-feat-ws-web-agent-dashboard-substrate`
- `260514-feat-ws-web-editor-substrate`
- `260514-feat-ws-web-server-link-forwarding`
- `260514-feat-ws-web-remote-wsl-hardening`

`260514-feat-ws-web-daemon-foundation` remains as the first concrete child
because daemon shell, owner authentication, bind-mode guards, and serving shape
are prerequisite decisions for every later panel or workspace feature.

## Repository And Documentation Shape

The dashboard may stay in this monorepo while it is a personal devenv tool and
while implementation pressure mostly lands on existing ws runtime contracts.
If the dashboard becomes an independently released product, has a separate
release cadence, or its frontend/build churn interferes with workflow runtime
work, split it into a separate repository or package.

The root project directory is `ws-dashboard/`. The scaffold currently contains
Rust workspace slots for core resource primitives, harness-core abstractions,
a harness CLI wrapper, a daemon binary, and a frontend placeholder. The name is
deliberately descriptive because the dashboard is usually started once and
reached through a browser URL. Avoid `tools/` for the main source tree: this
surface is a daemon, frontend, and runtime-facing product area rather than a
small helper utility. `wsdash` can remain a short command alias later, but the
source directory should favor discoverability.

To control documentation bloat:

- Keep dashboard tickets flat under normal ticket status directories.
- Use `ai-docs/spec/ws-web-dashboard/` for dashboard specs once behavior is
  ready for spec coverage.
- Use `ai-docs/mental-model/ws-web-dashboard/` only after implementation
  creates real dashboard subdomains. Mental-model docs should not prefill
  speculative design material.

## Resource Model

The dashboard resource model should preserve an honest hierarchy while letting
the UI compress singleton chains:

```text
server
  workspace
    workRoot [online | offline | moved | inaccessible]
      mainInstance
        subInstance
        subInstance
```

`server` is a physical or logical host environment such as the local machine, a
WSL distro, or a remote host. `workspace` is a daemon-discovered project group,
not a user-created category. A workspace groups one or more workRoots and is
usually inferred from a Git repository group or from a single plain directory.

`workRoot` is the physical directory where processes and UI state run. A
workRoot can be offline, moved, or inaccessible without disappearing from the
user's recent context. A workRoot identity should survive kind changes such as a
plain directory becoming Git-backed or a Git-backed directory losing Git
metadata.

The workRoot kind should stay additive:

```text
workRoot.kind: plainDirectory | gitPrimaryRoot | gitLinkedWorktree
```

`plainDirectory` can still be opened, inspected, and used as a terminal,
editor, or instance-spawn target. `gitPrimaryRoot` and `gitLinkedWorktree`
share the same core workRoot UI and Git-aware affordances, but their metadata
should preserve the difference between a repository's primary root directory
and a linked worktree. That distinction matters for labels, grouping,
branch/lifecycle context, and destructive actions.

The UI may compact singleton chains without changing the data model:

```text
workspace has one workRoot -> render workspace/workRoot as one row
workRoot has one mainInstance -> render workRoot/mainInstance as one row
all three are singletons -> render workspace/workRoot/mainInstance as one row
```

`mainInstance` is the user-facing conversation or control point, analogous to
the primary interaction with an AI assistant in the current workflow.
`subInstance` is delegated work attached to that main interaction: ws agents,
exec jobs, translation jobs, document viewers, diagnostics, subprocesses, or
side tasks.

The data model can implement instances as parented entities, but dashboard
language should preserve the main/sub distinction:

- main instances have no parent and receive direct user interaction;
- sub instances have a parent main instance and are delegated, passive, or
  auxiliary unless a later UX explicitly promotes them.

Potential instance fields:

```text
instanceId
parentInstanceId | null
role: main | sub
kind: harness | agent | terminal | editor | viewer | exec | translation | task
interactionMode: direct | delegated | passive
```

Use opaque ids at every level. Host paths, Git roots, link details, and runtime
session identifiers remain daemon-owned state exposed through view models, not
URL identity.

The dashboard should never expose generic recursive folder deletion. If it
offers destructive workRoot lifecycle actions, keep them Git-aware and
explicit: linked worktrees may later support a guarded `git worktree remove`
action, while plain directories and Git primary roots should not show a
delete-folder action.

Discovery needs both automatic and user-directed entry points. Automatic
discovery can propose recent workRoots, known ws roots, daemon working
directories, configured search roots, and Git worktrees. User-directed
discovery should feel like a lightweight explorer that lets the owner navigate
server filesystem roots and open a directory even when it is not a Git
repository. The explorer may support `Create empty folder` as a narrow
new-workRoot affordance, but it should not become a generic file manager with
delete, rename, move, or copy operations.

A workRoot kind should be re-detected through manual refresh plus opportunistic
refresh when selecting, opening, or spawning from a workRoot. Broad filesystem
watcher behavior should wait until the visible substrate proves the model and
can constrain watching to opened or visible roots.

Bookmarks are a useful navigation idea, but they should stay research-level
until the first visible substrate stabilizes. Model space should remain open for
future saved pointers such as discovered, recent, bookmarked, or manually opened
workRoots without making bookmark CRUD part of the first implementation child.

## Harness And Runtime Library Direction

A self-contained agent harness library is a larger runtime direction, not a
dashboard MVP child. The long-term goal is to support MCP and skill standards
while reducing hard dependence on external harnesses such as Codex or Claude:
use host harness capabilities when present, but keep enough local harness
capability for standalone operation.

This direction also prepares for a possible subscription CLI to API-based model
transition. The dashboard should not own harness authority. Runtime detection,
backend execution, model aliasing, and MCP/session state stay in ws runtime
contracts. Dashboard/library UI can expose harness status, configuration, and
control views as consumers of those contracts.

The harness/runtime layer should eventually include automatic API-key and
secret filtering. The filter should protect prompts, model transcripts, logs,
diagnostic streams, document viewers, translation jobs, and API-backed model
calls from accidentally exposing credentials. Treat this as a defense-in-depth
runtime feature, not an authorization boundary: host authentication, model API
configuration, and secret storage still need separate controls.

Likely future split:

- a runtime/library epic for local harness capability, API-backed model access,
  secret filtering, and host-harness compatibility;
- dashboard children that render and control harness view models once the
  runtime contracts exist.

## Dashboard Shell And Navigation

The frontend shell should remain extension-ready rather than a fixed mock page.
Useful deferred substrate ideas include:

- panel and command registries;
- server/workspace/workRoot/main-instance/sub-instance scope context;
- dock layout, tabbed panels, persisted layout state, reset affordances, and
  duplicate-dashboard affordances;
- a typed mock/live data boundary for daemon APIs and event streams;
- dense operational primitives following `ai-docs/ref/design.md`: restrained
  surfaces, square corners, hairlines, and practical information density.

Keyboard behavior should be treated as a dashboard-level interaction design
topic, not only an editor feature. The desired direction is tmux- and Vim-like
navigation across panes, tabs, command surfaces, and editor buffers. The exact
binding list is intentionally deferred for user curation.

The next frontend substrate should treat the current three-panel shell as an
information-architecture skeleton, not as final visual design. Preserve the
left navigation as a location selector, but evolve the layout into a
workRoot-scoped constrained workbench:

```text
left nav:
  server
    workspace
      workRoot rows with compact status badges

workRoot workbench:
  split group A
    pinned row: agent or persistent terminal
    opened row: editor, viewer, diagnostics, events, or task view
  split group B
    pinned row: agent or persistent terminal
    opened row: editor, viewer, diagnostics, events, or task view
```

The old `center/sub area` split is less useful than sibling split groups whose
placement policy protects the active agent view while still letting editors,
viewers, terminals, and support surfaces move flexibly. The default preset
should start with two groups, side-by-side on wide screens and stacked on
narrow screens. The model should keep room for later free splitting without
requiring the first substrate to expose a complete split-manipulation UI.

Use `ai-docs/ref/design.md` as a Carbon-inspired reference for density, square
corners, hairlines, and restrained operational UI. Do not apply it as a default
light palette. The dashboard should start from a dark-first semantic theme and
allow later color tuning without hardcoding light-mode values throughout the
component tree.

Workbench-library verification currently favors Dockview as a constrained
layout substrate, with FlexLayout as the comparison fallback. Dockview's tabbed
groups, serialization, theming, and focus navigation line up with the desired
VS Code-editor-group-like workbench shape. The important constraint is that
Dockview should stay a layout skeleton, not an IDE platform or resource model.
Placement policy belongs in a dashboard-owned panel registry:

```text
left nav: server/workspace/workRoot location selection only
split group pinned rows: durable agent and persistent terminal surfaces
split group opened rows: editor, viewer, diff, diagnostics, logs/events, task view, inspector
file open default: second or later split group, preserving the active agent view
workRoot utility toggles: global/workRoot combined bar, not split-group tabs
```

`mainInstance` should not remain a default left-nav child. It is better treated
as a durable workRoot-local surface that can appear in a split group's pinned
row. `subInstance` should remain a view-only projection attached to a main
instance through badges, popovers, cards, or drawers rather than becoming a
top-level split-group tab. Long-running tasks should aggregate into a
workRoot-scoped task view and main-instance-local badges/popovers; individual
tasks should not become top-level tabs by default.

Terminal panes should behave like daemon-owned, tmux-like sessions: closing a
panel detaches the view, while explicit terminate commands own process
shutdown. Agent panes should remain a higher-level abstraction where a PTY is
only one possible interface type, so future named-agent projections, headless
calls, or structured agent GUIs do not have to masquerade as terminals.

The selected layout library must not own dashboard auth, route identity,
resource identity, runtime authority, or command semantics. Persisted layout
JSON stores arrangement only. The authoritative resource route shape should
keep explicit server identity, such as `/servers/:serverId/...`, rather than
encoding backed server context inside workspace, workRoot, or instance ids.

The pairing URL should remain a one-time startup entrypoint. After successful
pairing, the browser should receive the owner session cookie and redirect to a
token-free stable app URL so refresh and deep-link navigation do not expose or
depend on the pairing token.

Terminal and agent TUI panes need a stable logical size policy. Visual panel
resize should not continuously trigger PTY/TUI logical width changes, because
Codex-like TUIs can redraw and dump large conversation state when columns
change. Logical columns should move through presets, committed resize, or an
explicit later fit command.

## Documents, Translation, And Mentions

The dashboard should have a reusable text/document viewer rather than one-off
renderers per panel. It should eventually support tickets, specs, mental
models, plans, agent output, command output, and image-capable rich text where
needed.

Deferred viewer ideas:

- detect ticket stems, spec stems, mental-model names, and plan stems in main
  harness or sub-agent text;
- let shift-click open a related-document popup instead of navigating away;
- support drag or selection gestures that create mentions for prompts or notes;
- support a right-side document viewer panel;
- add a translation button and translation API configuration page;
- cache translated document content in the browser by document SHA256, with
  translation providers being either LLMs or dedicated translation APIs.

Translation should be a viewer feature over immutable content hashes, not a
mutation of source documentation.

## First Visible Substrate Closure

The first visible substrate discussion is closed as an implementation milestone.
The recreated child tickets are:

- `260516-feat-ws-web-resource-view-model-contract` - first child and
  implementation-order blocker for workRoot vocabulary, authenticated resource
  APIs, mock provider, and golden fixtures.
- `260516-feat-ws-web-minimal-frontend-shell` - first inspectable browser shell
  over the shared API contract.
- `260516-feat-ws-web-local-workspace-discovery` - live local discovery and
  root-picker support after the resource contract exists.
- `260516-feat-ws-web-instance-event-stream` - shared authenticated stream
  envelope and fixture-backed reconnect/backfill scaffold.

These tickets replace the earlier provisional frontend/workspace/terminal/agent
substrate split for the first visible milestone. The rest of this research
ticket remains idea-level direction for later epics and children.

## Deferred Substrates

Future child tickets can be recreated from this research once their boundaries
are ready:

- browser terminal and PTY bridge;
- named-agent dashboard view models;
- browser-native editor and modal editing;
- document viewer, translation, stem popup, and mention substrate;
- constrained workRoot workbench layout substrate with Dockview/FlexLayout
  verification, sibling split groups, pinned/opened rows, and daemon-owned
  terminal/agent lifecycle boundaries;
- linked daemon/server forwarding for local, WSL, and remote environments;
- remote, WSL, and public-bind hardening;
- runtime/library harness capability;
- bookmarks and saved workRoot pointers;
- broad filesystem watcher behavior.

The next implementation-order blocker for the dashboard is the resource
view-model contract child, not another broad research split.
