---
title: Implement merge target discovery can select the wrong parent branch
related:
  260514-epic-ws-web-dashboard-mvp: dashboard dogfood exposed nested implementation branch merge risk
---

# Implement merge target discovery can select the wrong parent branch

## Background

Dashboard dogfood exposed a dangerous merge-target case: an `implement/*` branch
derived from `ws-dashboard-dev` attempted to merge toward `main`. The observed
branch ancestry did not support that target. For
`implement/ws-dashboard-workroot-registry-activation`,
`git merge-base ws-dashboard-dev implement/ws-dashboard-workroot-registry-activation`
returns the implement branch tip, while `git merge-base main
implement/ws-dashboard-workroot-registry-activation` returns an older shared
ancestor. A final-action merge target chosen as `main` would bypass the parent
dashboard branch and mix unrelated branch lifecycle boundaries.

The current `lead-implement` text says an `implement/*` branch should use a
caller-provided merge target or confirm before execution. The failure mode
suggests one of these gaps:

- the caller can provide a stale or default target such as `main`;
- the skill lacks a mechanical way to validate that a supplied target is the
  actual parent branch;
- dashboard-spawned or nested worktree flows may need branch ancestry metadata
  that survives the handoff.

## Proposed Direction

Treat "merge base hash" and "workflow merge target" as different contracts.
The runtime already exposes `git.merge_base`, which answers the read-only Git
question for two explicit revisions. That is useful for validation, but it does
not decide which branch is the intended parent when several branches contain or
pre-date the implementation branch.

Consider adding a higher-level read-only Git/workflow API that resolves a
candidate merge target for the current branch. Possible shape:

- inputs: `head` defaulting to `HEAD`, optional `candidates`, optional
  `prefer_upstream`, optional expected branch prefix;
- output: selected target branch, merge-base hash, confidence, evidence, and
  ambiguity reasons;
- safety: never defaults to `main` merely because it is the default branch;
- ambiguity: returns "needs confirmation" when multiple candidates are plausible
  or when the best candidate is only an old ancestor.

Skill guidance should then require `lead-implement` to validate any
caller-provided merge target before the Final Action Gate and stop for explicit
user confirmation when the target is not proven by branch ancestry or captured
handoff metadata.

## Confirmed Direction (2026-08-07): self-describing `impl/<merge-root>/<stem>`

Discussion settled on making the impl branch name self-describing about its merge
target, rather than deriving the target only from agenda/policy — the current
`deriveImplementBranchPlan` behavior, which stops the plan when policy is absent
and was the source of the observed wrong-parent merges.

- **Chosen shape: `impl/<merge-root>/<stem>`** (Option A). The `impl/` prefix is
  kept, so autodelete (the literal `impl/*` glob at `lead-implement.md:96`), the
  prefix gates (`strings.HasPrefix(branch, "impl/")` in `implement_resolver.go`),
  and goal-run nesting all survive. `<stem>` keeps its meaningful scope-slug (not
  a random slug), so branch names stay human-scannable.
- **Parsing is unambiguous** because `<stem>` is guaranteed single-segment
  (non-slash): strip the `impl/` prefix and split on the LAST `/` — everything
  before is `<merge-root>` (which may itself contain slashes, e.g.
  `impl/feature/foo/<stem>`), and the final segment is `<stem>`. This is exactly
  the parse rule the `goal/<parent>/<slug>` convention already uses, which is why a
  slashed merge-root is safe. Single-segment legacy `impl/<stem>` has no slash and
  falls back to the default target (`main`) — backward compatible.
- **Rejected: renaming impl onto the `goal/` namespace directly.** `goal/` was
  cited only as prior art. A literal `goal/<merge-root>/<stem>` rename breaks two
  ways: (1) impl branches are created *inside* goal runs (fan-out workers,
  drain-queue tickets), so it produces `goal/.../goal/...` and breaks the goal
  parser ("strip `goal/`, split on last `/`"); (2) goal slugs are deliberately
  random for concurrent-run collision avoidance while impl stems are meaningful —
  merging the namespaces forces one property to be lost. Converging only the
  *structure* (`<prefix>/<merge-root>/<stem>`) while keeping the `impl/` prefix
  avoids both.
- **Sub-item — git ref D/F conflict.** git cannot hold `impl/foo` (a loose-ref
  file) and `impl/foo/bar` (a directory) at once, so a leftover legacy
  single-segment `impl/<stem>` can block creating `impl/<stem>/...`. The resolver
  should detect a conflicting legacy ref on create and clean up or warn. Low risk
  in practice (impl branches are autodeleted post-merge).

This turns the "merge-base hash ≠ workflow merge target" distinction above into a
name-encoded target: the resolver reads `<merge-root>` from the branch name
instead of requiring a merge-target policy or inferring an ambiguous base.
Coupling to update: `implementTargetBranchName` and the `impl/`-prefix checks in
`implement_resolver.go`, the impl branch-name test literals, the convention
statements in `spec/mcp-tools.md` and `spec/workflow-skills.md`,
`mental-model/workflow-skills.md`, and the mirrored skill trees (`agents-plugin/`
+ `agents-plugin-wsflow/`). Ready-ticket-sized, not a quick edit.
