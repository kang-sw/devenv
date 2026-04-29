---
title: Documentation tooling restructure — forge-mental-model, write-mental-model removal, forge skill palette visibility, bootstrap legacy routing
completed: 2026-04-23
spec:
  - 260423-forge-mental-model-skill
  - 260423-bootstrap-legacy-forge-routing
spec-remove:
  - 260421-write-mental-model
---

# Documentation tooling restructure

## Background

The mental-model tooling has three actors that currently overlap in confusing ways:

- `/write-mental-model` skill — "rebuilds or updates" but is checkpoint-agnostic; its full-rebuild path is underspecified for domain discovery.
- `mental-model-updater` agent — checkpoint-based, automated, dispatched by implementation skills.
- No from-scratch skill — equivalent to the gap that `/forge-spec` fills for the spec system.

Additionally, `/forge-spec` (and the planned `/forge-mental-model`) are one-time reconstruction operations that should never appear in the model's routine skill palette.

This ticket restructures to:
1. Mark forge skills as palette-hidden (`disable-model-invocation: true`).
2. Introduce `/forge-mental-model` as the explicit from-scratch construction path.
3. Remove `/write-mental-model` — its use cases are fully covered by `mental-model-updater` (incremental) and `/forge-mental-model` (from-scratch).
4. Add legacy-project detection to `/bootstrap` to surface the forge workflow.

## Decisions

- **write-mental-model is deleted, not kept as ad-hoc tool.** The "non-checkpoint-triggered, survey-driven, single-domain refresh" use case was considered but deemed insufficient to justify a third mental-model tool. Forge-mental-model can target a single domain with the same survey approach.
- **Staleness warning in discuss is warn-only.** No skill suggestion accompanies the 90-day staleness warning — users decide what action to take based on context.
- **forge-mental-model spec-gate is soft (warn, not block).** Mental-model is buildable without spec; stems are embedded opportunistically when spec exists.
- **bootstrap legacy detection is suggest-only.** Bootstrap never auto-invokes forge skills — they require explicit user confirmation at domain-selection step.

## Constraints

- Phase 3 (removal) must follow Phase 2 (forge-mental-model creation) — the removal breaks the toolchain if no replacement exists.
- Phase 4 (bootstrap detection) must follow Phase 2 — bootstrap suggests `/forge-mental-model`, which must exist.
- Phase 1 is independent of all other phases.
- `disable-model-invocation: true` hides skills from the model's palette; users can still invoke explicitly. This flag was missed for `/forge-spec` at creation time.

## Phases

### Phase 1: forge-spec — add disable-model-invocation

Add `disable-model-invocation: true` to `claude/skills/forge-spec/SKILL.md` frontmatter.

This is a one-line correction — the flag was intended at creation but omitted.

### Result (f55a3a7) - 2026-04-23

Added `disable-model-invocation: true` to forge-spec/SKILL.md frontmatter. One-line change, no deviations.

### Phase 2: forge-mental-model — new skill

Create `claude/skills/forge-mental-model/SKILL.md` mirroring the forge-spec pattern:

**Frontmatter:**
- `disable-model-invocation: true`
- `argument-hint: "[target domain or 'all']"`

**Invariants:**
- No domain file is written without completing the survey for that domain first.
- All domain-level steps require explicit user confirmation (domain list approval before any file is written).
- TaskCreate prefix `forge-mental-model-<domain>` for cross-compact resumability — renaming breaks resume.
- Follows mental-model-conventions.md for every document written.

**On: invoke flow:**
1. **judge: spec-gate (soft)** — check if `ai-docs/spec/` exists. If absent, warn: "No spec found — mental-model will be built without spec stem cross-references. Run /forge-spec first for full cross-reference support." Proceed regardless.
2. Survey codebase with parallel Explore subagents (structure, entry points, coupling hotspots).
3. Present domain candidate list to user. Confirmed list locks via TaskCreate.
4. Per domain (sequentially):
   a. Dispatch Explore subagent to survey domain internals.
   b. Draft domain file per mental-model-conventions.md inclusion test.
   c. If spec exists, embed relevant spec stems inline.
   d. Dispatch mental-model-verifier subagent; apply HIGH findings, collect LOW/BLOAT.
   e. Write file; mark TaskCreate task complete.
5. Update `ai-docs/mental-model.md` index.

**Rejected alternative:** Keep write-mental-model as a lightweight single-domain refresh. Rejected because forge-mental-model with a single-domain argument covers the same case with better quality (verifier step, domain confirmation).

### Result (9fad0e9) - 2026-04-23

Created `claude/skills/forge-mental-model/SKILL.md` (288 lines). Mirrors forge-spec structure with three-agent cold-start survey (structure, entry-points, coupling), per-domain Explore survey, verifier step, and spec-stem embedding when spec is available. mental-model-updater created a new `doc-tooling.md` mental-model domain and updated `spec-system.md` after this phase landed.

### Phase 3: write-mental-model removal and discuss/bootstrap cleanup

**Depends on Phase 2.**

Files to change:
- Delete `claude/skills/write-mental-model/SKILL.md`.
- `claude/skills/discuss/SKILL.md`:
  - Invariants (line ~25): remove "say so and suggest `/write-mental-model`" → "say so".
  - Staleness warning (line ~40): remove "consider `/write-mental-model`" suffix from the warning text (warn-only, no suggestion).
  - Done handler (line ~65): remove the mental-model update item entirely.
- `claude/skills/bootstrap/CLAUDE.template.md` v0021: replace `/write-mental-model` invocation with `mental-model-updater` delegation. Use the initial git commit as the base commit when no `mental-model-updated` checkpoint exists.

Commit implementing this phase must include `removed: 260421-write-mental-model` in `## Spec`.

### Result (c9798b3) - 2026-04-23

Deleted write-mental-model/SKILL.md. Removed three discuss/SKILL.md write-mental-model references (Invariants, staleness warning suffix, done-handler item). Updated v0021 in bootstrap/CLAUDE.template.md to delegate to mental-model-updater with initial-commit base fallback. Commit includes `removed: 260421-write-mental-model` signal.

### Phase 4: bootstrap legacy detection

**Depends on Phase 2.**

After `/bootstrap` completes a `fresh` or `adopt` run, check:
- If `ai-docs/spec/` is absent → suggest `/forge-spec`.
- If `ai-docs/mental-model/` is absent → suggest `/forge-mental-model`.
- If both absent → suggest `/forge-spec` first, then `/forge-mental-model`.

The suggestion is output-only — bootstrap never invokes forge skills directly.

Locate the correct file for this change: either `claude/skills/bootstrap/SKILL.md` or `CLAUDE.template.md` depending on where the fresh/adopt completion step is defined.

### Result (82b6b88) - 2026-04-23

Added legacy detection to bootstrap/SKILL.md On: fresh (step 7) and On: adopt (step 6). Detection checks for absent `ai-docs/spec/` and `ai-docs/mental-model/` and surfaces appropriate /forge-spec and/or /forge-mental-model suggestions. Output-only — no auto-invocation. Also added v0026-v0027 migration items to CLAUDE.template.md for upgrade-path detection (stem presence check via grep).
