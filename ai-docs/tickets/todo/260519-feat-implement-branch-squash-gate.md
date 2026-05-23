---
title: implementation doc closeout compaction gate
related-mental-model:
  - workflow-skills
  - git-workflow-tools
---

# implementation doc closeout compaction gate

## Background

`ws:lead-implement` currently leaves implementation branch history in the same
granular shape used for recovery checkpoints: brief, plan, implementation,
review fixes, spec updates, mental-model updates, and ticket closeout are often
separate commits. That is useful while the branch is in flight, but the main
history noise that motivated this ticket is mostly the stack of small
post-implementation documentation commits created by the final doc pipeline.

Full implementation-branch history cleanup is intentionally out of scope. Long
flows can contain main merges, conflict-resolution merge commits, user commits,
and source/review checkpoints. Rewriting that whole graph would turn
`lead-implement` into a git history surgery workflow whose risk exceeds the
value of a cleaner log.

The desired policy is narrower: after the final doc gate has completed and
before merge readiness is reported, `ws:lead-implement` should compact only the
contiguous post-implementation documentation closeout suffix when that suffix is
mechanically safe to combine.

## Decisions

- Preserve pre-implementation planning history. Briefs, surveys, planned spec
  entries, ready-ticket promotion, and other setup commits explain why the
  implementation started and are not closeout compaction targets.
- Preserve source, test, review-fix, and merge commits. The gate is not a
  general branch squash, rebase, or merge-commit rewrite.
- Consider only the branch tail after the final doc gate. Eligible commits must
  be a contiguous suffix of documentation-only closeout commits.
- Documentation-only closeout commits are commits whose changed paths are
  limited to project documentation surfaces such as `ai-docs/spec/`,
  `ai-docs/mental-model/`, `ai-docs/tickets/`, `ai-docs/_index.md`, and
  narrowly relevant workflow reference docs.
- If the suffix has zero or one eligible commit, skip compaction.
- If the suffix includes source, tests, skill files, runtime code, generated
  artifacts, merge commits, or ambiguous user/foreign-authored commits, skip
  compaction and report why.
- The compacted commit message must preserve important metadata from the
  compacted commits, including AI Context rationale, ticket Result references,
  Updated Tickets, Updated Specs, Mental Model Notes, and review/doc-audit
  rationale.
- wsflow should use the same simple suffix-only rule if mirrored; it should not
  gain a specialist branch-history workflow.

## Phases

### Phase 1: Add post-implementation doc closeout compaction

Update `ws:lead-implement` so implementation-branch modes run a doc closeout
compaction check after the final doc gate and before the final action gate
reports merge readiness.

The check should inspect the merge-target-relative branch range and identify
only the contiguous documentation-only suffix at the current branch tip. It
should compact that suffix into one closeout commit only when the changed paths,
commit authorship/ownership, and metadata synthesis are unambiguous.

The expected final branch history should keep planning and source/review
checkpoints visible while reducing post-implementation documentation noise:

- `docs(plan): prepare <scope>` or equivalent pre-implementation planning
- `<type>(<scope>): implement <scope>`
- optional source/review-fix commits
- `docs(ticket): close <scope>`

The closeout commit may include spec, mental-model, ticket Result, index, and
doc-audit updates together. It must not absorb pre-implementation planning,
ready-promotion, source, test, or review-fix commits.

If the suffix is unsafe to compact, the gate should skip compaction and include
the reason in the final action gate report. Unsafe suffixes are not blockers for
merge readiness; the fallback remains a normal lead-owned merge with the branch
history preserved.

Verification should cover:

- the skill text clearly places the compaction check after the final doc gate
  and before the final action gate;
- the eligible range is limited to a contiguous documentation-only branch-tip
  suffix;
- pre-implementation planning, ready-promotion, source, test, review-fix, and
  merge commits are explicitly excluded;
- the fallback path skips compaction without blocking merge readiness;
- compacted commit messages preserve workflow metadata from the absorbed doc
  commits;
- downstream merge reporting states whether closeout compaction was applied or
  skipped and why.
