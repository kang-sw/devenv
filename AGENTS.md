# AGENTS.md - devenv

## Current Authority

`AGENTS.md` is the canonical root workflow context for this repository.
`CLAUDE.md` is a compatibility shim whose body is `@AGENTS.md`.

This repo is still mid-migration from a Claude-centered workflow to a
host-neutral Agents/open-conventions workflow. Treat these as authoritative until
a ticket replaces them:

- `AGENTS.md` - root behavioral rules and project-specific invariants.
- `ai-docs/_index.md` - project memory, inventory, specs, tickets, queue.
- `agents-plugin/` - Codex-first plugin distribution candidate.
- `agents-plugin-tool/` - native MCP/tooling source tree.
- `claude-plugin/` - stable Claude compatibility package and legacy reference.

If shared host-neutral guidance and Claude compatibility guidance conflict,
follow the more conservative rule and surface the conflict before changing
workflow semantics.

## Project Memory

Read at every session start, before other action:

1. **Preamble** - read `ai-docs/_index.md`; keep only context a session must not
   re-derive.
2. **Local** - read `ai-docs/_index.local.md` if present; it is .gitignored
   machine context.
3. **Project arc** - run `git log --oneline --graph -50`.
4. **Recent history** - run `git log -10` for `## AI Context` rationale.
5. **Migration anchor** - read
   `ai-docs/tickets/idea/260429-research-host-neutral-ws-plugin.md` when the task
   touches plugin architecture, host-neutral migration, or adapter boundaries.

## Response Discipline

- **Evidence before claims.** Run verification and read output before stating
  success.
- **No performative agreement.** Restate the requirement, verify, then act or
  push back.
- **Actions over words.** Prefer "Fixed. [what changed]" or the diff. Skip
  filler.

## Project Scope

This is a meta-workflow repo: workflow docs, skills, agents, plugin packaging,
helper commands, MCP tooling, and dev-environment templates. Specs, tickets, and
mental models here describe the workflow system itself; do not add downstream
application-domain material.

Root migration artifacts stay grouped by deliverable:

- `agents-plugin/` - Codex-first plugin distribution candidate.
- `agents-plugin-tool/` - native tooling and MCP source tree.
- `claude-plugin/` - stable Claude compatibility package.

Do not add loose root-level `cmd/`, `internal/`, `scripts/`, or language module
files for this migration unless a ticket changes the layout.

## Code Standards

1. **Simplicity.** Write the simplest complete implementation that satisfies the
   spec.
2. **Surgical changes.** Change only what the task requires; follow existing
   style.
3. **Responsibility check.** Keep module roles clean; split when responsibility
   drifts.
4. **Testability.** Prefer explicit dependencies, minimal hidden state, and pure
   logic over side effects.
5. **Skill/agent authoring.** Before editing skills, agents, prompts, or
   convention docs, read `ai-docs/ref/skill-authoring.md` and apply its invariant
   checklist to every changed Invariants/Constraints line.

## Workflow

### Approval Protocol

- **Auto-proceed:** bug fixes, pattern-following additions, tests, boilerplate,
  single-module refactors, and documentation updates that preserve existing
  semantics.
- **Ask first:** new skills/agents, cross-skill interfaces, template changes that
  affect downstream projects, convention changes, architecture changes, and
  observable workflow behavior changes.
- **Always ask:** deleting skills/agents, changing canonical flows, modifying
  migration checklist semantics, deleting functionality, or changing protocol/API
  semantics.

### Commit Rules

Auto-create one commit per logical unit unless the user asks not to commit.
Include `## AI Context` explaining why the approach was chosen.

```text
<type>(<scope>): <summary>

<what changed - brief>

## AI Context
- <decision rationale, rejected alternatives, user directives, etc.>

## Ticket Updates                          # optional - ticket-driven only
- <ticket-stem>[: <optional-label>]
  > Forward: <future-phase finding>

## Spec                                    # optional - omit when none
- <spec-stem>
```

When a spec heading `{#slug}` changes, include
`renamed-spec: <old-stem> -> <new-stem>`.

Keep unrelated untracked files out of commits. `.codex` may exist locally; do
not stage it unless explicitly requested.

### Context Window Discipline

- Source code is ground truth; load only docs relevant to the task.
- Update drifted docs on contact.

## Architecture Rules

1. **Workflow repo scope.** Specs, tickets, and mental models describe the ws
   workflow system itself; downstream application rules belong in downstream
   projects.
2. **Grouped migration layout.** `agents-plugin/` and `agents-plugin-tool/` own
   the Codex/plugin-runtime migration surface. Do not introduce new root module
   directories without a ticket.
3. **Host-neutral first.** Shared skill text should prefer canonical MCP tool
   names and host-neutral behavior. Treat Claude-specific commands and paths as
   adapter or fallback behavior.
4. **Shell state is ephemeral.** Shell state does not persist between tool calls;
   values needed later must be captured from output and passed explicitly.
5. **Windows compatibility for Claude bin additions.** Every new script added to
   `claude-plugin/bin/` must include a Windows-compatible variant (`.cmd` shim or
   equivalent) verified under both PowerShell and Cmd.

## Documentation System

- Project memory and queue: `ai-docs/_index.md`
- Tickets: `ai-docs/tickets/`
- Specs: `ai-docs/spec/`
- Mental models: `ai-docs/mental-model/`
- Static references: `ai-docs/ref/`
- Skill/agent authoring: `ai-docs/ref/skill-authoring.md`
- Codex behavior notes: `ai-docs/ref/codex-integration.md`
- MCP runtime contract: `ai-docs/ref/ws-mcp.md`

Before editing:

- Skills, agents, prompts, or convention docs: read
  `ai-docs/ref/skill-authoring.md`.
- Tickets: read ticket conventions through `ws/convention.read` or the
  compatibility fallback `claude-plugin/infra/ticket-conventions.md`.
- Specs: read spec conventions through `ws/convention.read` or the compatibility
  fallback `claude-plugin/infra/spec-conventions.md`.
- Mental models: read mental-model conventions through `ws/convention.read` or
  the compatibility fallback
  `claude-plugin/infra/mental-model-conventions.md`.

## Ticket System

Status is directory-based:

```text
ai-docs/tickets/idea/
ai-docs/tickets/todo/
ai-docs/tickets/.done/
ai-docs/tickets/.dropped/
```

- Reference tickets by stem, not path: `260429-research-host-neutral-ws-plugin`.
- Creation-date prefixes are stable; never rename to change the date.
- Move status with `git mv` when possible.
- Research tickets use freeform topic sections and no phases.
- Actionable tickets use `## Phases` and stable `### Phase N: <title>`.
- Do not edit a phase after it has a `### Result` section.
- To check ticket completion or prior phase results, use
  `git log --grep=<ticket-stem>` and inspect `## Ticket Updates`.
- Check `## Ticket Queue` in `ai-docs/_index.md` before starting a ticket.
- All AI-authored ticket content must be English.

## Project Knowledge

- **Language:** AI-authored docs, plans, commits, tickets, and code comments are
  English. Human-facing UI strings are exempt.
- Current priority is making the project and ticket system usable from
  Agents/Codex while preserving Claude compatibility.
- Research anchor: `260429-research-host-neutral-ws-plugin`. Promote or split it
  before broad structural changes.
- Existing Claude workflows still assume `ws-*` on `PATH`; prefer MCP tools for
  new shared guidance and keep CLI fallbacks documented until replacement and
  compatibility paths are complete.
- Key CLI fallbacks: `ws-print-infra`, `ws-list-mental-model`, `ws-proj-tree`,
  `ws-list-spec-stems`, `ws-generate-spec-stem`, and `ws-spec-build-index`.
- Claude plugin artifacts under `claude-plugin/` remain the compatibility
  reference; do not rewrite them just to port Codex behavior unless a ticket says
  to.

<!-- Inclusion test: if breaking this rule makes a skill produce wrong results
     AND it applies everywhere, keep it here. Domain-scoped rules belong in
     `ai-docs/mental-model/<domain>.md ## Domain Rules` via `ws:lead-add-rule`.
     Context goes in `_index.md`; process goes in skills. -->

<!-- Template Version: v0034 -->
