# CLAUDE.md — [PROJECT_NAME]

## Project Summary

<!-- One paragraph: what the project is, who it's for, current milestone/target. -->

**[PROJECT_NAME]** — [brief description]. [Solo dev / team size].
Target: **[YYYY-MM milestone description].**

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

Project state, architecture, and source layout live in **`notes/ai-docs/_index.md`**.
All files under `notes/ai-docs/` are AI-maintained; this is the primary cross-session
context store.

**When to read:** Load `_index.md` at session start. Load relevant module docs before tasks.
**When to update:** After implementing changes that affect operational state or a module's
  public API. Update the specific section/doc, not everything.

**Language:** All AI-authored artifacts — documents, plans, commit messages, ticket entries,
  `### Result` entries, `MEMORY` sections, and inline code comments — must be in English,
  regardless of conversation language. Human-facing UI strings are exempt.

**Tickets** (`notes/ai-docs/tickets/YYMMDD-<name>.md`) track substantial features.
In-progress tickets use a `[wip]` suffix: `YYMMDD-<name>[wip].md`.
Remove the `[wip]` marker when the ticket is complete.
Phases that require non-trivial design before coding are marked **(plan mode)** — use the
`EnterPlanMode` tool, explore + design, get user approval, then `ExitPlanMode` to implement.
After completing a ticket phase, append a `### Result (<short-hash>)` subsection recording:
what was implemented, deviations from the plan, and key findings for future phases.

**MEMORY.md** (`~/.claude/projects/.../memory/MEMORY.md`) persists across sessions.
Stores user-specific preferences only (communication style, workflow habits).
Project-specific memory (build memos, recent context, workspace ref) lives in the
`# MEMORY` section at the bottom of this file so it's git-tracked with the project.

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

## Workflow — AI-Driven Implementation

### Approval Protocol
- **Auto-proceed**: Bug fixes, pattern-following additions, test code, boilerplate,
  refactoring within a single module.
- **Ask first**: New component/protocol additions, architectural changes,
  cross-module interface changes, anything that changes observable behavior.
- **Always ask**: Deleting existing functionality, changing protocol/API semantics,
  modifying persistence schema.

### Implementation Process
1. **Clarify & plan.** For non-trivial changes, state assumptions and define success
   criteria before starting. Break the work into brief steps via `TaskCreate` —
   implementation, docs, and commit. Check them off as you progress.
2. **Implement & test.** Write and run unit tests alongside non-trivial pure logic (math,
   protocol, ECS, state machines). When tests fail, first diagnose whether the **test
   assumptions** or the **implementation logic** is wrong — don't blindly fix the
   implementation to match a bad test.
   For user-interactive features (UI, visual output), request manual testing instead.
3. **Verify.** Run the full test suite and integration test harness. If the project
   has a build or asset-import step, run it after editing relevant files to catch
   errors early. All checks must pass before committing.
4. **Update docs.** After non-trivial tasks:
   - Update `_index.md` Operational State if project capabilities changed.
   - Update `# MEMORY` section in this file (what was done, what's next).
   - If completing a ticket phase, append `### Result` to that phase in the ticket doc.
   - Prune aggressively: keep both documents focused on current state.
5. **Commit.** Auto-create git commits broken down by logical units.
   Commit messages must include an **AI context** section after the change summary.
   Record design decisions, alternatives considered, trade-offs — focus on *why*
   this approach was chosen. Format:
   ```
   <type>(<scope>): <summary>

   <what changed — brief>

   ## AI Context
   - <decision rationale, rejected alternatives, user directives, etc.>
   ```

### Session Start
- Read `notes/ai-docs/_index.md` to understand project state and architecture.
- Run `git log --oneline -10` to catch up on recent work.

### Dependency API Notes
- **`notes/ai-docs/deps/<package>[v<ver>].md`** stores verified API facts for libraries
  whose actual API differs from training knowledge or is too new to be known.
- **When to read:** Before writing code that uses a package listed in
  `# MEMORY → Documented Dependencies`. Also check on compile/type errors that look like
  wrong signatures, missing types, or changed fields — consult `ai-docs/deps/` **before**
  exploring package source from scratch.
- **When to write/update:** After discovering API drift (wrong arg count, renamed types,
  removed methods, etc.) or after learning a previously-unknown package's API, document
  the verified correct API so future sessions skip re-exploration.

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
- Integration test: `[command]`

## Recent Work

<!-- Max 3 items. What was done, what's next. -->

-

## Workspace Reference

<!-- Key package/crate names, important paths, architecture quick-ref. -->

-

## Documented Dependencies

<!-- Packages with verified API docs in notes/ai-docs/deps/. Read before using. -->

-
