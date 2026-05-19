---
title: Contextual mental-model update via commit annotations and brief detection
spec:
  - 260518-mental-model-update-context-annotation
related-mental-model:
  - workflow-skills
  - prompt-bundle
---

# Contextual mental-model update via commit annotations and brief detection

## Background

`mental-model-updater` currently receives only a commit range and reconstructs
intent purely from code diffs and spec changes. This means non-obvious
invariants, ordering constraints, lifecycle assumptions, and cross-module
contracts — the kind of implicit knowledge that mental models exist to
capture — are regularly missed or inaccurately inferred.

The updater runs after `lead-update-spec` in `lead-implement`'s Doc Pre-Pass.
Its invocation is a single MCP call with no brief, no plan, and no
session-context hint from the implementer.

## Decisions

- Use `### Mental Model Notes` as an H3 sub-section under `## AI Context` in
  commit bodies. H3 gives visual attention without escaping the AI Context
  block and without adding a project-wide root convention in AGENTS.md.
- This is a workflow-internal ("local dialect") convention: defined in
  `impl-playbook`, consumed by `mental-model-updater`, invisible to downstream
  projects.
- Implementer obligation: record `### Mental Model Notes` when the
  implementation creates a non-obvious invariant, ordering constraint,
  lifecycle assumption, or cross-module contract not directly visible in code.
- Updater change: read `### Mental Model Notes` entries from the commit range
  before processing diffs; treat them as primary intent context, diffs as
  secondary verification.

## Constraints

- Do not add `### Mental Model Notes` to AGENTS.md. It is a runtime-bundled
  workflow convention only.
- Do not change `lead-implement`'s Doc Pre-Pass call signature for this phase.
- Keep `### Mental Model Notes` optional; absence means no implicit contracts
  were created, not a violation.

## Deferred

- **Brief self-detection:** when a `.brief.md` path appears in the diff stat
  within the commit range, the updater reads it as additional intent context.
  This handles the delegated path without any implementer annotation and is a
  pure updater-side enhancement. Deferred because it requires understanding
  how brief commits land relative to `<implementation-start>` in
  `lead-implement`.
- **direct-edit path enrichment:** `lead-edit` produces no brief. For
  direct-edit work, `### Mental Model Notes` in the lead's commit is the only
  context injection available. This does not require a separate ticket phase
  but should be verified during Phase 1 testing.

## Phases

### Phase 1: Commit annotation convention and updater priority processing

Add the `### Mental Model Notes` convention to `impl-playbook` and update
`mental-model-updater` to consume it.

Requirements:

- In `impl-playbook`, add an obligation rule: when implementation creates a
  non-obvious invariant, ordering constraint, lifecycle assumption, or
  cross-module contract not visible from the code itself, capture it in the
  commit body under `### Mental Model Notes` inside `## AI Context`.
- The rule must be falsifiable (concrete violation: implementer omits
  annotation for a new lifecycle invariant), one-line actionable, and
  positioned near existing commit-related guidance.
- In `mental-model-updater`, update Process step 1 to extract
  `### Mental Model Notes` entries from commit bodies via
  `ws/git.log(include_body: true)` before reading diffs.
- Treat extracted notes as primary intent signals; use diffs to verify and
  fill gaps.
- Notes absence: fall back to current diff-only process without error.
- Verify that `ws/git.log` with `include_body: true` surfaces commit body
  content correctly for the updater's use.

Verification:
- `go test ./...` from `agents-plugin-tool/`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- Manual trace: a commit with `### Mental Model Notes` in body is prioritized
  over raw diff inference in updater output.

### Result (cab3ef2f) - 2026-05-19

Phase 1 added the implementation-playbook obligation to record non-obvious
mental-model context under `## AI Context` -> `### Mental Model Notes`, updated
`mental-model-updater` to read commit bodies with `ws/git.log(include_body:
true)` before diff analysis, and refreshed both ws and wsflow prompt bundle
hashes.

Verification passed:

- `go test ./...` from `agents-plugin-tool/`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- Manual trace: `ws/git.log(include_body: true)` over `cab3ef2f` surfaced the
  `### Mental Model Notes` subsection for updater consumption.

Dogfooding found that `ws/git.commit` cannot yet emit H3 subsections under
`## AI Context`; follow-up ticket `260519-bug-git-commit-mental-model-notes`
captures that gap. Phase 2 remains deferred.

### Phase 2: Updater brief self-detection

Enhance `mental-model-updater` to detect and read brief files from the diff
stat when available, providing rich intent context for the delegated path
without any changes to `lead-implement` or `lead-write-code`.

Requirements:

- In Process step 1, after reading `ws/git.log`, inspect
  `ws/git.diff(mode: "stat")` for paths matching `ai-docs/.plans/*.brief.md`.
- If a brief path is present, read it before processing domain diffs; use its
  `## Contract Instructions`, `## Implementation Strategy Decisions`, and
  `## Rejected Alternatives` as intent context.
- Brief presence does not suppress `### Mental Model Notes` processing;
  both sources are additive.
- Confirm whether brief commits fall within the `<implementation-start>..HEAD`
  range produced by `lead-implement` before landing this phase.

Verification:
- `go test ./...` from `agents-plugin-tool/`
- Manual trace: updater reads brief and targets only relevant domains rather
  than scanning all mental-model docs.
