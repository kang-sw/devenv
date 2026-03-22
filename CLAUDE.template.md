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

Project state, architecture, and source layout live in **`ai-docs/_index.md`**.
All files under `ai-docs/` are AI-maintained and serve as the primary
cross-session context store.

```
ai-docs/
  _index.md          — project state overview (load at session start)
  mental-model/      — architecture docs, regenerable from source
  deps/              — external library API delta docs
  ref/               — static reference material (external specs, protocol docs, design notes)
  tickets/<status>/  — idea/ todo/ wip/ done/ dropped/
```

**When to read:** Load `_index.md` at session start. Load relevant module docs before tasks.
**When to update:** After implementing changes that affect operational state or a module's
public API. Update the specific section/doc, not everything.

**Language:** All AI-authored artifacts — documents, plans, commit messages, ticket entries,
`### Result` entries, `MEMORY` sections, and inline code comments — must be in
English regardless of conversation language. Human-facing UI strings are exempt.

**Tickets** (`ai-docs/tickets/<status>/YYMMDD-<category>-<name>.md`) track substantial features.
`YYMMDD` is the **creation date**; it never changes when the ticket moves between statuses.
Categories: `bug`, `feat`, `refactor`, `chore`, `research`.

- Frontmatter requires `title`. Add `started: YYYY-MM-DD` on move to
  `wip/`; add `completed: YYYY-MM-DD` on move to `done/`.
- **Status is directory-based only:** `idea/` → `todo/` → `wip/` → `done/` (or `dropped/`).
  The containing directory is the single source of truth for status — do not duplicate
  it in frontmatter or elsewhere.
- **Reference tickets by stem only** (e.g., `260115-feat-foo-bar`), never by full
  path including the status directory. This keeps references stable across moves.
- **Move tickets immediately** when status changes — `git mv` to the new directory
  in the same commit. Since references use stems, no cross-link updates are needed.
- After completing a ticket phase, append a `### Result (<short-hash>) - YY-MM-DD` subsection
  recording what was implemented, deviations from the plan, and key findings for future phases.

**MEMORY.md** (`~/.claude/projects/.../memory/MEMORY.md`) persists across sessions
and stores user-specific preferences only (communication style, workflow habits).
Project-specific memory (build memos, recent context, workspace ref) belongs in the
`# MEMORY` section at the bottom of this file, keeping it git-tracked with the project.

## Code Standards

<!-- Principles governing code quality and style. -->

1. **Simplicity.** Write the simplest code that works. Implement fully when the spec is
   clear — judge scope by AI effort, not human-hours.
2. **Surgical changes.** Change only what the task requires. Follow existing style. Every
   changed line must trace to the request.
3. **Module structure.** Split files at ~300 lines. Extract an entry file
   (e.g. `mod.rs`, `index.ts`, `__init__.py`) containing doc comments and public
   re-exports only — reading it alone conveys the module's interface.
4. **Hot-path performance.** In performance-critical paths, prefer minimal allocation
   and data locality over convenience abstractions. Apply only when benefit clearly
   outweighs complexity cost.

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

- Read `ai-docs/_index.md` for project state and architecture.
- Run `git log --oneline -10` for recent changes.

### Dependency API Notes

- **`ai-docs/deps/<package>[v<ver>].md`** stores verified API facts for libraries
  whose actual API differs from training knowledge or is too recent to be known.
- **When to read:** Before writing code that uses a package listed in
  `# MEMORY → Documented Dependencies`. On compile/type errors resembling wrong
  signatures, missing types, or changed fields, consult `ai-docs/deps/` **before**
  exploring package source from scratch.
- **When to write/update:** After discovering API drift (wrong arg count, renamed types,
  removed methods) or learning a previously unknown package's API. Document the verified
  correct API so future sessions skip re-exploration.

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

---

# MEMORY

<!-- AI-maintained. Update after each non-trivial session. Prune aggressively. -->

## Build & Workflow

<!-- Commands, flags, known pitfalls that affect every session. -->

- Build: `[command]`
- Test: `[command]`
- Integration test: `[command]` <!-- prerequisites (e.g. sandbox disabled, server running), when to run -->

## Recent Work

<!-- Max 3 items. What was done, what's next. -->

-

## Workspace Reference

<!-- Key package/crate names, important paths, architecture quick-ref. -->

-

## Documented Dependencies

<!-- Packages with verified API docs in ai-docs/deps/. Read before using. -->

-
