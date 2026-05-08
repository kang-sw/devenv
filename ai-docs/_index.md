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

Stable Claude package: `claude-plugin/` (`ws@0.15.0`).
Codex-first candidate: `agents-plugin/` (`ws@0.20.2`).
Native MCP/tooling source: `agents-plugin-tool/`.

## Current Branch Rules

- Branch: `main`.
- No branch-specific spec or mental-model freeze is active.
- Keep `.codex` untracked unless the user explicitly asks to stage it.

## Plugin Topology

- Claude runtime uses `claude-plugin/`; after edits there, run
  `claude plugin update ws@ws`.
- `./install.sh update` handles first-time install and settings patching on a
  new machine.
- `claude-plugin/CLAUDE.home.md` is the repo copy of `~/.claude/CLAUDE.md`.
- External Claude install: `/plugin marketplace add kang-sw/devenv`, then
  `/plugin install ws@ws`.
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
  `agents-plugin` remains manual closeout.

## Read Before Editing

| File | Use |
|------|-----|
| `ai-docs/ref/skill-authoring.md` | Skill/agent/prompt/convention authoring rules |
| `ai-docs/ref/codex-integration.md` | Probed Codex CLI behavior |
| `ai-docs/ref/ws-mcp.md` | MCP process, tools, CLI fallbacks, verification levels |
| `ai-docs/ref/ws-agent-runtime.md` | Durable agent runtime contract |
| `ai-docs/ship/ws.md` | Release process for `ws` |
| `claude-plugin/infra/impl-playbook.md` | Implementation discipline |
| `claude-plugin/infra/subagent-rules.md` | Subagent dispatch rules |
| `claude-plugin/infra/executor-wrapup.md` | Shared post-implementation wrapup |

Before editing tickets/specs/mental models, read the matching convention doc in
`claude-plugin/infra/`. Before editing skill, agent, prompt, or convention text,
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
  `ws/agents.cancel`, `ws/agents.print`, `ws/agents.erase`
- API docs: `ws/api.list`, `ws/api.ask`

Shared `agents-plugin` skill text uses MCP names, not repo-local
`claude-plugin/infra/*` paths. Convention text is bundled into the runtime and
read through `ws/convention.read`.

CLI fallbacks still exist under `claude-plugin/bin/` for Claude compatibility:
`ws-proj-tree`, `ws-print-infra`, `ws-list-mental-model`,
`ws-list-spec-stems`, `ws-generate-spec-stem`, `ws-spec-build-index`,
`ws-review-path`, `ws-oneshot-agent`, `ws-named-agent`, `ws-ask-api`, and shims.

## MCP Runtime Notes

`agents-plugin-tool/cmd/ws-mcp` provides stdio MCP, `version`, and `doctor`.
Runtime binaries are plugin cache-local under
`agents-plugin/.runtime/<os>-<arch>/ws-mcp[.exe]`.

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

Claude prompt documents live in `claude-plugin/infra/prompts/` and are invoked
through `ws-oneshot-agent -p <stem>` or `ws-named-agent new -p <stem>`.

Key prompts: `clerk`, `code-reviewer`, `code-review-correctness`,
`code-review-fit`, `code-review-test`, `mental-model-updater`,
`spec-updater`, `project-survey`, `sprint-survey`, `implementer`,
`searcher`, `skeleton-writer`, `subquery`, `api-doc-manager`, `pre-router`.

Embedded runtime prompt bundle currently includes reviewer, implementer,
skeleton, subquery, API docs, and delegate-orientation material. Prompt bundle
hash is recorded in `agents-plugin/runtime.json`; update it after embedded
prompt changes.

## Skill Inventory

Claude package skills remain under `claude-plugin/skills/` as the compatibility
reference.

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
lead-ship
lead-skill-authoring
lead-sprint
lead-update-spec
lead-workflow-manual
lead-write-code
lead-write-skeleton
lead-write-spec
lead-write-ticket
```

`manual-think` remains Claude-only while `claude-plugin/` exists as the
compatibility tree.

## Canonical Flows

```text
Full ceremony:  discuss -> proceed -> write-skeleton? -> implement -> (write-code | edit)
Direct:         implement <description>
Auto-route:     proceed <ticket-path>
Sprint:         sprint -> write-code | edit per task -> wrap-up
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
| `ai-docs/spec/claude-compatibility.md` | Claude Compatibility | Claude shims, legacy plugin package, CLI fallbacks, installer behavior, and Windows shims |
| `ai-docs/spec/developer-environment-tools.md` | Developer Environment Tools | Personal bootstrap, shell/terminal/editor config, tmux helpers, statusline, and Claude TUIs |

## Tickets

Status directories: `ready/`, `todo/`, `idea/`, `.done/`, `.dropped/`.
Reference tickets by stem. This index lists active tickets only; completed or
dropped tickets live in hidden archive dirs and git history.

| Stem | Status | Summary |
|------|--------|---------|
| `260503-feat-agents-plugin-runtime-boundary` | ready | Go stdio MCP/runtime boundary; Python launcher smoke in progress |
| `260506-bug-ws-mcp-launcher-startup-delay` | ready | Launcher hot-path cache and runtime capabilities probe done; timeout mitigation remains optional follow-up |
| `260505-feat-agent-backend-failure-diagnostics` | ready | Improve named-agent backend failure diagnostics |
| `260505-bug-codex-jsonl-trailing-noise` | ready | Fix Windows Codex persistent-agent trailing stdout parsing |
| `260429-feat-api-deps` | ready | `ws-ask-api` 2-layer API doc cache |
| `260427-chore-claude-dash-windows` | ready | Verify native Windows behavior for claude-dash |
| `260508-feat-api-ask-async-jobs` | todo | Add a separate async job surface for long-running API documentation lookups |
| `260508-chore-lightweight-epic-tickets` | todo | Keep epic tickets as milestone boards and move detail into child tickets |
| `260508-chore-skill-description-attention-policy` | todo | Tune skill descriptions by entry-point strength and explicit rule persistence intent |
| `260503-epic-ws-agent-workflow-stability` | todo | Named-agent workflow stabilization parent; active blockers closed |
| `260504-research-durable-leaf-role-assignment` | idea | Research stricter leaf/subquery recursion control |
| `260505-bug-plugin-managed-default-root-discovery` | idea | Investigate plugin-managed default root discovery |
| `260429-research-host-neutral-ws-plugin` | idea | Host-neutral ws plugin architecture research anchor |
| `260501-research-agents-bootstrap-root-context` | idea | Agents bootstrap root context research |

## Ticket Queue

`260503-feat-agents-plugin-runtime-boundary` - macOS/Codex launcher, release download, and Windows Go runtime smoke are verified; Python launcher is the current shared startup path and needs installed-cache verification on Windows after Python 3 is available.
`260506-bug-ws-mcp-launcher-startup-delay` - Phases 1, 2, and 3 are done; Phase 4 startup timeout mitigation remains optional upgrade-buffer scope after release validation.
`260505-feat-agent-backend-failure-diagnostics` - improve named-agent backend invocation failures with raw errors, PATH-detected backend hints, and explicit reconfiguration guidance.
`260505-bug-codex-jsonl-trailing-noise` - hotfix Windows Codex persistent-agent results when process-control stdout appears after a valid JSONL agent message.
`260429-feat-api-deps` - API docs cache; phases: manager prompt -> pre-router -> bin tools -> workflow integration.
`260427-chore-claude-dash-windows` - verify Windows build/runtime behavior.

## Session Notes

Workflow documentation compression is complete:
`260504-chore-compress-workflow-docs` is closed after compressing root context,
bootstrap template, active `lead-*` skills, delegate prompts, and
`ai-docs/ref/skill-authoring.md`.

In-flight: none.
Next: verify native Windows plugin-managed Python launcher startup under
`260503-feat-agents-plugin-runtime-boundary`, then prepare a patch release if
the installed-cache smoke passes.

Key artifacts: `agents-plugin-tool/internal/wsagent/agent.go`,
`agents-plugin-tool/internal/mcp/server.go`,
`agents-plugin-tool/internal/wsprompt/infra/delegate-orientation.md`,
`ai-docs/ref/ws-agent-runtime.md`.

Open: verify Codex hook feedback semantics on macOS/later CLI; durable leaf role
assignment remains deferred.
