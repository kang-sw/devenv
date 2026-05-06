# Mental Model Index

Cross-domain operational knowledge for modifying the devenv workflow system.

## Domains

| Domain | File | Scope |
|--------|------|-------|
| plugin-runtime | `mental-model/plugin-runtime.md` | Codex plugin manifests, launcher repair, runtime metadata, release assets |
| mcp-runtime | `mental-model/mcp-runtime.md` | ws-mcp stdio server, tool registry, CLI mirror, concurrency, profile gates |
| named-agent-runtime | `mental-model/named-agent-runtime.md` | File-backed agents, async calls, locks, subqueries, Codex backend handling |
| workflow-skills | `mental-model/workflow-skills.md` | Codex lead skills, Claude compatibility skills, workflow prompt orchestration |
| documentation-system | `mental-model/documentation-system.md` | Project memory, conventions, specs, tickets, mental models, reference tracing |
| git-workflow-tools | `mental-model/git-workflow-tools.md` | Constrained Git MCP/CLI tools and structured commit behavior |
| api-documentation-cache | `mental-model/api-documentation-cache.md` | API docs domain routing, manager agents, cache ownership, prompt contracts |
| claude-compatibility | `mental-model/claude-compatibility.md` | Claude shims, legacy package, CLI fallbacks, installer snapshot, Windows wrappers |
| developer-environment-tools | `mental-model/developer-environment-tools.md` | install.sh, shell/editor config, tmux helpers, Claude TUIs |
| prompt-bundle | `mental-model/prompt-bundle.md` | Embedded prompt discovery, resolution, delegate orientation, bundle metadata |

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

**Host split:** Codex-first behavior lives under `agents-plugin/` and
`agents-plugin-tool/`; `claude-plugin/` remains the Claude compatibility
reference and must not silently redefine shared workflow semantics.
