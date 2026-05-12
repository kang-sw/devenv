<!-- Memory policy: prune aggressively as project advances. Completed
     work belongs in git history, not here. Keep only what an AI session
     needs to orient itself and pick up work. If it's derivable from
     code or git log, delete it from this file. -->

# devenv - Project Index

## Repo

Meta-workflow repository for workflow documents, skills, agents, plugin
packaging, helper commands, MCP tooling, and dev-environment templates. Specs,
tickets, and mental models here describe the workflow system itself; downstream
application material belongs in downstream projects.

Active plugin package: `agents-plugin/` (`ws@0.23.0`).
Native MCP/tooling source: `agents-plugin-tool/`.
Retired Claude source material: `ai-docs/ref/claude-home-legacy.md` and git
history.

## Current Branch Rules

- Branch: `main`.
- No branch-specific spec or mental-model freeze is active.
- Keep `.codex` untracked unless the user explicitly asks to stage it.

## Plugin Topology

- `./install.sh update` handles first-time install and settings patching on a
  new machine.
- Root `CLAUDE.md` is the only live Claude compatibility shim and points at
  `AGENTS.md`.
- `install.sh` snapshots `agents-plugin/` for Claude-compatible plugin installs
  when Claude Code is available.
- `agents-plugin/` is registered through `.agents/plugins/marketplace.json`;
  Codex UI install has verified `ws:lead-skill-authoring`,
  `ws:lead-write-ticket`, and `ws:lead-discuss`.
- Codex local plugin iteration has no known CLI refresh path; use UI
  uninstall/install or a fresh Codex session after editing the registered source.
- `agents-plugin/.codex-plugin/plugin.json` references plugin-local `.mcp.json`
  through `"mcpServers": "./.mcp.json"`.
- Changed plugin-managed Codex MCP config requires user-performed plugin cache
  refresh before installed-cache verification.
- `claude plugin validate agents-plugin` passes; runtime Claude invocation of
  `agents-plugin` remains compatibility behavior, not a separate source tree.
- `ai-docs/.old/` is the Git-tracked project archive for inactive reference
  material that should not appear in default file listings.

## Read Before Editing

| File | Use |
|------|-----|
| `ai-docs/ref/skill-authoring.md` | Skill/agent/prompt/convention authoring rules |
| `ai-docs/ref/codex-integration.md` | Probed Codex CLI behavior |
| `ai-docs/ref/ws-mcp.md` | MCP process, tools, CLI fallbacks, verification levels |
| `ai-docs/ref/ws-agent-runtime.md` | Durable agent runtime contract |
| `ai-docs/ship/ws.md` | Release process for `ws` |
| `ws/infra.read("impl-playbook")` | Implementation discipline |
| `ws/infra.read("subagent-rules")` | Subagent dispatch rules |
| `ws/infra.read("executor-wrapup")` | Shared post-implementation wrapup |

Before editing tickets/specs/mental models, read the matching convention through
`ws/convention.read`. Before editing skill, agent, prompt, or convention text,
read `ai-docs/ref/skill-authoring.md`.

## Runtime Surfaces

MCP contract: `ai-docs/ref/ws-mcp.md`.

Implemented MCP tools:

- Context/docs: `ws/project_tree`, `ws/infra.read`, `ws/convention.read`,
  `ws/mental_models.list`, `ws/mental_models.find`,
  `ws/mental_models.status`
- Session root: `ws/session.set_default_root`, `ws/session.get_default_root`
- Specs: `ws/spec_stem.generate`, `ws/spec_index.verify`, `ws/specs.list`,
  `ws/specs.find`, `ws/specs.status`
- Runtime: `ws/runtime.info`
- Delegation: `ws/subquery`, `ws/path.generate`
- References: `ws/references.trace`
- Tickets: `ws/tickets.list`, `ws/tickets.find`, `ws/tickets.status`
- Git: `ws/git.status`, `ws/git.diff`, `ws/git.log`, `ws/git.merge_base`,
  `ws/git.commit`
- Agents: `ws/agents.register`, `ws/agents.call`, `ws/agents.wait`,
  `ws/agents.result`, `ws/agents.status`, `ws/agents.tail`,
  `ws/agents.cancel`, `ws/agents.recall`, `ws/agents.print`,
  `ws/agents.erase`
- API docs: `ws/api.list`, `ws/api.ask`, `ws/api.ask_async`,
  `ws/api.status`, `ws/api.result`, `ws/api.cancel`

Shared `agents-plugin` skill text uses MCP names, not repo-local paths. Infra
and convention text are bundled into the runtime and read through
`ws/infra.read` and `ws/convention.read`.

## MCP Runtime Notes

`agents-plugin-tool/cmd/ws-mcp` provides stdio MCP, `version`, and `doctor`.
Runtime binaries are plugin cache-local under
`agents-plugin/.runtime/<os>-<arch>/ws-mcp[.exe]`.
Downstream simulation smoke: create a temp Git root with only `ai-docs/_index.md`,
run `WS_MCP_PROJECT_ROOT=<tmp> go run ./cmd/ws-mcp serve --stdio` from
`agents-plugin-tool/`, and call `infra.read("executor-wrapup")`; it must succeed
without any `claude-plugin/` directory.

The Python launcher `agents-plugin/bin/ws-mcp-launcher.py`:

- runs from the installed Codex plugin cache with `.mcp.json` `cwd: "."`;
- derives `WS_MCP_PROJECT_ROOT` from the parent Codex process `PWD`;
- downloads release assets named `ws-mcp-<os>-<arch>[.exe]`;
- verifies `SHA256SUMS`;
- repairs missing, incompatible, or tool-surface-stale cache binaries;
- for this machine's local install only, can use dev binaries or build from
  `~/devenv/agents-plugin-tool` when `.local-devenv-runtime` exists.

Windows plugin-managed startup uses the same Python launcher. Native Windows
needs a working `python3` command; if the Windows Store alias is present without
Python installed, install Python 3 and refresh/reinstall the plugin before
rechecking `codex mcp list`.

## Prompt And Agent Inventory

Active workflows use the embedded runtime prompt bundle.

Key prompts: `code-reviewer`, `code-review-correctness`, `code-review-fit`,
`code-review-test`, `mental-model-updater`, `project-survey`, `sprint-survey`,
`implementer`, `skeleton-populator`, `skeleton-reviewer`, `api-doc-manager`,
and `pre-router`.

Embedded runtime prompt bundle currently includes reviewer, implementer,
skeleton, subquery, API docs, and delegate-orientation material. Prompt bundle
hash is recorded in `agents-plugin/runtime.json`; update it after embedded
prompt changes.

## Skill Inventory

Codex-first `agents-plugin/skills/` currently uses `lead-*` names:

```text
lead-add-rule
lead-bootstrap
lead-discuss
lead-edit
lead-exit-session
lead-forge-mental-model
lead-forge-spec
lead-implement
lead-proceed
lead-salvage
lead-ship
lead-skill-authoring
lead-sprint
lead-update-spec
lead-verify-discussion
lead-workflow-manual
lead-write-code
lead-write-skeleton
lead-write-spec
lead-write-ticket
```

## Canonical Flows

```text
Full ceremony:  discuss -> proceed -> write-skeleton? -> implement -> (write-code | edit)
Direct:         implement <description>
Auto-route:     proceed <ticket-path>
Sprint:         sprint -> write-code | edit per task -> wrap-up
Recovery:       salvage -> research report -> recovery epic? -> child tickets
```

User decides next step at each handoff. `proceed` is the explicit opt-in for
auto-chaining through the pipeline.

## Specs

| File | Title | Summary |
|------|-------|---------|
| `ai-docs/spec/plugin-runtime.md` | Plugin Runtime | Codex plugin packaging, runtime metadata, launcher repair, release assets, and runtime CLI |
| `ai-docs/spec/mcp-tools.md` | MCP Tools | Host-neutral ws MCP tool contracts for context, workflow state, Git, docs, and agents |
| `ai-docs/spec/named-agent-runtime.md` | Named Agent Runtime | Durable named-agent sessions, async lifecycle, subquery fan-out, diagnostics, and Codex backend behavior |
| `ai-docs/spec/workflow-skills.md` | Workflow Skills | Codex-facing lead skills, routing, sprint work, reconstruction, utilities, and workflow primitives |
| `ai-docs/spec/documentation-system.md` | Documentation System | Project memory, conventions, specs, tickets, mental models, reference tracing, and doc workflows |
| `ai-docs/spec/api-documentation-cache.md` | API Documentation Cache | Host-neutral API documentation lookup through cached domain docs and manager sessions |
| `ai-docs/spec/claude-compatibility.md` | Claude Compatibility | Root Claude shim, agents-plugin compatibility metadata, installer behavior, and retired legacy boundaries |
| `ai-docs/spec/developer-environment-tools.md` | Developer Environment Tools | Personal bootstrap, shell/terminal/editor config, tmux helpers, statusline, and Claude TUIs |

## Tickets

Status directories: `ready/`, `todo/`, `idea/`, `.done/`, `.dropped/`.
Reference tickets by stem. This index lists active tickets only; completed or
dropped tickets live in hidden archive dirs and git history.

| Stem | Status | Summary |
|------|--------|---------|
| `260427-chore-claude-dash-windows` | ready | Verify native Windows behavior for claude-dash |
| `260512-feat-gemini-named-agent-backend` | todo | Add Gemini CLI as a ws named-agent backend |
| `260512-research-gemini-cli-stream-json` | idea | Capture Gemini CLI headless stream-json contract |
| `260504-research-durable-leaf-role-assignment` | idea | Research stricter leaf/subquery recursion control |
| `260505-bug-plugin-managed-default-root-discovery` | idea | Investigate plugin-managed default root discovery |
| `260429-research-host-neutral-ws-plugin` | idea | Host-neutral ws plugin architecture research anchor |
| `260501-research-agents-bootstrap-root-context` | idea | Agents bootstrap root context research |

## Ticket Queue

`260427-chore-claude-dash-windows` - verify Windows build/runtime behavior.

## Session Notes

Workflow documentation compression is complete:
`260504-chore-compress-workflow-docs` is closed after compressing root context,
bootstrap template, active `lead-*` skills, delegate prompts, and
`ai-docs/ref/skill-authoring.md`.

In-flight: none.
Next: choose between Windows claude-dash verification from the ready queue or
promoting accepted todo work such as skeleton contract population.

Key artifacts: `agents-plugin-tool/internal/wsagent/agent.go`,
`agents-plugin-tool/internal/mcp/server.go`,
`agents-plugin-tool/internal/wsprompt/infra/delegate-orientation.md`,
`ai-docs/ref/ws-agent-runtime.md`.

Open: verify Codex hook feedback semantics on macOS/later CLI; durable leaf role
assignment remains deferred.
