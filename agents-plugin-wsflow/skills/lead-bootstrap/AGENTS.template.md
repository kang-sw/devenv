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
- Workflow shape and plugin-less maintenance guidance live in `ai-docs/WORKFLOW.md`; it is explanatory and does not override wsflow runtime or wsflow MCP parser behavior.
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
  mental-model.md    - overall mental-model index and optional project reading map
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
Deep source narratives, behavior inventories, extension recipes, dependency
notes, stable project reading maps, and completed history are scope-drift
candidates; keep only compact pointers here and route semantic extraction
through the owning workflow.

_index.md must start with:

  <!-- Memory policy: prune aggressively as project advances. Completed
       work belongs in git history, not here. Keep only what an AI session
       needs to orient itself and pick up work. If it's derivable from
       code or git log, delete it from this file. --\>

Adapt structure to the project; this is a starting point, not a schema.
-->

<!-- Inclusion test: if breaking this rule makes a skill produce wrong results
     AND it applies everywhere, keep it here. Domain-scoped rules belong in
     `ai-docs/mental-model/<domain>.md ## Domain Rules` via `wsflow:lead-add-rule`.
     Context goes in `_index.md`; process goes in skills. -->

<!-- MIGRATION CHECKLIST
     Template-internal. NEVER copy into a project AGENTS.md; only the Template
     Version tag belongs there. Read the tag at the bottom, apply items with
     version > current in order, then update the tag.
     This template has package-local version history; apply only entries listed here.
     Preserve project-specific Architecture Rules and standards; merge
     surgically and mark conflicts instead of overwriting.

- v0001: Align `AGENTS.md` with the initial wsflow baseline: Project Memory,
  Response Discipline, Workflow, Architecture Rules, Project Knowledge, the
  inclusion-test comment, and the Template Version tag.
- v0001: Ensure `CLAUDE.md` contains `@AGENTS.md`.
- v0001: Ensure `ai-docs/WORKFLOW.md` exists from the bootstrap workflow guide.
- v0001: Ensure `ai-docs/` has `_index.md`, `mental-model.md`,
  `mental-model/`, `spec/`, `ref/`, `.old/`, and tickets status directories:
  `idea/`, `todo/`, `ready/`, `.done/`, `.dropped/`.
- v0001: Ensure `.gitignore` includes `ai-docs/**/*.local.md` and
  `ai-docs/.deps/`.
- v0002: If `ai-docs/_index.md` looks like an old all-in-one
  architecture digest, report an index health note and ask whether to clean up
  `_index.md`. Do not move semantic content into specs or mental models from
  bootstrap; compact `_index.md` only after user approval and only when an
  owning document already preserves the meaning.
- v0003: Treat stable task/topic -> document routing maps as candidates for
  `ai-docs/mental-model.md ## Project Reading Map` during later
  mental-model work. Bootstrap may report the drift, but must not move mixed
  status or feature inventory automatically.
-->

<!-- Template Version: v0003 -->
