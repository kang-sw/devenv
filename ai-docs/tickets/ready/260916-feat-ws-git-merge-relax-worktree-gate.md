---
title: Relax ws/git.merge worktree gate to git-native semantics and clarify target-landing contract
related:
  260911-feat-ws-git-merge-lead-owned-merge-authority: prerequisite
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3454e81f5d22c67f
sage-review-completeness-reviewed: 3454e81f5d22c67f
---

# Relax ws/git.merge worktree gate to git-native semantics and clarify target-landing contract

## Background

`ws/git.merge`'s cleanliness precondition (`checkWorktree` in
`agents-plugin-tool/internal/mcp/git_merge.go`, called both pre-switch and again
post-switch) runs `git status --porcelain=v1 --untracked-files=all` and raises a
`must_resolve` `dirty_worktree` diagnostic on **any** non-empty output — staged,
unstaged, *and* untracked. This is far stricter than plain git and produces
recurring dogfood clutter: an untracked file created mid-flow (e.g. an `idea/`
ticket) blocks an otherwise-clean merge, forcing a move-to-scratchpad / merge /
restore shuffle. The surprise surfaced repeatedly during a ship + goal-run
session.

The tool is not plain `git merge`: it internally `git switch --no-guess`es to the
derived target root branch and then runs
`git merge --no-ff --no-squash --commit <sourceOID>`. So the honest comparison is
`git switch <target> && git merge`, and git's own guards already provide exactly
the safe subset we want — see `## Decisions`.

Owner policy (confirmed): the tool does **not** switch back to the caller's
original branch. It lands the caller on the target branch by design, and this
ticket keeps that policy (no switch-back, no rename this pass).

## Decisions

### The blanket gate is stricter than git's own guards (empirically verified)

Verified in an isolated repo this session with `git merge --no-ff --commit`:

| working state (non-overlapping with merged paths) | plain `git merge` | folded into merge commit? | preserved after? |
|---|---|---|---|
| dirty **index** (any staged change, or unmerged) | **refuses** | — | yes, untouched |
| unstaged tracked modification | proceeds | no | yes, in working tree |
| untracked file | proceeds | no | yes |
| change **overlapping** a merged path | refuses (at switch or merge) | — | yes |

So git already refuses precisely the dangerous case (dirty index / overlap, both
fail closed with clean aborts) and safely tolerates non-overlapping
unstaged/untracked changes without ever folding them into the merge commit. The
tool's blanket porcelain gate adds no safety over this — it only adds friction.

### Confirmed changes

1. **Narrow `checkWorktree`** (covers both call sites via the shared closure):
   reject only a **dirty index** (staged changes) or **unmerged paths**, plus the
   existing in-progress `MERGE_HEAD` refusal (`merge_in_progress`, keep as-is).
   Allow non-overlapping unstaged-tracked modifications and untracked files.
   Delegate index-clean and overlap safety to git's native `switch`/`merge`,
   which already fail closed.
2. **Keep the `--commit` flag.** It is git's default and is exactly what produces
   the clean refuse-on-dirty-index behavior. Do **not** drop it or switch to
   `--no-commit` (which would only defer the commit, not improve safety).
3. **Post-merge-success nudge.** After the merge succeeds, if the working tree is
   still dirty, surface a **non-blocking advisory** (Classification `advisory`,
   like the existing `cleanup_failed` diagnostic — not `must_resolve`) that
   **names the branch the caller now sits on** (the target), e.g. "merged; now on
   `<target>`; N dirty entr(y/ies) remain here — verify." This exists because
   switch-back is not policy: allowed unstaged/untracked changes travel with the
   internal `git switch` and remain stranded on the target checkout.
4. **Strengthen the `git.merge` tool description** (its MCP schema description
   string in the `tools/list` registration) to state the contract explicitly: it
   switches to and lands the caller on the target branch and does **not** switch
   back, and it tolerates a dirty working tree but still requires a clean index.
   This string is agent-facing MCP output, so `shipped-surface-boundary.md`
   applies.

### Rejected / deferred

- **Rename** the tool (`git.merge_into`, `git.promote`) — deferred this pass.
  Kept as a possible future `idea/` ticket only; not authorized here. The
  description strengthening (change 4) carries the target-landing signal instead.
- **Adding switch-back** — rejected; landing on the target is deliberate owner
  policy (e.g. release flows end on the target to tag).
- **Full plain-git parity that also allows a dirty index** — rejected; git itself
  refuses a dirty index for a `--commit` merge, so narrowing to index-only keeps
  the tool's diagnostic aligned with git's own behavior and avoids a stranded
  half-committed state.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- `agents-plugin-tool/internal/mcp/` edits: read `ai-docs/manuals/ws-mcp.md`
  before editing.
- The strengthened description is agent-facing shipped-surface text: read
  `ai-docs/manuals/shipped-surface-boundary.md` before editing it; keep it
  downstream-neutral (no devenv-only assumptions).
- Overlap now surfaces later than before: with the gate narrowed, a
  change that overlaps a merged path reaches the internal `git switch`
  (`run("switch", ...)`) or the `git merge` step and fails there. The `git merge`
  failure path already yields a `conflict` status/advisory; ensure the
  `git switch` failure path (currently `return result, err` on switch error)
  surfaces an **actionable** result rather than a bare error string, so a
  now-permitted-but-overlapping working-tree change does not degrade the caller
  experience.
- Preserve every existing diagnostic and the release-target / OID-acknowledgement
  / containment / review-frontier logic unchanged; this ticket only changes the
  `dirty_worktree` branch of `checkWorktree`, adds the success nudge, and edits
  the description string.
- Keep the caller-lands-on-target behavior; do not add a switch-back.

## Prior Art

- `agents-plugin-tool/internal/mcp/git_merge.go` — `mergeImplBranch` and the
  `checkWorktree` / `checkTips` closures; the `advisory`-classification
  `cleanup_failed` diagnostic is the shape to mirror for the nudge.
- The `git.merge` schema/description lives in the `tools/list` registration in
  `agents-plugin-tool/internal/mcp/server.go`.
- `260911-feat-ws-git-merge-lead-owned-merge-authority` (`.done/`) — introduced
  the tool and the original clean-tree gate; its rationale was fast-forward-flatten
  prevention and removing free-form git from the worker, not clean-tree per se.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/git_merge.go, agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/mcp/git_merge_test.go |
| scope.surface | public-interface | the git.merge MCP tool's behavior and description contract (agents-plugin-tool/internal/mcp/server.go#L3559-L3577); no new Go exported symbol |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/git_merge_test.go (e.g. TestImplMergeRefusals, TestImplMergeConflictAdvisory) |
| complexity.reuse_points | confirmed | the add/gitFailure diagnostic closures and the advisory-classification cleanup_failed diagnostic shape (agents-plugin-tool/internal/mcp/git_merge.go#L120-L133, #L314-L319) |
| complexity.side_effect_risk | moderate | narrows a safety gate used by every git.merge call; a parsing error in the narrowed check could silently admit an unsafe merge |
| risk.correctness | moderate | requires correctly distinguishing staged/unmerged porcelain codes from unstaged/untracked ones in checkWorktree (agents-plugin-tool/internal/mcp/git_merge.go#L176-L196) |
| risk.fit | low | the ticket's Decisions and Rejected/deferred sections already resolve the product choices, leaving little open product ambiguity |
| risk.test | moderate | five new scenarios (untracked proceeds, non-overlapping unstaged proceeds and is excluded from the commit, staged blocks, overlap blocked actionably, dirty-remains nudge) must be added to git_merge_test.go |
| risk.security_or_contract | moderate | changes a safety gate and the shipped git.merge tool description, a shipped-surface contract callers rely on |

## Phases

### Phase 1: Narrow the worktree gate, add the target-named nudge, and clarify the description

Change the `dirty_worktree` check in `checkWorktree` to reject only a dirty index
or unmerged paths (keep the `MERGE_HEAD` `merge_in_progress` refusal), allowing
non-overlapping unstaged and untracked changes. Add a post-merge-success,
non-blocking advisory that fires when the working tree is still dirty and names
the target branch the caller now sits on. Strengthen the `git.merge` tool
description to state the switch-to-and-land-on-target (no switch-back) and
clean-index-but-dirty-worktree-tolerated contract. Ensure the `git switch`
failure path surfaces an actionable result for a now-permitted overlapping
change.

Verification: extend the existing `git_merge` tests in
`agents-plugin-tool/internal/mcp` to cover — untracked file present → merge
proceeds; non-overlapping unstaged modification → merge proceeds and the change
is not in the merge commit; staged change → pre-empt `dirty_worktree` (still
blocked); unmerged paths present without an active `MERGE_HEAD` (e.g. a stalled
cherry-pick leaving conflict entries) → also pre-empt `dirty_worktree` (still
blocked); overlapping change → blocked (via switch/merge native guard, surfaced
actionably); and the dirty-remains nudge fires after a successful merge naming
the target branch. `go test ./...` in `agents-plugin-tool` green.
