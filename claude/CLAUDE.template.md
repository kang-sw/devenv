<!-- AI-maintained project state — read before work, update after -->
<!-- - `ai-docs/_index.md` — architecture, conventions, build/test, session notes -->

# CLAUDE.md — [PROJECT_NAME]

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

---

## Project Knowledge

Project state and cross-session context live in `ai-docs/`.
Read `_index.md` at session start.
Before creating or editing tickets, load `/write-ticket` for conventions.
Reference tickets by **stem only** (e.g., `260115-feat-foo-bar`), never by
full path — stems stay stable across status moves.
When starting work on a ticket, move it to `wip/` immediately.

**Language:** All AI-authored artifacts — documents, plans, commit messages, ticket entries,
`### Result` entries, and inline code comments — must be in English regardless of
conversation language. Human-facing UI strings are exempt.

<!-- MIGRATION: Set up ai-docs/ for this project, then delete this block.

ai-docs/
  _index.md          — single session-start read; project context
  spec/              — external-facing feature specs (what the project does)
  mental-model/      — architecture docs, regenerable from source
  deps/              — external library API delta docs
  ref/               — static reference material (external specs, protocol docs, design notes)
  tickets/<status>/  — idea/ todo/ wip/ done/ dropped/

_index.md should cover:
  - Architecture (module/directory map, relationships)
  - Spec index (list spec files with one-line summaries)
  - Conventions (tickets, dependency docs, naming rules)
  - Build/test commands and operational pitfalls
  - Session notes (cross-session intent only, 2-5 lines max, delete when stale)

Adapt structure to fit the project — these are guidelines, not a rigid schema.
-->

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
```

### Session Start

- Read `ai-docs/_index.md` for project context.
- Run `git log -10` for recent changes.

### Response Discipline

- **Evidence before claims.** Run verification commands and read output before
  stating success. Never use "should pass", "probably works", or "looks correct."
- **No performative agreement.** Never respond with "Great point!", "You're
  absolutely right!", or similar. Restate the technical requirement, verify
  against the codebase, then act (or push back with reasoning).
- **Actions over words.** "Fixed. [what changed]" or just show the diff.
  Skip gratitude expressions and filler.

### Context Window Discipline

- Keep context small. Load only the module docs relevant to the current task.
- Source code is the ground truth; docs supplement it.
- When a module doc drifts from source, update the doc (or flag it).

<!-- MIGRATION CHECKLIST — apply on contact, then delete completed items.

- [ ] If `ai-docs/_memory.md` exists, merge any useful content into
      `ai-docs/_index.md` and delete `_memory.md`.
- [ ] If `ai-docs/spec/` does not exist, set it up. Load `/write-spec` for
      conventions. Add a spec index section to `_index.md`.
- [ ] If tickets lack `plans:` frontmatter, add it (use `null` for unplanned
      phases). Audit phase content: discussion decisions stay in tickets,
      codebase-derived details belong in plans.
- [ ] If tickets lack `parent:` frontmatter for epic relationships, add it
      where applicable. Epic tickets use category `epic`.
- [ ] If plan paths use old format (`YYMM/DD-HHMM.<name>.md`), rename to
      `YYYY-MM/DD-hhmm.<name>.md` via `git mv`.

Prioritize migrating universal conventions (ai-docs/ structure, workflow
procedures, code standards) from the current template. Project-specific
content (Architecture Rules, custom standards, tailored workflows) must
survive — merge surgically, and flag conflicts rather than overwriting.
-->
