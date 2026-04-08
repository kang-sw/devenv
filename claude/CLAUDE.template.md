# CLAUDE.md — [PROJECT_NAME]

Read `ai-docs/_index.md` at session start for project context, conventions, and build/test commands.

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
4. **[Project-specific rule].** [Description.]

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

### Session Start

- Run `git log -10` for recent changes. (without `--oneline`!)

### Context Window Discipline

- Source code is ground truth; load only docs relevant to the current task. Update drifted docs on contact.

---

## Project Summary

<!-- One paragraph: what the project is, who it's for, current milestone/target. -->

**[PROJECT_NAME]** — [brief description].

## Tech Stack

<!-- List primary languages, frameworks, key libraries. -->

[Language] + [Framework/Engine]. Key libs: [lib1], [lib2], [lib3].

## Workspace

<!-- Describe top-level directories and their roles. -->

```
[dir1]/   — [purpose]
[dir2]/   — [purpose]
[dir3]/   — [purpose]
```

## Architecture Rules

<!-- Project-specific invariants the AI must never violate. -->

1. **[Rule name].** [Rule description.]
2. **[Rule name].** [Rule description.]

## Project Knowledge

Project state and cross-session context live in `ai-docs/`.
Before creating or editing tickets, load `/write-ticket` for conventions.
Reference tickets by **stem only** (e.g., `260115-feat-foo-bar`), never by
full path — stems stay stable across status moves.
When starting work on a ticket, move it to `wip/` immediately.

**Language:** All AI-authored artifacts — documents, plans, commit messages, ticket entries,
and inline code comments — must be in English regardless of conversation language.
Human-facing UI strings are exempt.

<!-- MIGRATION: Set up ai-docs/ for this project, then delete this block.

ai-docs/
  _index.md          — single session-start read; project context
  mental-model/      — project map: contracts, coupling, architectural narrative
  deps/              — external library API delta docs
  ref/               — static reference material (external specs, protocol docs, design notes)
  tickets/<status>/  — idea/ todo/ wip/ done/ dropped/

_index.md should cover:
  - Architecture (module/directory map, relationships)
  - Conventions (tickets, dependency docs, naming rules)
  - Build/test commands and operational pitfalls
  - Session notes (cross-session intent only, 2-5 lines max, delete when stale)

Adapt structure to fit the project — these are guidelines, not a rigid schema.
-->

<!-- Inclusion test: if breaking this rule makes a skill produce
     wrong results, it belongs here. Everything else goes in
     _index.md (context) or skills (process). -->

<!-- MIGRATION CHECKLIST
     Read the Template Version tag at the bottom of this file.
     Apply all items with version > current, in order. Then update the tag.
     Items marked [obsoleted by vNNNN] — skip entirely.
     Project-specific content (Architecture Rules, custom standards) must
     survive — merge surgically, flag conflicts rather than overwriting.

- v0001: If `ai-docs/_memory.md` exists, merge useful content into
         `ai-docs/_index.md` and delete `_memory.md`.
- v0002: If `ai-docs/spec/` exists, merge architectural narrative and
         design rationale into `ai-docs/mental-model/` docs. Remove API
         signatures, struct layouts, and status tracking (already in
         source/tickets). Delete `ai-docs/spec/` after migration.
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
- v0008: If Session Start uses `git log --oneline`, remove the `--oneline`
         flag. Full commit messages include AI Context sections.
- v0009: If Commit Rules lack `## Ticket Updates`, add it. Ticket-driven
         commits must record the stem and forward-facing findings.
- v0010: If the `<!-- Inclusion test: ... -->` comment block is missing
         above this checklist, add it. Do not remove after migration —
         permanent authoring guardrail.
- v0011: Template version tracking starts here. If the project has no
         `<!-- Template Version: ... -->` tag, review v0001-v0010 against
         the current project state to determine which items still apply.
         After resolving, add the tag at the bottom of CLAUDE.md.
-->

<!-- Template Version: v0011 -->
