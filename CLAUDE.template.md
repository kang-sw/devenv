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
This is the primary cross-session context document.

**When to read:** Load `_index.md` at session start. Load relevant module docs before tasks.
**When to update:** After implementing changes that affect operational state or a module's
  public API. Update the specific section/doc, not everything.

**Tickets** (`notes/ai-docs/tickets/YYMMDD-MMDD-<name>.md`) track substantial features.
In-progress tickets use a `[wip]` suffix: `YYMMDD-MMDD-<name>[wip].md`.
Remove the `[wip]` marker when the ticket is complete.
Phases that require non-trivial design before coding are marked **(plan mode)** — always
plan and get approval before implementing those phases.

**MEMORY.md** (`~/.claude/projects/.../memory/MEMORY.md`) persists across sessions.
Structure by priority (top = highest, survives 200-line truncation):
1. Build/workflow memos — things that affect every session
2. Recent work context (max 3 items) — what was done, what's next
3. User preferences — communication style, workflow habits
4. Workspace reference — crate/package names, architecture quick ref

## Coding Guidelines

1. **Think first.** State assumptions. Verify before guessing. Define success criteria
   before starting.
2. **Simplicity.** Write the simplest code that works. Implement fully when the spec is
   clear — judge scope by AI effort, not human-hours.
3. **Surgical changes.** Change only what the task requires. Follow existing style. Every
   changed line must trace to the request.
4. **Test proactively.** Write unit tests for non-trivial pure logic (math, protocol, ECS,
   state machines) as you code. Run the test suite before moving on.
   When tests fail, first diagnose whether the **test assumptions** or the **implementation
   logic** is wrong — don't blindly fix the implementation to match a bad test.
   For user-interactive features (UI, visual output), request manual testing instead.
5. **Module structure.** Split files at ~300 lines into `<module>/mod.rs` (or equivalent)
   + submodules. The module entry point = doc comment + pub re-exports only. Reading it
   alone conveys the module's interface.
6. **Hot-path allocation.** In performance-sensitive paths, prefer pre-allocated buffers
   and stack-friendly types. Apply when benefit > complexity cost.

## Workflow — AI-Driven Implementation

### Approval Protocol
- **Auto-proceed**: Bug fixes, pattern-following additions, test code, boilerplate,
  refactoring within a single module.
- **Ask first**: New component/protocol additions, architectural changes,
  cross-module interface changes, anything that changes observable behavior.
- **Always ask**: Deleting existing functionality, changing protocol/API semantics,
  modifying persistence schema.

### Implementation Process
1. **Plan first.** Before writing code, output the implementation plan (affected files,
   approach, success criteria). For trivial changes, a one-liner suffices.
   **Plans must be written in English** regardless of conversation language.
2. **Verify.** Run the project's test command (e.g. `cargo test`, `pytest`, `npm test`).
   Also run any integration test harness available. Must pass before committing.
3. **Verify assets.** After editing non-code assets (scenes, configs, schemas), run the
   appropriate validator or importer in headless/dry-run mode to check for parse errors.
4. **Build.** Run a full build so all artifacts are up to date.
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
6. **Update docs.** After non-trivial tasks:
   - Update `_index.md` Operational State if project capabilities changed.
   - Update `MEMORY.md` recent work context (what was done, what's next).
   - Prune aggressively: keep both documents focused on current state.

### Session Start
- Read `notes/ai-docs/_index.md` to understand project state and architecture.
- Run `git log --oneline -10` to catch up on recent work.

### Context Window Discipline
- Keep context small. Load only the module docs relevant to the current task.
- Source code is the ground truth; docs supplement it.
- When a module doc drifts from source, update the doc (or flag it).
