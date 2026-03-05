# CLAUDE.md

## Documentation Layout

- All AI-readable documentation lives under `ai-docs/`. Do NOT store docs elsewhere.
- `ai-docs/mental-model.md` is the single source of truth for project insights: goals, architecture blueprint, recent progress, and design decisions.
- All `ai-docs/` documents and AI-facing plans must be written in **English**.

## Workflow Rules

- Before implementing or designing anything, identify every architectural ambiguity that could affect future direction and **ask the user** rather than guessing.
  - Exception: decisions that are trivially reversible even after code accumulates may be made autonomously with a note in the response.
- Read existing code before proposing or making changes.
- Keep solutions minimal — no extra features, abstractions, or refactors beyond what is explicitly requested.
- Update `ai-docs/mental-model.md` when significant design decisions are made or the project state changes.

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
  - Session-by-session task breakdown with detailed goal items per session
  - Technical implementation ideas and notes (when sufficiently clear)
- Use these files to track progress across sessions and maintain continuity.

## Testing Rules

- Write test cases aggressively for all logic where feasible.
- When a test fails, diagnose whether the **assumption in the test** is wrong or the **logic under test** is buggy. Fix the correct side — do not blindly modify the test to make it pass.
- If testing requires user interaction (e.g., UI, manual CLI invocation), notify the user and request manual testing rather than skipping or faking it.

## Project-Specific Notes

*(Populated as the project evolves — see `ai-docs/mental-model.md` for current state.)*
