<!-- AI-maintained project state — read before work, update after -->
<!-- - `ai-docs/_index.md` — architecture overview, conventions -->
<!-- - `ai-docs/_memory.md` — recent work, workspace reference -->

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
Read `_index.md` and `_memory.md` at session start.
Before creating or editing tickets, load `/write-ticket` for conventions.
Reference tickets by **stem only** (e.g., `260115-feat-foo-bar`), never by
full path — stems stay stable across status moves.
When starting work on a ticket, move it to `wip/` immediately.

**Language:** All AI-authored artifacts — documents, plans, commit messages, ticket entries,
`### Result` entries, and inline code comments — must be in English regardless of
conversation language. Human-facing UI strings are exempt.

<!-- MIGRATION: Set up ai-docs/ for this project, then delete this block.

ai-docs/
  _index.md          — project architecture and stable conventions
  _memory.md         — cross-session continuity, updated each session
  mental-model/      — architecture docs, regenerable from source
  deps/              — external library API delta docs
  ref/               — static reference material (external specs, protocol docs, design notes)
  tickets/<status>/  — idea/ todo/ wip/ done/ dropped/

_index.md should cover:
  - Project state overview (current milestone, active initiatives)
  - Module/directory map and relationships
  - Conventions (tickets, dependency docs, naming rules)
  - Build/test commands and operational pitfalls

_memory.md should cover:
  - Recent work context (what was done, what's pending)
  - Workspace quick-reference (key paths, package names)
  - Ephemeral memos (temporary notes that may expire)

Adapt structure to fit the project — these are guidelines, not a rigid schema.
-->

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

- Read `ai-docs/_index.md` and `ai-docs/_memory.md` for project context.
- Run `git log --oneline -10` for recent changes. Read full messages
  (`git show`) when a commit is relevant — they carry decision context.

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
