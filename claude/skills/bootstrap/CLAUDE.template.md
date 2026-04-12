# CLAUDE.md — [PROJECT_NAME]

## Project Memory

Read in this order at every session start, before any other action:

1. **Preamble** — read `ai-docs/_index.md`. Project-level truth that no
   session should re-derive. Prune aggressively: if derivable from code
   or commit history, delete.
2. **Local** — read `ai-docs/_index.local.md` if it exists. .gitignored.
   Machine-bound context (paths, env vars, build config) and personal
   session notes.
3. **Project arc** — run `git log --oneline --graph -50`. Trajectory and
   topic clusters at a glance.
4. **Recent history** — run `git log -10`. Decision rationale via AI Context
   sections. Fades as history grows.

## Response Discipline

- **Evidence before claims.** Run verification commands and read output before
  stating success. Never use "should pass", "probably works", or "looks correct."
- **No performative agreement.** Never respond with "Great point!", "You're
  absolutely right!", or similar. Restate the technical requirement, verify
  against the codebase, then act (or push back with reasoning).
- **Actions over words.** "Fixed. [what changed]" or just show the diff.
  Skip gratitude expressions and filler.

## Code Standards

<!-- Principles governing code quality and style. -->

1. **Simplicity.** Write the simplest code that works. Implement fully when the spec is
   clear — judge scope by AI effort, not human-hours.
2. **Surgical changes.** Change only what the task requires. Follow existing style. Every
   changed line must trace to the request.
3. **Responsibility check.** As you implement, ask whether each change
   keeps the module's role clean. Split when responsibility drifts.
4. **Testability.** Prefer designs that are straightforward to test —
   explicit dependencies, minimal hidden state, pure logic over side effects.
5. **[Project-specific rule].** [Description.]

## Workflow

### Approval Protocol

- **Auto-proceed**: Bug fixes, pattern-following additions, test code, boilerplate,
  refactoring within a single module.
- **Ask first**: New component/protocol additions, architectural changes,
  cross-module interface changes, anything that changes observable behavior.
- **Always ask**: Deleting existing functionality, changing protocol/API semantics,
  modifying persistence schema.

### Commit Rules

Auto-create git commits, each covering one logical unit of change.
Include an **AI context** section in every commit message recording design decisions,
alternatives considered, and trade-offs — focus on _why_ this approach was chosen.

```
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives, etc.>

## Ticket Updates                          # optional — only when ticket-driven
- <ticket-stem>[: <optional-label>]
  > Forward: <what future phases must know>
```

### Context Window Discipline

- Source code is ground truth; load only docs relevant to the current task. Update drifted docs on contact.

## Architecture Rules

<!-- Project-specific invariants the AI must never violate. -->

1. **[Rule name].** [Rule description.]
2. **[Rule name].** [Rule description.]

<!-- Optional — enable for projects with a GUI/TUI presentation layer:
1. **Headless-testable architecture.** All domain logic and state must live in
   framework-agnostic layers testable without a display. UI layers are thin
   adapters: no branching logic, no state ownership, no domain knowledge.
-->

## Project Knowledge

- Project state and cross-session context live in `ai-docs/`.
- Before creating or editing tickets, load `/write-ticket` for conventions.
- Reference tickets by **stem only** (e.g., `260115-feat-foo-bar`), never by
  full path — stems stay stable across status moves.
- When starting work on a ticket, move it to `wip/` immediately.
- To check ticket completion or prior phase results, use `git log --grep=<ticket-stem>`
  and look for `## Ticket Updates` sections in matching commits.
- **Language:** All AI-authored artifacts — documents, plans, commit messages, ticket entries,
  and inline code comments — must be in English regardless of conversation language.
  Human-facing UI strings are exempt.

<!-- MIGRATION: Set up ai-docs/ for this project, then delete this block.

ai-docs/
  _index.md          — single session-start read; project context
  _index.local.md    — local memory (free-form, .gitignored)
  mental-model/      — project map: contracts, coupling, architectural narrative
  deps/              — external library API delta docs
  ref/               — static reference material (external specs, protocol docs, design notes)
  tickets/<status>/  — idea/ todo/ wip/ done/ dropped/

_index.md should cover:
  - Project summary (what it is, who it's for, current milestone)
  - Tech stack (languages, frameworks, key libraries)
  - Workspace (top-level directories and their roles)
  - Conventions (tickets, dependency docs, naming rules)
  - Build/test commands and operational pitfalls
  - Session notes (cross-session intent only, 2-5 lines max, delete when stale)
  - Never reference done/ or dropped/ tickets — they live in git history

_index.md must start with this comment (do not remove after creation):

  <!-- Memory policy: prune aggressively as project advances. Completed
       work belongs in git history, not here. Keep only what an AI session
       needs to orient itself and pick up work. If it's derivable from
       code or git log, delete it from this file. --\>

Adapt structure to fit the project — these are guidelines, not a rigid schema.
-->

<!-- Inclusion test: if breaking this rule makes a skill produce
     wrong results, it belongs here. Everything else goes in
     _index.md (context) or skills (process). -->

<!-- MIGRATION CHECKLIST
     This block is template-internal tooling. NEVER copy it into a
     project's CLAUDE.md — only the Template Version tag belongs there.
     Read the Template Version tag at the bottom of this file.
     Apply all items with version > current, in order. Then update the tag.
     Items marked [obsoleted by vNNNN] — skip entirely.
     Project-specific content (Architecture Rules, custom standards) must
     survive — merge surgically, flag conflicts rather than overwriting.

- v0001: If `ai-docs/_memory.md` exists, merge useful content into
         `ai-docs/_index.md` and delete `_memory.md`.
- v0002: [obsoleted]
- v0003: If tickets lack `plans:` frontmatter, add it (only phases with
         existing plan documents — no null placeholders). Audit phase
         content: discussion decisions stay in tickets, codebase-derived
         details belong in plans.
- v0004: If tickets have `plans:` entries with `null` values, remove them.
         Absence means "not yet created".
- v0005: If tickets lack `parent:` frontmatter for epic relationships, add
         it where applicable. Epic tickets use category `epic`.
- v0006: If plan paths use old format (`YYMM/DD-HHMM.<name>.md`), rename
         to `YYYY-MM/DD-hhmm.<name>.md` via `git mv`.
- v0007: If `ai-docs/list-active.sh` does not exist, create it:
         ```bash
         #!/usr/bin/env bash
         find ai-docs -type f -name '*.md' \
           ! -path '*/tickets/done/*' \
           ! -path '*/tickets/dropped/*' \
           ! -path '*/plans/*' \
           | sort
         ```
         Then `chmod +x ai-docs/list-active.sh` and commit.
- v0008: [obsoleted by v0014]
- v0009: If Commit Rules lack `## Ticket Updates`, add it. Ticket-driven
         commits must record the stem and forward-facing findings.
- v0010: If the `<!-- Inclusion test: ... --\>` comment block is missing
         above this checklist, add it. Do not remove after migration —
         permanent authoring guardrail.
- v0011: Template version tracking starts here. If the project has no
         `<!-- Template Version: ... --\>` tag, review v0001-v0010 against
         the current project state to determine which items still apply.
         After resolving, add the tag at the bottom of CLAUDE.md.
- v0012: [obsoleted by v0014]
- v0013: Add the memory-policy comment to the top of `ai-docs/_index.md`
         (see MIGRATION block for exact text). Do not remove after adding —
         permanent pruning guardrail. Remove any references to done/ or
         dropped/ tickets from `_index.md`.
- v0014: Replace the session-start `_index.md` / `_index.local.md` lines
         with the `## Project Memory` section (see template). Remove
         Session Start section — it is now part of Project Memory.
         Add `_index.local.md` to `.gitignore` if not already present.
- v0015: Move Project Summary, Tech Stack, and Workspace sections from
         CLAUDE.md to `ai-docs/_index.md`. Remove the `---` separator
         if it was used to divide behavioral/contextual sections.
         CLAUDE.md keeps only behavioral rules (Architecture Rules,
         Project Knowledge). Context lives in `_index.md`.
- v0016: If Project Knowledge lacks a ticket completion check rule, add:
         "To check ticket completion or prior phase results, use
         `git log --grep=<ticket-stem>` and look for `## Ticket Updates`
         sections in matching commits."
- v0017: If Project Knowledge items are plain paragraphs, convert to a
         bulleted list (`- ` prefix per item) for readability.
- v0018: If the project has a GUI/TUI presentation layer and Architecture
         Rules lacks a headless-testable rule, add:
         `**Headless-testable architecture.** All domain logic and state
         must live in framework-agnostic layers testable without a display.
         UI layers are thin adapters: no branching logic, no state
         ownership, no domain knowledge.`
- v0019: Replace any explicit per-file `.local.md` gitignore entries
         under `ai-docs/` with a single glob: `ai-docs/**/*.local.md`.
         Covers existing files like `_index.local.md` and session-scratch
         files like `_continue.local.md` without per-file maintenance.
- v0020: If `related:` entries use list format (`- stem  # comment`),
         convert to map format (`stem: comment`) across all tickets in
         `ai-docs/tickets/` (all statuses). Empty lists (`related: []`)
         may be removed or left as-is — the map script handles both.
         See `claude/skills/discuss/list-active.py` for canonical parser.
- v0021: If `ai-docs/mental-model/overview.md` exists, promote it to the
         top-level index: `git mv ai-docs/mental-model/overview.md ai-docs/mental-model.md`.
         Then run `/write-mental-model` to add frontmatter (`domain`, `description`,
         `sources`, `related`) to all existing domain docs that lack it.
         Commit all changes with `(mental-model-updated)` in the message body.
-->

<!-- Template Version: v0021 -->
