# AGENTS.md - [PROJECT_NAME]

## Project Memory

Read at every session start, before other action:

1. **Preamble** - read `ai-docs/_index.md`; keep only context a session must not re-derive.
2. **Local** - read `ai-docs/_index.local.md` if present; it is .gitignored machine context.
3. **Project arc** - run `git log --oneline --graph -50`.
4. **Recent history** - run `git log -10` for `## AI Context` rationale.

## Response Discipline

- **Evidence before claims.** Run verification and read output before stating success.
- **No performative agreement.** Restate the requirement, verify, then act or push back.
- **Actions over words.** Prefer "Fixed. [what changed]" or the diff. Skip filler.

## Code Standards

<!-- Project-wide code quality rules. -->

1. **Simplicity.** Write the simplest complete implementation that satisfies the spec.
2. **Surgical changes.** Change only what the task requires; follow existing style.
3. **Responsibility check.** Keep module roles clean; split when responsibility drifts.
4. **Testability.** Prefer explicit dependencies, minimal hidden state, pure logic over side effects.
5. **[Project-specific rule].** [Description.]

## Workflow

### Approval Protocol

- **Auto-proceed:** bug fixes, pattern-following additions, tests, boilerplate, single-module refactors.
- **Ask first:** new components/protocols, architecture changes, cross-module interfaces, observable behavior changes.
- **Always ask:** deleting functionality, changing protocol/API semantics, modifying persistence schema.

### Commit Rules

Auto-create one commit per logical unit. Include `## AI Context` explaining why the approach was chosen.

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

When a spec heading `{#slug}` changes, include `renamed-spec: <old-stem> -> <new-stem>`.

### Context Window Discipline

- Source code is ground truth; load only docs relevant to the task.
- Update drifted docs on contact.

## Architecture Rules

<!-- Project-wide invariants the AI must never violate. -->

1. **[Rule name].** [Rule description.]
2. **[Rule name].** [Rule description.]

<!-- Optional for GUI/TUI projects:
1. **Headless-testable architecture.** Domain logic and state live in framework-agnostic layers testable without a display. UI layers stay thin: no branching logic, state ownership, or domain knowledge.
-->

## Project Knowledge

- Project state and cross-session context live in `ai-docs/`.
- Workflow shape and plugin-less maintenance guidance live in `ai-docs/WORKFLOW.md`; it is explanatory and does not override ws runtime or MCP parser behavior.
- Before creating or editing tickets, load the write-ticket workflow skill for conventions.
- Reference tickets by stem only, never full path; stems survive status moves.
- Check `## Ticket Queue` in `ai-docs/_index.md` before starting implementation; it lists `ready/` work only.
- To check ticket completion or prior phase results, use `git log --grep=<ticket-stem>` and inspect `## Ticket Updates`.
- Claude Code compatibility is `CLAUDE.md` containing `@AGENTS.md`.
- **Language:** AI-authored docs, plans, commits, tickets, and code comments are English. Human-facing UI strings are exempt.

<!-- MIGRATION: Set up ai-docs/ for this project, then delete this block.

ai-docs/
  _index.md          - session-start context and queue
  _index.local.md    - local memory, .gitignored
  mental-model.md    - overall mental-model index
  mental-model/      - contracts, coupling, architecture narrative
  spec/              - external-perspective specs
  .old/              - tracked project archive hidden from default listings
  ref/               - static reference material
  WORKFLOW.md        - plugin-less maintenance guide
  tickets/<status>/  - idea/ todo/ ready/ .done/ .dropped/

CLAUDE.md compatibility shim:

  @AGENTS.md

_index.md should cover project summary, stack, workspace, conventions,
build/test commands, operational pitfalls, current queue, and 2-5 lines of
session notes. Do not list `.done/` or `.dropped/` tickets; use git history.

_index.md must start with:

  <!-- Memory policy: prune aggressively as project advances. Completed
       work belongs in git history, not here. Keep only what an AI session
       needs to orient itself and pick up work. If it's derivable from
       code or git log, delete it from this file. --\>

Adapt structure to the project; this is a starting point, not a schema.
-->

<!-- Inclusion test: if breaking this rule makes a skill produce wrong results
     AND it applies everywhere, keep it here. Domain-scoped rules belong in
     `ai-docs/mental-model/<domain>.md ## Domain Rules` via `ws:lead-add-rule`.
     Context goes in `_index.md`; process goes in skills. -->

<!-- MIGRATION CHECKLIST
     Template-internal. NEVER copy into a project AGENTS.md; only the Template
     Version tag belongs there. Read the tag at the bottom, apply items with
     version > current in order, then update the tag. Skip obsoleted items.
     Preserve project-specific Architecture Rules and standards; merge
     surgically and mark conflicts instead of overwriting.

- v0001: If `ai-docs/_memory.md` exists, merge useful content into `_index.md`, then delete it.
- v0002: [obsoleted]
- v0003: If tickets lack `plans:` frontmatter, add entries only for phases with existing plan docs. Keep discussion decisions in tickets; codebase-derived detail belongs in plans.
- v0004: Remove `plans:` entries with `null`; absence means "not yet created".
- v0005: Add `parent:` frontmatter for epic relationships where applicable. Epic tickets use category `epic`.
- v0006: Rename old plan paths `YYMM/DD-HHMM.<name>.md` to `YYYY-MM/DD-hhmm.<name>.md` with `git mv`.
- v0007: [obsoleted]
- v0008: [obsoleted by v0014]
- v0009: If Commit Rules lack `## Ticket Updates`, add it.
- v0010: If the Inclusion test comment above is missing, add it and keep it permanently.
- v0011: If no `<!-- Template Version: ... --\>` tag exists, review v0001-v0010, resolve applicable items, then add the tag to the managed context file.
- v0012: [obsoleted by v0014]
- v0013: Add the memory-policy comment to the top of `ai-docs/_index.md`; keep it permanently. Remove done/dropped ticket references.
- v0014: Replace session-start lines with `## Project Memory`; add `ai-docs/_index.local.md` to `.gitignore`.
- v0015: Move Project Summary, Tech Stack, and Workspace from CLAUDE.md to `_index.md`; keep CLAUDE.md behavioral.
- v0016: Add the ticket completion check rule to Project Knowledge if missing.
- v0017: Convert Project Knowledge paragraphs to bullets.
- v0018: For GUI/TUI projects, add the headless-testable Architecture Rule if missing.
- v0019: Replace per-file `ai-docs/*.local.md` ignores with `ai-docs/**/*.local.md`.
- v0020: Convert ticket `related:` list format to map format across all ticket statuses.
- v0021: If `ai-docs/mental-model/overview.md` exists, `git mv` it to `ai-docs/mental-model.md`; then run mental-model-updater to add required frontmatter to domain docs. If no `(mental-model-updated)` checkpoint exists, pass the initial commit as base. Commit with `(mental-model-updated)`.
- v0022: If flat `ai-docs/spec/` has multi-doc areas, reorganize to `ai-docs/spec/<area>/index.md` plus children; run `ws:lead-write-spec` to rebuild `features:` frontmatter.
- v0023: If Commit Rules lack `## Spec`, add it after `## Ticket Updates`; add `renamed-spec: <old-stem> -> <new-stem>`.
- v0024: Replace `[!note] Constraints` in specs: permanent invariants -> body prose; known unscheduled gaps -> `[!note] Implementation Gap · <YYYY-MM-DD>`; planned ticketed features -> `### 🚧 <Feature Name>`.
- v0025: Delete `ai-docs/_continue.local.md` if present; the removed exit-session consumer no longer reads it.
- v0026: If specs exist but no `{#YYMMDD-slug}` anchor exists, suggest `ws:lead-forge-spec`; do not edit specs automatically.
- v0027: If mental-model docs exist but embed no spec stem, suggest `ws:lead-forge-mental-model`; do not edit mental models automatically.
- v0028: Reclassify domain-scoped rules from `## Architecture Rules` or `_index.md` into `ai-docs/mental-model/<domain>.md ## Domain Rules` via `ws:lead-add-rule`.
- v0029: If `ai-docs/tickets/wip/` exists, `git mv` tickets to `todo/`, remove empty `wip/`, add `## Ticket Queue` if absent, then use `ws:lead-discuss` to agree order.
- v0030: Rename archive dirs to dot-prefix via `git mv`: `tickets/done` -> `.done`, `tickets/dropped` -> `.dropped`, `ai-docs/plans` -> `.plans`; update references.
- v0031: If `ai-docs/deps/` exists, archive it to `ai-docs/ref/deps-old`; it is superseded by `ws/api.ask` and `ai-docs/.deps/`.
- v0032: If `AGENTS.md` is absent and `CLAUDE.md` exists, create `AGENTS.md` from current `CLAUDE.md`.
- v0033: Replace `CLAUDE.md` body with `@AGENTS.md`.
- v0034: Treat `AGENTS.md` as the canonical managed template target.
- v0035: Create `ai-docs/tickets/ready/` if absent. Move existing non-`epic`, non-`research` implementation-ready tickets from `todo/` to `ready/` with `git mv` when they have spec linkage; keep `epic`, `research`, missing-spec, and uncertain tickets in `todo/`; recreate/keep an empty `todo/` directory when needed; treat `ready/` as the implementation queue and `## Ticket Queue` source; promote scoped `idea/` tickets to `todo/` through `ws:lead-discuss`.
- v0036: If `ai-docs/WORKFLOW.md` is absent, create it from the bootstrap workflow guide source. If `AGENTS.md` lacks the workflow-guide Project Knowledge bullet, add it without expanding root context into convention detail. The guide is explanatory only and does not override ws runtime or MCP parser behavior.
- v0037: Add `ai-docs/.deps/` to `.gitignore` if absent; API documentation cache contents are runtime-managed local data, not project memory.
- v0038: Create `ai-docs/.old/` as the tracked project archive for files kept only as possible future reference and hidden from default listings. Move legacy spec archives with `git mv`: `ai-docs/ref/old-spec` or `ai-docs/old-spec` -> `ai-docs/.old/spec`; move `ai-docs/old` -> `ai-docs/.old/misc` when present and not already project-specific.
-->

<!-- Template Version: v0038 -->
