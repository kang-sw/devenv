---
name: bootstrap
description: >
  Bootstrap a new project or upgrade an existing one to match the
  canonical CLAUDE.md template. Handles fresh scaffolding, version
  upgrades, and unversioned adoptions.
disable-model-invocation: true
argument-hint: "[fresh | upgrade — optional, auto-detected if omitted]"
---

# Bootstrap

Mode: $ARGUMENTS

## Invariants

- Template source is `${CLAUDE_SKILL_DIR}/CLAUDE.template.md` — read it before any action.
- Never overwrite project-specific sections: Architecture Rules, custom Code Standards entries, custom Project Knowledge entries.
- Merge surgically. When template and project conflict, flag the conflict inline with `<!-- CONFLICT: ... -->` and move on — do not resolve silently.
- Every migration item is idempotent — re-running on an already-migrated project produces no changes.
- Commit each logical unit (scaffolding, migration batch, cleanup) separately, following CLAUDE.md commit rules.
- Delegate aggressively — only handle strategic judgment directly; mechanical work goes to subagents (see `judge: delegation`).

## On: invocation

1. Read `${CLAUDE_SKILL_DIR}/CLAUDE.template.md`.
2. Read the current project's `CLAUDE.md` (if it exists).
3. Detect mode:
   - **fresh** — no `CLAUDE.md` exists. Scaffold from template.
   - **upgrade** — `CLAUDE.md` exists with `<!-- Template Version: vNNNN -->`. Apply items where version > current.
   - **adopt** — `CLAUDE.md` exists without version tag. Follow v0031 rules: audit v0001–latest against project state, apply what's missing.
4. Execute the appropriate handler below.

## On: fresh

1. Copy template to `CLAUDE.md`, stripping the `<!-- MIGRATION: ... -->` setup block.
2. Leave placeholder markers in project-specific sections (`[PROJECT_NAME]`, `[Rule name]`, `[Description]`).
3. Create `ai-docs/` structure per the MIGRATION block in the template:
   - `ai-docs/_index.md` (with memory-policy comment, skeleton headings)
   - `ai-docs/_index.local.md` (stub, add to `.gitignore`)
   - `ai-docs/tickets/` with status subdirectories: `idea/`, `todo/`, `.done/`, `.dropped/`
   - `ai-docs/mental-model/`, `ai-docs/ref/`
4. Add `ai-docs/_index.local.md` to `.gitignore` if not present.
5. Set `<!-- Template Version: vNNNN -->` to latest version from template.
6. Commit.
7. **Legacy detection**: check for absent documentation baselines and suggest forge skills as needed (output-only — do not invoke).
   - `ai-docs/spec/` absent → suggest `/forge-spec` to build the spec baseline.
   - `ai-docs/mental-model/` absent → suggest `/forge-mental-model` to build the mental-model baseline.
   - Both absent → suggest `/forge-spec` first, then `/forge-mental-model`.

## On: upgrade

1. Parse current version from `<!-- Template Version: vNNNN -->`.
2. Walk migration checklist items where version > current, in order.
3. For each item:
   - If marked `[obsoleted by vNNNN]`, skip.
   - Check the condition (e.g., "If X lacks Y").
   - If condition met, apply the change. If not, skip.
4. After all items applied, update `<!-- Template Version: vNNNN -->` to latest.
5. Sync non-project-specific template sections (Response Discipline, Workflow, Commit Rules) with latest template wording — use diff judgment.
6. Commit.

## On: adopt

1. Follow v0031: review v0001 through latest against current project state.
2. For each item, check whether already satisfied; apply only what's missing.
3. Add `<!-- Template Version: vNNNN -->` at bottom of CLAUDE.md set to latest.
4. Proceed to **upgrade** handler for any remaining sync.
5. Commit.
6. **Legacy detection**: same as On: fresh step 7 — check for absent `ai-docs/spec/` and `ai-docs/mental-model/`; suggest forge skills as needed (output-only).

## Judgments

### judge: delegation

Delegate aggressively — only handle strategic judgment directly.
- *haiku* — directory creation, file scaffolding, .gitignore edits, formulaic checklist items (deterministic condition → deterministic apply).
- *sonnet* — migration items needing context reads, multi-file coordination, section sync with diff.
- *direct* — mode detection, section-merge judgment, conflict resolution, adopt-mode audit.

### judge: section-merge

When syncing a template section (e.g., Response Discipline) with the project's version:
- If project version is identical or trivially reformatted → replace with template.
- If project version has meaningful additions → merge template updates around them, preserve additions.
- If unsure → flag with `<!-- CONFLICT: ... -->`.

### judge: migration-condition

When a migration item's condition is ambiguous (e.g., "If tickets lack X" but no tickets exist):
- No tickets/files to migrate → condition not met → skip.
- Partial match → apply to matching subset only.

## Doctrine

Bootstrap optimizes for **idempotent correctness**: every run
produces the same result regardless of how many times it executes, and
project-specific content is never lost. When a rule is ambiguous, apply
whichever interpretation more reliably preserves both idempotency and
project content.
