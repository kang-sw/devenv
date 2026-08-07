---
title: Implement merge target discovery can select the wrong parent branch
related:
  260514-epic-ws-web-dashboard-mvp: dashboard dogfood exposed nested implementation branch merge risk
sage-review-design: required
sage-review-completeness: required
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
  slashed merge-root is safe. A single-segment branch (rootless `impl/<stem>`, or a
  legacy `implement/<stem>` — the resolver still gates both prefixes at
  `implement_resolver.go:577-578,696`) carries no merge-root; how the resolver
  treats that case is an **Open Decision** below, and is explicitly **not** a
  silent default to `main`, which would reproduce the wrong-parent incident this
  ticket exists to fix.
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
  should detect a conflicting legacy ref on create and **warn/stop rather than
  auto-delete** — a stale `impl/<stem>` may hold unmerged commits, and the
  existing cleanup gate at `lead-implement.md:96` already refuses to delete
  branches with commits unreachable from the target, so a non-destructive warn is
  the convention-consistent choice. Low risk in practice (impl branches are
  autodeleted post-merge).

This turns the "merge-base hash ≠ workflow merge target" distinction above into a
name-encoded target: the resolver reads `<merge-root>` from the branch name
instead of requiring a merge-target policy or inferring an ambiguous base.
Coupling to update: `implementTargetBranchName` and the `impl/`-prefix checks in
`implement_resolver.go`, the impl branch-name test literals, the convention
statements in `spec/mcp-tools.md` and `spec/workflow-skills.md`,
`mental-model/workflow-skills.md`, and the mirrored skill trees (`agents-plugin/`
+ `agents-plugin-wsflow/`). Ready-ticket-sized, not a quick edit.

## Open Decisions

Recorded as OPEN — needs a user decision before ready (surfaced by design review,
resolution: missing):

- **Rootless / legacy fallback contract.** For a branch with no merge-root segment
  (rootless `impl/<stem>` on re-entry, or a legacy `implement/<stem>`), the
  resolver must pick one contract. The current code already **stops and asks**
  ("merge target required while already on an implementation branch",
  `implement_resolver.go:702-706`) — a safe gate. The two options:
  - **(a) Preserve stop-and-ask** (recommended): a rootless/legacy branch with no
    supplied target stops for explicit confirmation, exactly as today. Keeps the
    incident from recurring; no silent target.
  - **(b) Default to `main`**: convenient but reproduces the wrong-parent merge for
    precisely the branch class that triggered the incident. The reviewer and I
    judge this unsafe.
  Until this is chosen, Phase 1's fallback behavior is unspecified. The
  recommendation is (a); it is recorded here rather than assumed because it is a
  caller-visible contract decision.

## Spec Impact

The impl branch-name convention is stated in specs; encoding
`impl/<merge-root>/<stem>` changes those caller-visible statements.

- **`spec/workflow-skills.md`**: update the `implement` / `lead-implement` branch
  convention to the `impl/<merge-root>/<stem>` shape and the split-on-last-slash
  parse rule, and document the single-segment legacy `impl/<stem>` fallback to the
  default target. Caller-visible change: an impl branch's merge target is read
  from its name, not inferred from agenda/policy.
- **`spec/mcp-tools.md`**: update the `implement`-resolver branch-name contract
  (`implementTargetBranchName`, the `impl/`-prefix behavior) so the merge target
  is the name's `<merge-root>`. Caller-visible change: an impl branch created with
  a non-`main` merge-root merges toward that root, not `main`.

## Phases

### Phase 1: Encode impl/<merge-root>/<stem> in the resolver, convention, and mirrors

Implement the Confirmed Direction above (fallback contract per Open Decision):

- `implementTargetBranchName` builds `impl/<merge-root>/<stem>`. Encoding the root
  makes the name depend on the current branch, so both call sites —
  create and the `enter.implement` preflight (`session_state.go:1020`, feeding
  `observeImplementBranch`'s `TargetExists` check) — must observe the current
  branch before building the name.
- On re-entry/continue, `deriveImplementBranchPlan` reads the merge target from the
  **existing branch name** (strip the `impl/` prefix, split on the last `/`:
  everything before is `<merge-root>`, the final segment is `<stem>`), not from the
  current branch; this is the actual bug scenario. A rootless/legacy single-segment
  branch has no `<merge-root>` — its behavior is the Open Decision, not a silent
  `main`.
- Keep the `impl/` prefix so autodelete (`impl/*` at `lead-implement.md:96`) and
  the `strings.HasPrefix(branch, "impl/")` gates survive.
- Detect the git ref D/F conflict on create and warn/stop (see the D/F sub-item —
  non-destructive, no auto-delete of a possibly-unmerged legacy ref).
- Update the coupling set: the impl branch-name test literals,
  `spec/mcp-tools.md`, `spec/workflow-skills.md`,
  `mental-model/workflow-skills.md`, the goal-staging / fan-out anchors that state
  "one `impl/<stem>` branch" (`workflow-skills.md` `260707-drain-goal-branch-staging`
  ~L555-558 and the `lead-goal-fan-out-step` text ~L596-608, which become
  `impl/<goal-branch>/<stem>`), and the mirrored skill trees (`agents-plugin/` +
  `agents-plugin-wsflow/`).

Verification: an impl branch created for a ticket whose merge root is a non-`main`
branch (e.g. `ws-dashboard-dev`) is named `impl/ws-dashboard-dev/<stem>` and its
resolver-selected merge target is `ws-dashboard-dev`, not `main`; **re-entering**
that branch re-derives `ws-dashboard-dev` from the name (not the current branch); a
rootless/legacy single-segment branch follows the Open Decision's chosen contract
(no silent `main`); the D/F-conflict path warns/stops and is exercised by a test.
