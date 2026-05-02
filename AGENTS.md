# AGENTS.md - devenv

## Current Migration State

This repository is in transition from a Claude-centered workflow to a
host-neutral Agents/open-conventions workflow.

The durable workflow knowledge still lives primarily in:

- `CLAUDE.md` for repository-level operating rules.
- `ai-docs/_index.md` for project memory, inventory, specs, tickets, and current queue.
- `claude-plugin/infra/` for convention documents.
- `claude-plugin/skills/` for workflow skills.
- `claude-plugin/bin/ws-*` for helper commands used by the skills.

Treat those files as authoritative until a ticket explicitly replaces them with
host-neutral equivalents. Do not assume the migration is complete just because
this file exists.

## Session Start

At the start of a session, load context in this order:

1. Read `ai-docs/_index.md`.
2. Read `ai-docs/_index.local.md` if it exists.
3. Run `git log --oneline --graph -50`.
4. Run `git log -10`.
5. For this migration branch, read
   `ai-docs/tickets/idea/260429-research-host-neutral-ws-plugin.md`.

Use `CLAUDE.md` as the compatibility source for existing repository rules when
this file is incomplete. If `AGENTS.md` and `CLAUDE.md` conflict, prefer the
more conservative rule and surface the conflict before changing workflow
semantics.

## Project Scope

This is a meta-workflow repository. It defines workflow documents, skills,
agents, plugin packaging, helper commands, and development environment templates.

Domain specs, tickets, and mental models in this repository describe the
workflow system itself. Do not add downstream application-domain material here.

## Documentation System

Use these locations as the current document system:

- Project memory and inventory: `ai-docs/_index.md`
- Tickets: `ai-docs/tickets/`
- Specs: `ai-docs/spec/`
- Mental models: `ai-docs/mental-model/`
- Conventions: `claude-plugin/infra/*-conventions.md`
- Skill and agent authoring rules: `ai-docs/ref/skill-authoring.md`
- Codex behavior notes: `ai-docs/ref/codex-integration.md`

Before authoring or auditing any skill, agent prompt, or convention document,
read `ai-docs/ref/skill-authoring.md`.

Before editing tickets, read `claude-plugin/infra/ticket-conventions.md`.
Before editing specs, read `claude-plugin/infra/spec-conventions.md`.
Before editing mental models, read
`claude-plugin/infra/mental-model-conventions.md`.

## Ticket System

Ticket status is directory-based:

```text
ai-docs/tickets/idea/
ai-docs/tickets/todo/
ai-docs/tickets/wip/
ai-docs/tickets/.done/
ai-docs/tickets/.dropped/
```

Use ticket stems as stable references, not full paths. A ticket stem is the file
name without `.md`, for example `260429-research-host-neutral-ws-plugin`.

Creation-date prefixes are stable. Do not rename a ticket to change its date.
Move status changes with `git mv` when possible.

Research tickets have freeform topic sections and no phases. Actionable tickets
use `## Phases` with stable `### Phase N: <title>` sections. Do not edit a phase
after it has a `### Result` section.

All AI-authored ticket content must be in English.

## Current Migration Priority

The current priority is to make the project and ticket system usable from
Agents/Codex before broad skill migration:

1. Establish `AGENTS.md` as the honest root context for the current mixed state.
2. Preserve Claude compatibility while moving durable rules toward shared,
   host-neutral conventions.
3. Prefer precise MCP tool/resource names in future shared skill text instead of
   vague capability descriptions.
4. Keep Claude-specific commands and path assumptions as adapter or fallback
   behavior, not as the long-term shared contract.

The current research anchor is
`260429-research-host-neutral-ws-plugin`. Promote or split that research into
actionable tickets before making broad structural changes.

## Helper Commands

Many existing workflows still assume `ws-*` commands are available on `PATH`.
Important current helpers include:

- `ws-print-infra <doc>`: print convention or infra documents.
- `ws-list-mental-model [paths...]`: list relevant mental-model documents.
- `ws-proj-tree`: render a project map for discussion.
- `ws-list-spec-stems [spec-file]`: list spec anchors.
- `ws-generate-spec-stem <slug>`: generate a collision-free spec stem.
- `ws-spec-build-index`: rebuild spec metadata and run spec checks.

During the open-conventions migration, prefer designing shared skill text around
canonical MCP names, with these CLI helpers retained as Claude-compatible
fallbacks. Do not remove a CLI fallback until the relevant MCP replacement and
Claude compatibility path are documented.

## Implementation Discipline

Follow the existing repository discipline:

- Evidence before claims: run verification commands and read output before
  reporting success.
- Keep edits scoped to the requested workflow or ticket.
- Do not revert user changes or unrelated worktree changes.
- All AI-authored artifacts in this repository must be in English, including
  docs, comments, and commit messages.
- When touching `claude-plugin/skills/`, `claude-plugin/infra/`, or
  `claude-plugin/infra/prompts/`, apply the checklist in
  `ai-docs/ref/skill-authoring.md`.

## Commit Rules

Create commits for logical units of work unless the user asks not to commit.
Include an `## AI Context` section in each commit message explaining why the
approach was chosen and any compatibility trade-offs.
Write `## AI Context` with enough detail for future agents to recover the
reasoning from git history: include the user intent, rejected alternatives,
verification limits, and migration or compatibility implications when relevant.
Prefer a heredoc or temporary commit-message file over long `git commit -m`
chains when writing multi-paragraph commit messages, so paragraph spacing remains
readable in git history.

Keep unrelated untracked files out of commits. In this workspace, `.codex` may
exist as an untracked local file; do not stage it unless explicitly requested.
