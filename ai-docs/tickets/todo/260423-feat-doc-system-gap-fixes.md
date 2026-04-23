---
title: Documentation System Gap Fixes (A/B/C/D)
spec:
  - 260423-spec-removal-commit-convention
  - 260423-spec-updater-pending-removal
  - 260423-discuss-mental-model-staleness-warning
---

# Documentation System Gap Fixes (A/B/C/D)

## Background

Discussion session identified four gaps where the spec and mental-model documentation systems lacked coordination. The two convention documents are well-designed individually but under-specify: (A) how to handle feature removal from specs, (B) how mental-model-updater learns from spec changes, (C) how discuss sessions surface stale mental-model docs, and (D) how mental-model domains cross-reference spec entries.

## Decisions

- **Gap A — Removal via commit syntax**: `removed: <spec-stem>` in a commit's `## Spec` section signals feature removal. spec-updater detects this and adds the entry to `### Pending removal` in its report — never auto-deletes. Human confirms and deletes manually. Rationale: spec-updater's conservative-correctness doctrine makes auto-deletion riskier than auto-stripping 🚧.
- **Gap B — Spec diff as signal**: mental-model-updater adds spec file changes to its domain assessment input, in addition to code diffs. When spec-updater strips 🚧 from a feature, mental-model-updater should verify the corresponding domain reflects the implemented contracts. Spec changes don't replace code diff assessment — they add to it.
- **Gap C — Staleness via git log, not frontmatter**: `git log -1 --format="%ai" -- ai-docs/mental-model/<domain>.md` gives the last-updated date per domain. A `last-verified:` frontmatter field would be redundant with git history and add maintenance burden. The 90-day threshold mirrors the spec Implementation Gap callout staleness rule.
- **Gap D — One-directional cross-reference**: mental-model files embed spec stems inline in body text. Reverse lookup (spec → mental-model) uses grep on the stem. No back-reference in spec files — avoids dual-maintenance risk when stems are renamed.

## Constraints

- spec-updater must remain conservative: never auto-delete body prose. `### Pending removal` is report-only; no file is touched.
- Gap D: when a spec stem is renamed (`renamed-spec: old → new` commit convention), mental-model files referencing it must be updated in the same commit. This constraint belongs in both convention docs.
- Gap B: spec diff interpretation is additive — it does not replace code diff assessment as the primary signal.

## Phases

### Phase 1: Convention docs

Files: `claude/infra/spec-conventions.md`, `claude/infra/mental-model-conventions.md`, `claude/infra/ticket-conventions.md`

**spec-conventions.md:**
- Add a "Feature Removal" authoring rule: when a commit removes a feature, include `removed: <spec-stem>` in the commit's `## Spec` section (one line per removed stem). Document that spec-updater reads this to populate `### Pending removal`.
- Add: when a spec stem referenced in a mental-model file is renamed, that mental-model file must be updated in the same commit.

**mental-model-conventions.md:**
- Add cross-reference guidance: when a domain covers behavior that has a corresponding spec entry, reference the spec stem inline in body text (e.g., `{#260421-slug}`). State that grep on the stem is the reverse lookup; no back-reference needed in spec files.
- Add: when a stem referenced in a mental-model file is renamed, the mental-model file must be updated in the same commit as the rename.

**ticket-conventions.md:**
- Add optional `spec-remove:` frontmatter field: lists spec stems the ticket's implementation will remove from the codebase. Intent-capture only — spec-updater reads the commit `## Spec` section, not the ticket. The field makes the removal intent visible in the ticket itself.

Success: convention docs fully describe the removal and cross-reference protocols with no agent changes needed to understand them.

### Phase 2: spec-updater agent — Pending removal detection

File: `claude/agents/spec-updater.md`

Extend the process:
- After step 3 (Check implementation via commit history), add a step: scan `git log --all` for commits whose message body contains `removed: <spec-stem>`. For each matched stem, locate the corresponding entry in spec files (a heading with that `{#slug}` anchor, without 🚧). Add it to the `### Pending removal` section of the report. Do not modify the spec file.
- Update the Output section to include the `### Pending removal` format:
  ```
  ### Pending removal
  - `<file>`: `Feature Name {#slug}` — flagged by commit <hash>; remove manually after confirmation
  ```

Success: running spec-updater after a removal commit reports the pending-removal entries without touching spec files.

### Phase 3: mental-model-updater agent — spec diff interpretation

File: `claude/agents/mental-model-updater.md`

Extend process step 1 (Determine changes):
- After reading the code diff, also check whether any `ai-docs/spec/` files changed since the base commit (`git diff <base-commit> HEAD -- ai-docs/spec/`).
- For each changed spec file: identify which spec entries were added, modified, or had 🚧 stripped. Map those entries to behavioral domains (by topic and file name conventions).
- Add the identified domains to the assessment list in step 3, in addition to domains derived from code file changes.

Rationale: when spec-updater strips 🚧 (confirming implementation landed), mental-model-updater should verify the corresponding domain reflects the implemented contracts — catching cases where only spec-updater ran but mental-model was not updated.

Success: mental-model-updater's domain assessment list includes domains inferred from spec file changes, not only from code changes.

### Phase 4: discuss skill — 90-day mental-model staleness warning

File: `claude/skills/discuss/SKILL.md`

In the `On: discussion loop` process, when a mental-model domain file is read:
- Run `git log -1 --format="%ai" -- ai-docs/mental-model/<domain>.md`.
- If the resulting date is more than 90 days before today, surface a staleness warning inline: "Domain `<domain>` last updated <date> — consider `/write-mental-model`."
- Mirror the spec Implementation Gap 90-day staleness rule exactly.

Success: discuss sessions surface a one-line staleness warning for any mental-model domain not updated in 90 days. No false positives for recently updated domains.
