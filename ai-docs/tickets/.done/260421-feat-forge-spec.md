---
title: "New Skill: /forge-spec — From-Scratch Spec Reconstruction"
related:
  260421-feat-rebuild-spec-skill: superseded-concept
  260420-feat-spec-driven-workflow: prerequisite
plans:
  phase-1: 2026-04/260421-1500.forge-spec-p1-3.survey
  phase-2: 2026-04/260421-1500.forge-spec-p1-3.survey
  phase-3: 2026-04/260421-1500.forge-spec-p1-3.survey
started: 2026-04-21
completed: 2026-04-21
---

# New Skill: /forge-spec — From-Scratch Spec Reconstruction

## Background

Downstream projects may have no spec, or specs that predate the `{#YYMMDD-slug}` anchor convention and have drifted from actual codebase behavior. `/forge-spec` reconstructs the spec from scratch by combining preemptive Sonnet subagent surveys of code, tickets, and existing documents with a guided user discussion loop.

The skill is discussion-weighted: the AI does the investigative legwork so the user is never asked "how does this component work?" — only "does this belong in spec, and is it implemented or planned?" Aggressive questioning on ambiguity is the operating principle, not a fallback.

## Decisions

**Archive-first, clean-slate reconstruction**
Existing spec files move to `ai-docs/ref/old-spec/YYMMDD/` (today's date) before any new spec is written. The old spec serves as reference only — not a base to extend. This eliminates drift contamination from prior decisions and forces fresh categorization of every behavior against `spec-conventions.md`.

**Sonnet override for all explore subagents**
Sonnet (not Haiku) for every codebase, ticket, and commit survey subagent. Smart behavioral summarization is required — not raw code scraping — and hallucination risk is unacceptable when output drives spec content decisions.

**TaskCreate-based domain checklist for cross-compact persistence**
Domain identification happens once at cold start: subagents propose domains, user confirms and orders them, then `TaskCreate` locks the checklist. On resume after compact, the skill reads the task list and continues from the next incomplete domain. This makes the skill fully resumable without context overhead.

**Aggressive questioning on ambiguity**
When it is unclear whether a behavior is caller-visible vs internal, or implemented vs planned, the skill asks — never guesses. Internal behaviors are excluded from spec per `spec-conventions.md`; ambiguous cases surface to the user.

**Spec placement follows `spec-conventions.md` doctrine**
External, caller-perspective behaviors go in spec. Internal implementation details do not. `spec-conventions.md` is the arbiter. Flat leaf file (`spec/<area>.md`) for single-surface areas; directory (`spec/<area>/`) for areas with multiple sub-sections.

## Constraints

- The archive step (`git mv`) is destructive to the spec tree. Confirm with user before executing — do not proceed silently.
- No spec entry is written without explicit user confirmation of caller-visible status and implemented/planned classification.
- Sonnet model override must be applied explicitly to every subagent dispatch — not inherited from parent.
- Spec content must follow `spec-conventions.md`: behavioral, external-perspective, no implementation details.

## Prior Art

- `generate-spec-stem` — collision-free stem generation before every anchor insertion.
- `spec-build-index` — frontmatter regeneration after every spec file write.
- `list-stems` — stem discovery across spec directory.
- `spec-updater` — strips 🚧 markers after implementation is confirmed via commit history.
- `spec-conventions.md` — canonical arbiter of what belongs in spec vs stays internal.
- `TaskCreate` / `TaskList` / `TaskUpdate` — cross-compact state persistence for domain checklist.

## Phases

### Phase 1: Skill file + cold start flow

Create `claude/skills/forge-spec/SKILL.md`.

**Resume detection (runs first on every invocation):**
Check `TaskList` for existing domain tasks scoped to this project. If found, skip to Phase 2 and continue from the next incomplete domain.

**Cold start sequence (no existing task list):**
1. Confirm with user that archive is about to proceed.
2. Archive existing spec: `git mv ai-docs/spec/* ai-docs/ref/old-spec/YYMMDD/`. If `ai-docs/spec/` is empty or absent, skip.
3. Dispatch Sonnet subagents in parallel to survey:
   - Directory + module structure (source tree layout)
   - All tickets under `ai-docs/tickets/` (all statuses including `done/` and `dropped/`)
   - Archived spec files (reference only)
   - Recent commit history (`git log --oneline -100`)
4. Synthesize domain candidate list from survey results.
5. Present candidate domains to user — allow reorder, merge, split, or drop before proceeding.
6. On user confirmation: `TaskCreate` one task per domain in confirmed order.
7. Proceed to Phase 2 with domain 1.

**Acceptance criteria:**
- Cold start: archive confirmed + executed before any spec is written.
- Domain list is user-confirmed before task list is locked.
- Resume: task list read at invocation start; completed domains skipped automatically.

### Result (fee9bbc) - 2026-04-21

Implemented as specified. All acceptance criteria met. Key deviations from plan: none.
Review found 8 issues across 2 rounds (wording, judge criteria alignment, post-write split check, conventions). All resolved before merge.
Mental-model updated: forge-spec entry added to spec-system domain with confirmation gate contract, resume-detection contract, and rebuild-from-scratch recipe.

### Phase 2: Per-domain spec authoring loop

For each domain (in task order, skipping completed tasks):

1. Dispatch Sonnet subagents in parallel to survey current behavior for this domain:
   - Source code under domain's module path(s).
   - Relevant tickets (filter by domain keywords or path patterns).
   - Archived old spec sections (reference only).
   - Commit log filtered by domain path (`git log --oneline -- <path>`).
2. Synthesize into a "behavior brief": bullet list of caller-visible behaviors, distinguishing implemented (present in code) vs planned (ticket in `wip/` or `todo/`).
3. Present behavior brief to user. For each behavior, establish:
   - Caller-visible (goes in spec) or internal-only (excluded)?
   - Implemented (`{#slug}`) or planned (`🚧 {#slug}`)?
4. Ask on every ambiguous item — never categorize without confirmation.
5. Write spec entries:
   - Run `generate-spec-stem <descriptive-slug>` for each new anchor.
   - Insert `{#YYMMDD-slug}` at appropriate point (heading or body line).
   - Add `🚧` prefix if planned; omit if implemented.
   - Run `spec-build-index <file>` after writing or updating each spec file.
6. `TaskUpdate` to mark domain done.
7. Continue to next incomplete domain.

**Acceptance criteria:**
- No spec entry written without user confirmation of caller-visible status and implemented/planned classification.
- `generate-spec-stem` called before every anchor insertion.
- `spec-build-index` run after every spec file modification.
- Domain task marked done after user confirms spec entries are complete.

### Result (fee9bbc) - 2026-04-21

Implemented as specified. Per-domain loop includes 4-way parallel survey subagents, behavior brief synthesis, user classification loop, spec-entry authoring with generate-spec-stem + spec-build-index, and post-write split-check (flags candidates without interrupting the loop).

### Phase 3: Wrap-up

After all domain tasks are complete:

1. Run `spec-build-index` on all created spec files (idempotent safety pass).
2. Emit summary report:
   - Domains covered and spec files created.
   - Total stems generated.
   - Count of 🚧 (planned) vs implemented entries.
3. Suggest next steps:
   - Run `spec-updater` to verify any 🚧 markers against commit history.
   - Move completed domain tasks to done or delete task list.

**Acceptance criteria:**
- All domain tasks marked done before wrap-up runs.
- Summary includes stem count and 🚧 count.
- `spec-updater` suggestion included in output.

### Result (fee9bbc) - 2026-04-21

Implemented as specified. Wrap-up runs final spec-build-index safety pass, emits domain/stem/🚧 summary, and suggests invoking the spec-updater agent (correctly identified as claude/agents/spec-updater.md, not a bin command).
