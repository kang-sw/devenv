# AGENTS.md - devenv

## Current Authority

This repo is mid-migration from Claude-centered workflow to host-neutral
Agents/open-conventions workflow. Treat these as authoritative until a ticket
replaces them:

- `CLAUDE.md` - compatibility rules and legacy repo discipline.
- `ai-docs/_index.md` - project memory, inventory, specs, tickets, queue.
- `claude-plugin/infra/` - convention documents.
- `claude-plugin/skills/` - legacy workflow skills.
- `claude-plugin/bin/ws-*` - helper command fallbacks.

If `AGENTS.md` and `CLAUDE.md` conflict, follow the more conservative rule and
surface the conflict before changing workflow semantics.

## Session Start

Load context in order:

1. Read `ai-docs/_index.md`.
2. Read `ai-docs/_index.local.md` if it exists.
3. Run `git log --oneline --graph -50`.
4. Run `git log -10`.
5. Read `ai-docs/tickets/idea/260429-research-host-neutral-ws-plugin.md`.

## Project Scope

This is a meta-workflow repo: workflow docs, skills, agents, plugin packaging,
helper commands, and dev-environment templates. Specs, tickets, and mental
models here describe the workflow system itself; do not add downstream
application-domain material.

Root migration artifacts stay grouped by deliverable:

- `agents-plugin/` - Codex-first plugin distribution candidate.
- `agents-plugin-tool/` - native tooling and MCP source tree.

Do not add loose root-level `cmd/`, `internal/`, `scripts/`, or language module
files for this migration unless a ticket changes the layout.

## Documentation System

- Project memory and queue: `ai-docs/_index.md`
- Tickets: `ai-docs/tickets/`
- Specs: `ai-docs/spec/`
- Mental models: `ai-docs/mental-model/`
- Conventions: `claude-plugin/infra/*-conventions.md`
- Skill/agent authoring: `ai-docs/ref/skill-authoring.md`
- Codex behavior notes: `ai-docs/ref/codex-integration.md`

Before editing:

- Skills, agents, or convention docs: read `ai-docs/ref/skill-authoring.md`.
- Tickets: read `claude-plugin/infra/ticket-conventions.md`.
- Specs: read `claude-plugin/infra/spec-conventions.md`.
- Mental models: read `claude-plugin/infra/mental-model-conventions.md`.

## Ticket System

Status is directory-based:

```text
ai-docs/tickets/idea/
ai-docs/tickets/todo/
ai-docs/tickets/wip/
ai-docs/tickets/.done/
ai-docs/tickets/.dropped/
```

- Reference tickets by stem, not path: `260429-research-host-neutral-ws-plugin`.
- Creation-date prefixes are stable; never rename to change the date.
- Move status with `git mv` when possible.
- Research tickets use freeform topic sections and no phases.
- Actionable tickets use `## Phases` and stable `### Phase N: <title>`.
- Do not edit a phase after it has a `### Result` section.
- All AI-authored ticket content must be English.

## Migration Priority

Current priority: make the project and ticket system usable from Agents/Codex
while preserving Claude compatibility.

- Keep `AGENTS.md` honest about the mixed state.
- Move durable rules toward shared, host-neutral conventions.
- Prefer precise MCP tool/resource names in shared skill text.
- Treat Claude-specific commands and paths as adapter or fallback behavior.

Research anchor: `260429-research-host-neutral-ws-plugin`. Promote or split it
before broad structural changes.

## Helper Commands

Existing workflows still assume `ws-*` on `PATH`. Key fallbacks:

- `ws-print-infra <doc>` - print convention or infra docs.
- `ws-list-mental-model [paths...]` - list relevant mental-model docs.
- `ws-proj-tree` - render project map.
- `ws-list-spec-stems [spec-file]` - list spec anchors.
- `ws-generate-spec-stem <slug>` - mint a collision-free spec stem.
- `ws-spec-build-index` - rebuild spec metadata and run checks.

Shared skill text should prefer canonical MCP names. Keep CLI fallbacks until the
MCP replacement and Claude compatibility path are documented.

## Implementation Discipline

- Evidence before claims: run verification and read output before reporting
  success.
- Keep edits scoped to the requested workflow or ticket.
- Do not revert user changes or unrelated worktree changes.
- All AI-authored artifacts here must be English, including docs, comments, and
  commit messages.
- When touching `claude-plugin/skills/`, `claude-plugin/infra/`, or
  `claude-plugin/infra/prompts/`, apply `ai-docs/ref/skill-authoring.md`.

## Commit Rules

Create commits for logical units unless the user asks not to commit.

Every commit message includes `## AI Context` with user intent, approach,
rejected alternatives, verification limits, and migration or compatibility
trade-offs when relevant. Prefer a heredoc or temporary commit-message file over
long `git commit -m` chains.

Keep unrelated untracked files out of commits. `.codex` may exist locally; do not
stage it unless explicitly requested.
