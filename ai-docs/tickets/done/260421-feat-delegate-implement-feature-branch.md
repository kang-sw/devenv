---
title: "delegate-implement: feature-branch auto-merge mode"
spec:
  - 260421-delegate-implement-feature-branch-mode
---

# delegate-implement: feature-branch auto-merge mode

## Background

`/delegate-implement` always presents a user approval gate before merging the implementation sub-branch. When working inside a feature branch (non-main/master/trunk), this gate is unnecessary overhead — the feature → main merge is the real quality gate. The approval consequence is lower-stakes (merging to feature, not to a protected branch), and doc updates defer with each tweak loop, creating anxiety about doc state.

The intended workflow: one feature branch per ticket, one or more `/delegate-implement` invocations per feature branch (each on its own sub-branch), all auto-merging to the feature branch. The user gates only the final feature → main merge.

## Decisions

- **Auto-detect approach**: if `original-branch` at invocation time is not `main`, `master`, or `trunk` → enter feature-branch mode. No explicit flag required at call time.
- **Rejected — explicit `--auto-merge` flag**: ergonomically worse; must be typed on every invocation. Auto-detect is unambiguous given the branch-naming convention.
- **Override option**: `--main-branch <name>` flag to support non-standard protected branch names (e.g., `production`, `release`).
- **Trade-off acknowledged**: without the per-unit gate, a bad implementation unit lands on the feature branch before the user sees it. Recovery requires reverting from the feature branch rather than rejecting at the gate. Acceptable when the reviewer process is trusted.

## Phases

### Phase 1: Implement feature-branch mode in `/delegate-implement`

Modify `claude/skills/delegate-implement/SKILL.md` to:

1. **Detect at Prepare (step 1)**: check whether `original-branch` matches `main`, `master`, or `trunk` (or the value of `--main-branch` if provided).
   - If not → `feature-branch mode = true`.
2. **At step 5 (Report and approval)**: if `feature-branch mode = true` → skip user gate entirely; proceed directly to step 6 (Merge).
3. **Doc pipeline**: runs post-auto-merge, same position as current flow, just without the preceding gate pause.
4. **Main-branch invocation**: `feature-branch mode = false` → existing behavior fully preserved, no change.

Success criteria:
- Invoking `/delegate-implement` from a feature branch completes without pausing for user approval.
- Invoking from `main`/`master`/`trunk` still presents the approval gate.
- Doc pipeline runs and completes after auto-merge in both modes.

### Result (c6ea74d) - 2026-04-21

Implemented as specified. Mode detection added at step 1 (Prepare); step 5 skip note added for feature-branch mode; invariants updated to document both modes; `--main-branch <name>` override flag documented. No deviations from the plan.
