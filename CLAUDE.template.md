# CLAUDE.md

## Documentation Layout

- All AI-readable documentation lives under `ai-docs/`. Do NOT store docs elsewhere.
- All `ai-docs/` documents and AI-facing plans must be written in **English**.

### Mental Model

- `ai-docs/mental-model.md` is the **project brief**: high-level goals, architecture overview, recent work log, and short/mid-term context.
  - **Actively update this file** — it is the highest-priority documentation task. Every session should begin and end by reviewing it.
  - Add recent work history, current status, and near-term plans.
  - **Prune aggressively**: remove or condense entries that are no longer relevant. Old completed tasks, resolved issues, and obsolete context should be deleted or collapsed into a one-line summary. The file must stay concise and current.
- `ai-docs/mental-model/` is a directory for **deep technical details**:
  - Per-module or per-subsystem design documents
  - Technical implementation specs and constraints
  - Decision logs: what was decided, what was tried and reverted, and why
  - Workarounds and non-obvious technical choices that future sessions must remember
  - Link to these documents from the root `mental-model.md` where relevant.

## Workflow Rules

- Before implementing or designing anything, identify every architectural ambiguity that could affect future direction and **ask the user** rather than guessing.
  - Exception: decisions that are trivially reversible even after code accumulates may be made autonomously with a note in the response.
- Read existing code before proposing or making changes.
- Keep solutions minimal — no extra features, abstractions, or refactors beyond what is explicitly requested.
- **Update `ai-docs/mental-model.md` at every meaningful checkpoint** — not just at the end. Treat it as a living working-memory document.

## Commit Rules

- Create a commit after every unit of work, including documentation changes.
- Every commit message must include an `## AI COMMENT` section at the end. This section should contain:
  - Summary of design decisions made during this work
  - Key discussion points and rationale
  - Any context needed to understand the commit's purpose
  - Information that helps a future AI reconstruct the working context from commit history

## Project Files for Complex Features

- When implementing a complex feature, create a project file at `ai-docs/projects/YYMMDD-HHMM-<project-name>.md`.
- Each project file should contain:
  - Implementation spec and design goals
  - Summary of discussions and decisions leading to this work
  - Session-by-session milestone breakdown with detailed goal items per session
  - Technical implementation ideas and notes (when sufficiently clear)
- **Plan mode tagging**: For each milestone/phase, assess its complexity.
  - If a phase requires careful architectural thought, non-trivial design, or coordination across multiple modules, append `(plan mode)` to the phase title. This signals that you must enter plan mode and produce a detailed plan before writing any code for that phase.
  - If a phase is straightforward (small scope, clear implementation path), omit the tag and proceed directly.
  - Example:
    ```
    ### Phase 1: Database schema migration (plan mode)
    ### Phase 2: Add unit tests for validators
    ### Phase 3: Refactor event pipeline (plan mode)
    ```
- Use these files to track progress across sessions and maintain continuity.

## Testing Rules

- Write test cases aggressively for all logic where feasible.
- When a test fails, diagnose whether the **assumption in the test** is wrong or the **logic under test** is buggy. Fix the correct side — do not blindly modify the test to make it pass.
- If testing requires user interaction (e.g., UI, manual CLI invocation), notify the user and request manual testing rather than skipping or faking it.

## Project-Specific Notes

*(Populated as the project evolves — see `ai-docs/mental-model.md` for current state.)*
