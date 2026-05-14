---
title: ws web dashboard direction research
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260514-feat-ws-web-daemon-foundation: first retained implementation child
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
while its core risks are wsstate, named-agent lifecycle, worktree scoping,
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

Use `ws-dashboard/` as the root project directory when implementation starts.
The name is deliberately descriptive because the dashboard is usually started
once and reached through a browser URL. Avoid `tools/` for the main source tree:
this surface is a daemon, frontend, and runtime-facing product area rather than
a small helper utility. `wsdash` can remain a short command alias later, but the
source directory should favor discoverability.

To control documentation bloat:

- Keep dashboard tickets flat under normal ticket status directories.
- Use `ai-docs/spec/ws-web-dashboard/` for dashboard specs once behavior is
  ready for spec coverage.
- Use `ai-docs/mental-model/ws-web-dashboard/` only after implementation
  creates real dashboard subdomains. Mental-model docs should not prefill
  speculative design material.

## Resource Model

The dashboard resource model should emphasize one main user interaction point
per worktree. A refined shape is:

```text
server
  workspace
    worktree [online | offline | moved | inaccessible]
      mainInstance
        subInstance
        subInstance
```

`server` is a physical or logical host environment such as the local machine, a
WSL distro, or a remote host. `workspace` is a project or repository family.
`worktree` is the concrete root where processes and UI state run. A worktree can
be offline, moved, or inaccessible without disappearing from the user's recent
context.

`mainInstance` is the user-facing conversation or control point, analogous to
the primary interaction with an AI assistant in the current workflow.
`subInstance` is delegated work attached to that main interaction: ws agents,
exec jobs, translation jobs, document viewers, diagnostics, subprocesses, or
side tasks.

The data model can implement this as parented instances, but dashboard language
should preserve the main/sub distinction:

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
- server/workspace/worktree/main-instance/sub-instance scope context;
- dock layout, tabbed panels, persisted layout state, reset affordances, and
  duplicate-dashboard affordances;
- a typed mock/live data boundary for daemon APIs and event streams;
- dense operational primitives following `ai-docs/ref/design.md`: restrained
  surfaces, square corners, hairlines, and practical information density.

Keyboard behavior should be treated as a dashboard-level interaction design
topic, not only an editor feature. The desired direction is tmux- and Vim-like
navigation across panes, tabs, command surfaces, and editor buffers. The exact
binding list is intentionally deferred for user curation.

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

## Deferred Substrates

Future child tickets can be recreated from this research once their boundaries
are ready:

- frontend shell and design primitives;
- workspace and Git worktree discovery;
- browser terminal and PTY bridge;
- named-agent dashboard view models;
- browser-native editor and modal editing;
- document viewer, translation, stem popup, and mention substrate;
- linked daemon/server forwarding for local, WSL, and remote environments;
- remote, WSL, and public-bind hardening;
- runtime/library harness capability.

The first implementation slice should stay focused on daemon foundation unless
a later discussion explicitly promotes a narrower UI or runtime child.
