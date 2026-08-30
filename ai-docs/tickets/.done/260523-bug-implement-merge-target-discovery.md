---
title: Implement merge target discovery can select the wrong parent branch
related:
  260514-epic-ws-web-dashboard-mvp: dashboard dogfood exposed nested implementation branch merge risk
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-10
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

> **Superseded** by the `## Confirmed Direction` and `## Resolved Decision` below.
> This section is the original exploration (a generic merge-target-discovery API)
> and is retained only for rationale; Phase 1 implements the name-encoding
> approach, not this API. Do not build the resolver API sketched here.

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
  treats that case is **settled** (see **Resolved Decision** below): it **stops
  and asks** for an explicit merge target — the safe gate already implemented at
  `implement_resolver.go:702-706` — and is explicitly **not** a silent default to
  `main`, which would reproduce the wrong-parent incident this ticket exists to
  fix.
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

## Resolved Decision

**Rootless / legacy fallback contract — settled (2026-08-10): preserve
stop-and-ask.** For a branch with no merge-root segment (rootless `impl/<stem>`
on re-entry, or a legacy `implement/<stem>`), the resolver **stops and asks** for
an explicit merge target, exactly as the current code already does ("merge target
required while already on an implementation branch", `implement_resolver.go:702-706`).
It never silently defaults to a target.

- **Chosen (a) preserve stop-and-ask.** A rootless/legacy branch with no supplied
  target stops for explicit confirmation. Keeps the wrong-parent incident from
  recurring; no silent target. This is a no-behavior-change for the legacy/rootless
  path — the ticket's new work is purely the `impl/<merge-root>/<stem>` encoding for
  freshly-created branches.
- **Rejected (b) default to `main`.** Convenient but reproduces the wrong-parent
  merge for precisely the branch class that triggered the incident; both the design
  reviewer and lead judged it unsafe. Rationale for not carrying migration
  exceptions into the resolver: a rootless/legacy branch is exactly the ambiguous
  provenance the safe convention exists to catch, and one-off migration cases do not
  belong hard-coded in the skill rulebook.

## Spec Impact

The impl branch-name convention is stated in specs; encoding
`impl/<merge-root>/<stem>` changes those caller-visible statements.

- **`spec/workflow-skills.md`**: update the `implement` / `lead-implement` branch
  convention to the `impl/<merge-root>/<stem>` shape and the split-on-last-slash
  parse rule, and document the single-segment rootless/legacy `impl/<stem>`
  contract: the resolver stops and asks for an explicit merge target, never a
  silent default. Caller-visible change: an impl branch's merge target is read
  from its name, not inferred from agenda/policy.
- **`spec/mcp-tools.md`**: update the `implement`-resolver branch-name contract
  (`implementTargetBranchName`, the `impl/`-prefix behavior) so the merge target
  is the name's `<merge-root>`. Caller-visible change: an impl branch created with
  a non-`main` merge-root merges toward that root, not `main`.

## Phases

### Phase 1: Encode impl/<merge-root>/<stem> in the resolver, convention, and mirrors

Implement the Confirmed Direction above (rootless/legacy fallback = stop-and-ask,
per the Resolved Decision):

- `implementTargetBranchName` builds `impl/<merge-root>/<stem>`. Encoding the root
  makes the name depend on the current branch, so both call sites —
  create and the `enter.implement` preflight (`session_state.go:1020`, feeding
  `observeImplementBranch`'s `TargetExists` check) — must observe the current
  branch before building the name.
- On re-entry/continue, `deriveImplementBranchPlan` reads the merge target from the
  **existing branch name** (strip the `impl/` prefix, split on the last `/`:
  everything before is `<merge-root>`, the final segment is `<stem>`), not from the
  current branch; this is the actual bug scenario. A rootless/legacy single-segment
  branch has no `<merge-root>` — it stops and asks for an explicit merge target
  (the existing `implement_resolver.go:702-706` gate), never a silent `main`.
- **Name-root precedence (load-bearing).** On a name-rooted impl branch the
  name-derived `<merge-root>` is **authoritative**: it supersedes any
  caller-supplied `merge_target` policy that diverges from it, so the encoding
  actually fires. Today `deriveImplementBranchPlan` sets
  `plan.MergeTarget = n.MergeTargetPolicy` unconditionally
  (`implement_resolver.go:688`) before the stop gate — a naive least-change edit
  that keeps that line would let a stale caller-supplied `main` override the
  name's root and reproduce the wrong-parent merge on exactly the branch class
  that triggered the incident. Required behavior: when the name carries a
  `<merge-root>`, derive the target from the name; if a caller target diverges from
  it, reconcile to the name-root or stop-and-ask — never silently honor the
  divergent caller target. (A caller target is only the sole source for a
  rootless/legacy branch, which stops-and-asks per the Resolved Decision.)
- **Preserve the single-segment `<stem>` invariant the parse relies on.** The
  split-on-last-slash parse is only unambiguous while `<stem>` has no slash.
  `implementTargetBranchName` (`implement_resolver.go:677-680`) currently only
  trims a trailing `-`; it must strip or reject slashes in the `ScopeSlug` so a
  slashed stem can never misattribute part of the stem to `<merge-root>`. Add or
  confirm this sanitization at name-build time.
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
that branch re-derives `ws-dashboard-dev` from the name (not the current branch);
re-entering a name-rooted branch with a **divergent** caller-supplied
`merge_target` (e.g. `main`) does not silently honor it — the name-root wins or the
resolver stops-and-asks; a rootless/legacy single-segment branch stops and asks for
an explicit merge target (no silent `main`); a slashed `ScopeSlug` is sanitized so
`<stem>` stays single-segment; the D/F-conflict path warns/stops and is exercised
by a test.

### Result (e1a83399) - 2026-08-10

Encoded `impl/<merge-root>/<stem>` in the implement branch resolver and drained
the full coupling set. Behavioral delta:

- `implementTargetBranchName(mergeRoot, scopeSlug)` now builds the rooted name and
  sanitizes any `/` in `scopeSlug` (→ `-`) so a caller-supplied `target.scope_slug`
  can never break the single-segment `<stem>` invariant.
- `deriveImplementBranchPlan` split three ways: **create** derives merge-root from
  the current branch; **name-rooted re-entry** parses the root from the branch name
  via new `parseImplBranchRoot` (strip `impl/`, split on last `/`) and treats it as
  authoritative; **rootless `impl/<stem>` / legacy `implement/<stem>`** keep today's
  stop-and-ask byte-for-byte. Divergent-caller resolution landed as
  **reconcile-with-warning** (the ticket-permitted alternative to stop-and-ask):
  a diverging `merge_target` on a name-rooted branch is overridden to the name-root
  and a warning is appended — never silently honored.
- New `MergeRootRefConflict` observation field + a git-level ancestor-path D/F scan
  in `observeImplementBranch` stop create instead of silently colliding with a
  legacy single-segment ref; non-destructive (no auto-delete).
- `enter.implement` preflight fixed an ordering bug: it now peeks the current branch
  (`observeImplementBranch(root, "")`) before building the root-dependent target
  name, then re-observes for real.
- Spec/mental-model prose (`spec/mcp-tools.md`, `spec/workflow-skills.md` ×3,
  `mental-model/workflow-skills.md` ×2) and both mirrored `lead-goal-fan-out-step.md`
  copies updated to the new shape; rsrc manifests regenerated (mirror-drift CI gate).

Verification: `go test ./internal/mcp/... -run Implement -v` and `go test ./...`
(all 12 packages) pass. New `TestResolveImplementMergeRootEncoding` covers the five
resolver invariants; `TestEnterImplementCreatePathMergeRootRefConflictDetectedByRealGit`
exercises the real git-backed D/F detection (proven load-bearing via a mutation test).
Mirror copies confirmed byte-identical.

Review: partitioned (correctness / fit / test), all clean after one relay cycle —
the sole Important finding (git-level D/F detection had no positive-conflict test)
was fixed by the git-backed test above. One non-blocking correctness Minor is
deferred as-is: an empty `scopeSlug` with a non-empty `mergeRoot` yields a
trailing-slash `impl/<root>/`; no worse than the pre-encoding rootless behavior and
`ScopeSlug` is validated non-empty upstream of the helper.

Phase 1 is the ticket's only phase; ticket complete.
