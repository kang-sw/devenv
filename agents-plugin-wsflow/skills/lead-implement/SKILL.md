---
name: lead-implement
description: Use when an approved task or ready ticket should be executed in wsflow; routes to direct edit, closes docs, then gates merge or continuation.
---

# Implement

Target: user request

## Invariants

- Lead-owned harness only: route, run doc pipeline, report, and gate continuation.
- Execute code changes through `wsflow:lead-edit`; `lead-edit` owns the implementation strategy.
- Honor caller-provided scope or phase slices as hard implementation boundaries.
- Invoke only the workflow skills named in this file.
- Create the task list at prepare; every task is mandatory and ordered.
- Commit logical units per repository commit rules with `## AI Context`.
- After Final Action Gate, wait for user approval before merging or starting another implementation slice.

## On: invoke

### 1. Assess

1. Parse target: ticket path or inline description.
2. If ticket-driven: read ticket; extract scope, stem, caller-provided slice, and phase results.
3. Apply `judge: branch-mode`.

### 2. Prepare

Create and maintain this task list:

```text
[ ] Confirm scope boundary - preserve caller-provided slice or whole-target scope
[ ] Prepare branch - continue or create the implementation branch
[ ] Execute - invoke wsflow:lead-edit; capture commit range and result commit
[ ] Doc pre-pass - invoke wsflow:lead-update-spec; update mental models directly when source changes require it
[ ] Doc commit gate - refresh ai-docs/_index.md and ticket Result, then commit docs
[ ] Doc closeout compaction - compact safe documentation-only branch-tip suffix
[ ] Final action gate - wait for merge, continue, or stop
[ ] Merge - only when approved
```

1. Record `<current-branch>`.
2. Outside `implement/*`, set `<merge-target>` to `<current-branch>`.
3. On `implement/*`, set `<merge-target>` from caller or confirm before execution.
4. If continuing on `implement/*` and branch name no longer matches selected scope, rename with `git branch -m implement/<scope>`; stop if target branch exists or upstream tracking is ambiguous.
5. Record `<implementation-start>` with `git rev-parse HEAD`.
6. If `branch-mode` = create: create `implement/<scope>` before any source edit.

### 3. Execute

1. Invoke `wsflow:lead-edit` with the target and scope boundary.
2. Capture commit range from `<implementation-start>..HEAD`.
3. Capture `<result-commit>` as current source HEAD before documentation updates.

### 4. Doc Pre-Pass

1. Invoke `wsflow:lead-update-spec` with `<commit-range>`.
2. Update relevant mental-model docs directly when the source diff changed module contracts, coupling, extension points, common mistakes, or technical debt.
3. Commit mental-model changes separately and mention the affected mental-model docs in `## AI Context`.

### 5. Doc Commit Gate

1. Call `wsflow/infra.read(name: "executor-wrapup")`.
2. Refresh `ai-docs/_index.md` for new skills, package surfaces, focus changes, or major patterns.
3. If ticket-driven, add a `### Result (<result-commit>) - YYYY-MM-DD` section to the completed phase.
4. Commit ticket and index changes through `wsflow/git.commit`.

### 6. Doc Closeout Compaction

1. Run this gate for every supported branch mode; implementation runs are always on scoped implementation branches.
2. Inspect commits from `HEAD` backward after Doc Commit Gate; build only the contiguous branch-tip suffix of eligible documentation closeout commits.
3. An eligible commit is non-merge, workflow-owned, and changes only `ai-docs/spec/`, `ai-docs/mental-model/`, `ai-docs/tickets/`, `ai-docs/_index.md`, or narrowly relevant `ai-docs/ref/` workflow docs.
4. Stop suffix collection at the first ineligible commit; never cross source, test, skill, runtime, generated, planning, ready-promotion, review-fix, merge, or ambiguous-authorship commits.
5. If the suffix has fewer than two eligible commits, record `skipped - fewer than two eligible closeout commits`.
6. Compact the suffix into one closeout commit only when metadata synthesis is unambiguous; preserve AI Context, ticket Result references, Updated Tickets, Updated Specs, Mental Model Notes, and doc-audit rationale from absorbed commits.
7. After compaction, verify the final tree matches the pre-compaction head; if equivalence cannot be proven, restore the pre-compaction head and report compaction as skipped with the blocker.

### 7. Final Action Gate

Report:

- implemented changes from edit output;
- documentation updates and ticket Result hash;
- doc closeout compaction status;
- review status from `wsflow:lead-edit`; if `lead-edit` performed no review, report `review: not performed by edit path`;
- test status;
- deviations or open items;
- next unfinished phase, if any.

Stop after reporting and wait for the user to choose merge, continue with a new
slice, or stop. If the user wants more changes, route to a new implementation
slice. Completed ticket Results are append-only; later changes add a new Result
or Edition section instead of editing prior Result text. If the user chooses
stop, leave the implementation branch unmerged and report the branch name plus
merge target.

### 8. Merge

Merge only when the user approves. Merge `implement/<scope>` to
`<merge-target>` with an equivalent non-interactive git sequence.

Use fast-forward for a single workflow-owned, message-clean commit. For multiple
commits, use `--no-ff` by default; fast-forward only when each commit is an
independent deployable and independently revertible target-history unit. Use
squash when the branch is one logical change with noisy or dependent commits.

## Judgments

### judge: branch-mode

| Decision | When |
|----------|------|
| Continue implementation branch | Current branch starts with `implement/` |
| Create implementation branch | Current branch does not start with `implement/` |

## Doctrine

Implement optimizes for **verified code reaching the target branch**. The
harness keeps routing and documentation explicit while `lead-edit` owns
implementation strategy, source integration, and review. When a rule is
ambiguous, preserve the caller-provided scope and keep mutation paths explicit.
