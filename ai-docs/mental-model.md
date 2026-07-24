# Mental Model Index

Cross-domain operational knowledge for modifying the devenv workflow system.

## Project Reading Map

This repo has enough workflow docs that common tasks benefit from repeatable
first-pass routing. Use this map to choose initial context; use specs, tickets,
source, and tests for behavioral truth.

| Task / topic | Read first | Then read |
|--------------|------------|-----------|
| Plugin packaging, install, runtime metadata, launcher repair | `spec/plugin-runtime.md` | `mental-model/plugin-runtime.md`, `mental-model/claude-compatibility.md` |
| MCP tools, wsdoc discovery, convention access, CLI mirrors | `spec/mcp-tools.md` | `mental-model/mcp-runtime.md`, `mental-model/documentation-system.md` |
| Workflow skills, routing, sprint/review/proceed behavior | `spec/workflow-skills.md` | `mental-model/workflow-skills.md`, `ref/wsflow-mirroring.md` when wsflow mirrors may change |
| Specs, tickets, mental models, project memory, references | `spec/documentation-system.md` | `mental-model/documentation-system.md` |
| Named agents, worktree-scoped registry metadata, backend behavior | `spec/named-agent-runtime.md` | `mental-model/named-agent-runtime.md`, `ref/ws-agent-runtime.md` |
| API documentation cache and manager sessions | `spec/api-documentation-cache.md` | `mental-model/api-documentation-cache.md` |
| Personal shell/editor/tmux/Claude dashboard tooling | `spec/developer-environment-tools.md` | `mental-model/developer-environment-tools.md` |
| Dashboard daemon, browser UI, owner auth, Activity Console streams, resource view-model API/fixtures, host-control boundary | `spec/ws-web-dashboard/index.md` | `mental-model/ws-web-dashboard/index.md` (terminal helper-process/boot-reconcile: `mental-model/ws-web-dashboard/terminal.md`; frontend terminal output-cursor batching: `mental-model/ws-web-dashboard/terminal-render.md`) |

## Domains

| Domain | File | Scope |
|--------|------|-------|
| plugin-runtime | `mental-model/plugin-runtime.md` | Codex plugin manifests, launcher repair, runtime metadata, release assets |
| mcp-runtime | `mental-model/mcp-runtime.md` | ws-mcp stdio server, tool registry, CLI mirror, concurrency, profile gates |
| named-agent-runtime | `mental-model/named-agent-runtime.md` | Worktree-scoped SQLite agent registry metadata, file-backed payloads, async calls, Codex backend handling |
| workflow-skills | `mental-model/workflow-skills.md` | Codex lead skills, workflow prompt orchestration |
| documentation-system | `mental-model/documentation-system.md` | Project memory, conventions, specs, tickets, mental models, reference tracing |
| git-workflow-tools | `mental-model/git-workflow-tools.md` | Constrained Git MCP/CLI tools and structured commit behavior |
| api-documentation-cache | `mental-model/api-documentation-cache.md` | API docs domain routing, manager agents, cache ownership, prompt contracts |
| claude-compatibility | `mental-model/claude-compatibility.md` | Claude shim, agents-plugin compatibility metadata, installer snapshot, retired legacy boundaries |
| developer-environment-tools | `mental-model/developer-environment-tools.md` | install.sh, shell/editor config, tmux helpers, Claude TUIs |
| prompt-bundle | `mental-model/prompt-bundle.md` | Embedded prompt loading (wsprompt/go:embed), call-time rsrc playbook loading (wsrsrc/filesystem), delegate orientation, bundle metadata |
| ws-web-dashboard | `mental-model/ws-web-dashboard/index.md` | Personal ws dashboard daemon, owner-auth boundary, UI serving, Activity Console streams, resource view-model API/fixtures, and host-control separation |
| ws-web-dashboard/terminal | `mental-model/ws-web-dashboard/terminal.md` | Sub-domain: daemon-side terminal helper-process ownership, NDJSON IPC transport, registry file, and boot-reconcile decision table |
| ws-web-dashboard/terminal-render | `mental-model/ws-web-dashboard/terminal-render.md` | Sub-domain: frontend terminal render-state batching, rAF-deferred output-cursor accumulator, and its flush/pending-read contract |

## Directory Hierarchy

Domain docs live in one of two shapes:

- **Flat file** - `mental-model/<domain>.md`. Single-concern domain.
- **Directory** - `mental-model/<domain>/index.md` + `mental-model/<domain>/<sub>.md`. Multi-concern domain; `index.md` carries cross-cutting context and inherited `## Domain Rules`, each child file covers one sub-concern.

Promotion from flat to directory is triggered by code-structure change observed
in the diff, not authorial preference. The hierarchy is encoded in the file
path; no frontmatter `parent:` link is maintained.

Ancestor loading is invariant: any agent loading a sub-domain doc must also load
the parent `index.md` before starting work, so inherited Domain Rules are
visible.

## Domain Rules

Each domain doc may carry a `## Domain Rules` section holding user-authored
prescriptions for AI agents working in that domain. Rules are scoped to the
domain, analogous to root `AGENTS.md` architecture rules.

Rules are authored via `ws:lead-add-rule` or manual edit. No agent modifies rule
content autonomously; updater workflows may only preserve or promote rules
during structural splits, or flag stale rules for user resolution.

## Shared Conventions

**Spec stems:** `{#YYMMDD-slug}` body anchors are the shared identity protocol
between specs, tickets, mental models, and discovery tools. Anchors live in spec
body text, not frontmatter.

**Ticket status:** ticket state is directory-derived under `ai-docs/tickets/`.
Discovery excludes `.done/` and `.dropped/` unless explicitly requested.

**Runtime metadata:** plugin runtime compatibility depends on
`runtime.capabilities`, `runtime.info`, `agents-plugin/runtime.json`, embedded
prompt bundle hashes, and MCP tool/CLI surface lists staying synchronized.

**Host split:** Active workflow behavior lives under `agents-plugin/` and
`agents-plugin-tool/`; retired Claude source material belongs only in historical
references and git history.
