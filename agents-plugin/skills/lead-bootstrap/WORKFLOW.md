# ws Workflow Guide

This guide is copied to `ai-docs/WORKFLOW.md` by bootstrap so a
maintainer can preserve the project shape when ws skills or MCP tools are not
available. It is an explanation and manual fallback only: editing this file does
not change MCP parser behavior, plugin/runtime semantics, ticket status logic,
spec indexing, or any other machine contract.

When this guide and installed ws tooling disagree, treat the installed plugin,
runtime, and bundled conventions as canonical. Update the upstream bootstrap
template rather than relying on a project-local guide override.

## Authority Files

- `AGENTS.md` is the canonical root workflow context for agents.
- `CLAUDE.md` exists only for Claude compatibility and should contain
  `@AGENTS.md` when the project has migrated to the host-neutral context.
- `ai-docs/WORKFLOW.md` is this pinned guide for plugin-less
  maintenance. Keep root context short; put durable project context in
  `ai-docs/_index.md` and workflow-system changes in upstream tooling.

## `ai-docs/` Layout

- `_index.md` is the session-start memory and active queue. Prune aggressively:
  completed work belongs in Git history, not the index.
- `_index.local.md` is machine-local memory and should be ignored by Git.
- `tickets/` stores work by status directory: `idea/`, `todo/`, `ready/`,
  `.done/`, and `.dropped/`.
- `spec/` stores caller-visible behavior specs with stable stem anchors.
- `mental-model.md` and `mental-model/` store modification-relevant operational
  knowledge and domain rules.
- `ref/` stores static references that are not active workflow state.
- `.old/` stores tracked project archive material kept only as possible future
  reference and hidden from default listings.
- `WORKFLOW.md` is this human-readable fallback guide.

## Tickets

- Reference tickets by stem, never by path; stems stay stable when tickets move
  between status directories.
- `idea/` is rough intake, `todo/` is accepted backlog, and `ready/` is the
  spec-gated implementation queue.
- `_index.md` `## Ticket Queue` lists `ready/` work only. Do not list `.done/`
  or `.dropped/` tickets there.
- Actionable tickets use `## Phases` with stable `### Phase N: <title>`
  headings. Research tickets may use freeform topic sections.
- After a phase has a `### Result` section, treat its plan text and existing
  result entries as frozen. Add later implementation tweaks as a
  `#### Edition` entry under that Result area.
- Move tickets with `git mv` when possible so history preserves status changes.

## Specs

- Specs describe caller-visible behavior, not implementation details that can
  change without changing behavior.
- Each behavior entry uses a stable `{#YYMMDD-slug}` anchor. The anchor stem is
  the identifier used in tickets, commits, and mental-model cross-references.
- Planned work uses `🚧` markers on headings or planned callouts. Remove the
  marker only after verifying the behavior is implemented.
- If stem-generation or duplicate-anchor tools are unavailable, choose a clear
  date-prefixed stem manually, search the spec tree for duplicates, and verify
  with ws tooling when it becomes available.

## Mental Models

- Mental models capture knowledge needed to safely modify the project: module
  contracts, coupling, extension recipes, common mistakes, and technical debt.
- Domain-scoped user rules belong in `## Domain Rules` inside the matching
  mental-model document, not in root `AGENTS.md`.
- If a domain has nested documents, read the parent `index.md` before any child
  document so inherited domain rules are visible.
- Include relevant spec stems in mental-model prose so future agents can trace
  operational guidance back to caller-visible behavior.

## Commit Traceability

- Every AI-authored commit should include `## AI Context` explaining why the
  approach was chosen and what alternatives or constraints mattered.
- Ticket-driven commits may include `## Ticket Updates` with forward-facing
  findings for future phases.
- Behavior-changing commits should include `## Spec` entries naming affected
  spec stems. If a spec anchor is renamed, record
  `renamed-spec: <old-stem> -> <new-stem>`.

## Manual Fallback

When ws skills, MCP tools, or Claude compatibility commands are unavailable:

1. Read `AGENTS.md`, `_index.md`, this guide, and the relevant current docs.
2. Use existing nearby tickets, specs, and mental models as formatting examples.
3. Prefer conservative, append-only changes when parser behavior is uncertain.
4. Keep generated AI docs and commit messages in English unless a human-facing
   product string requires another language.
5. Verify with plain Git and shell commands, then re-run ws verification tools
   when they become available.
