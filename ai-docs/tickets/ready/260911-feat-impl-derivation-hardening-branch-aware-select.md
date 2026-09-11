---
title: "Impl-branch derivation hardening and branch-aware selection via a ticket-selector playbook"
related:
  260911-research-impl-lifecycle-merge-authority-goal-loop-rehoming: context; the closed design this ticket derives from (research, stays in todo/, not a code prerequisite)
  260911-feat-ws-git-merge-lead-owned-merge-authority: adjacent; branch-aware Select's merge-recommendation terminal points at that ticket's merge gate
  260911-refactor-lead-run-ticket-only-delegate-implementer: prerequisite; it rewrites the same lead-run.md Select region (removes the ad-hoc Select-skip and the intro/Spawn ad-hoc rows) and regenerates the same exact-prose goldens — land it first and rebase this Select rewrite onto its result
  260910-feat-lead-run-worktree-parallel-route: adjacent; the future fan-out mode's batch selection is a superset of the branch-aware Select added here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0896d57c0cba5a32
sage-review-completeness-reviewed: 0896d57c0cba5a32
---

# Impl-branch derivation hardening and branch-aware selection

## Background

Diagnosis (recorded in the research ticket) established that the impl-branch
derivation policy in `agents-plugin-tool/internal/mcp/implement_resolver.go` is
robust and byte-identical between develop and epic/refound — the invariants hold.
Two narrow residual defects remain, and one downstream capability is missing:

1. A safety guard fails **open**: `aheadOfMergeRootCount` returns `0` on any git
   error, silently disabling the "different ticket on an impl branch stops" guard
   exactly when git misbehaves.
2. The stable per-stem slug is an opaque 3-word hash (`wimp-frame-suing`), which
   reads as per-run randomness — this is why the "same ticket continues unmerged"
   invariant was *suspected* broken though it holds — and, being non-reversible,
   blocks any branch → owning-ticket lookup.
3. `lead-run`'s Select ignores the git branch, so it can pick a different ticket
   while sitting on an in-progress impl branch — a wasted spawn the resolver then
   stops. Select should finish the branch's owning ticket, or (owning ticket
   closed, branch unmerged) raise a merge recommendation instead of spawning.

The slug hardening (2) is the precondition for the reverse map (3), so both live
in one ticket, phased, with one worker owning the slug contract end-to-end. The
fail-open fix (1) is an independent safety fix in the same file, landed first.

## Decisions

- **The ahead-count guard fails closed.** A safety guard that self-disables on
  error is backwards. On a git error, the resolver surfaces the unverifiable state
  (stop-and-report) rather than proceeding as if ahead=0. Rejected: leaving it
  fail-open — it defeats the very invariant it guards precisely when state is
  uncertain.
- **The slug becomes readable-prefix + short-deterministic-hash-suffix.** e.g.
  `impl/<root>/lead-run-8fa2`: a readable stem-derived prefix plus a short hash of
  the full stem. Keeps per-stem determinism and collision-safety while restoring
  human recognition, and — critically — lets a resolver helper re-derive the slug
  from a stem to build the branch → owning-ticket reverse map, without relying on
  a model's semantic guess. Rejected: full-stem slug (zero-logic, git ref limits
  are not a constraint, but long and noisy in branch listings); keeping the opaque
  hash (blocks the reverse map entirely).
- **Branch-aware Select is extracted into a renderable `ticket-selector`
  playbook.** The branch-aware logic is shared by lead-run's serial mode, the
  future fan-out mode's batch selection (a superset), and the wsflow mirror; one
  rendered playbook beats inlined duplication and unifies the cognition point.
  Split of concern: `ticket-selector` **orchestrates** (branch detect →
  owning-ticket lookup → ordering → merge-recommendation terminal); the
  resolver/tool computes the **deterministic** branch → owning-stem and slug
  derivation. Rejected: inlining the rules directly into lead-run — duplicated
  across serial/fan-out/wsflow, and encodes non-trivial branch logic in prose.
- **The branch → owning-stem lookup extends an existing resolver output, not a new
  MCP tool.** It is advisory data the `ticket-selector` playbook reads, not a
  mutation, so it rides on `route.resolve_implement`'s output rather than adding an
  Ask-first tool surface. The reverse scan covers both `ready/` and `.done/` stems:
  a closed ticket has left `ready/` (status is directory-based), so a `ready/`-only
  scan could never produce its slug, and the "owning ticket closed, branch
  unmerged" case would be undetectable. Rejected: a dedicated new MCP tool for the
  lookup (needless Ask-first surface for read-only advisory data).
- **New playbook = Ask-first at implementation time.** `ticket-selector` is a new
  shipped playbook surface; the resolver-output extension above is not.

## Constraints

- `agents-plugin-tool/internal/mcp/` — read `ai-docs/manuals/ws-mcp.md`; the
  fail-closed change and the slug/reverse-map helpers are resolver-local. Preserve
  the derivation invariants that hold (same-ticket continue; different-ticket
  stop; never nests).
- `agents-plugin/rsrc/`, `agents-plugin/skills/` — read
  `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/shipped-surface-boundary.md`; the `ticket-selector` playbook and
  the lead-run Select edit ship downstream and must not depend on repo-only facts.
- `agents-plugin-wsflow/` — read `ai-docs/manuals/wsflow-mirroring.md`; mirror the
  Select extraction and the new playbook into the wsflow derivative.
- The readable slug is a change to how impl branch names are computed. Existing
  live `impl/*` branches use the opaque slug; the change must not break resolution
  of a branch already checked out under the old scheme (continue/stop by name still
  works), only new derivations get the readable form.
- **Shared `lead-run.md` Select region (prerequisite ordering).** Phase 3's Select
  rewrite occupies the same section as `260911-refactor-lead-run-ticket-only-delegate-implementer`,
  which removes the ad-hoc Select-skip paragraph and the intro/Spawn ad-hoc rows and
  regenerates the same exact-prose goldens in
  `agents-plugin/tests/test_skill_dispatch_contracts.py` (and the wsflow mirror).
  That ticket lands first (declared as `prerequisite`); implement Phase 3 against the
  post-refactor `lead-run.md` body — do not target the pre-refactor line numbers — and
  regenerate the shared goldens once, against the then-current body.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/implement_resolver.go, agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin/rsrc/ and agents-plugin/skills/ (new ticket-selector playbook), agents-plugin-wsflow/ |
| scope.surface | public-interface | Phase 3's resolver/tool helper for the branch-to-owning-stem lookup is called by the new ticket-selector playbook, adding to or extending the MCP tool surface alongside the existing route.resolve_implement tool (agents-plugin-tool/internal/mcp/server.go#L2989) |
| scope.new_public_symbol | no | ticket's own Constraints: "the fail-closed change and the slug/reverse-map helpers are resolver-local" |
| scope.new_type_contract | yes | Phase 3 adds a new resolver/tool helper (branch to owning-stem) and a new renderable ticket-selector playbook contract |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/implement_resolver_test.go, agents-plugin/tests/test_skill_dispatch_contracts.py, agents-plugin-wsflow/tests/ already exist and are the suites the ticket names for extension |
| complexity.reuse_points | confirmed | parseImplBranchRoot (implement_resolver.go#L831-842) and wskey.Derive (agents-plugin-tool/internal/wskey/wskey.go#L78-86) are the existing derivation primitives Phase 2/3 build on |
| complexity.side_effect_risk | moderate | Phase 1's fail-closed change and Phase 3's Select rewrite both change when a worker spawns vs. stops on live workflow runs |
| risk.correctness | moderate | must preserve the 3 derivation invariants (same-ticket continue, different-ticket stop, never nests) across the slug and reverse-map change (Constraints) |
| risk.fit | low | follows the existing playbook.render / resolver split already used by route.resolve_implement |
| risk.test | low | each phase names concrete test additions (fail-closed injection test, slug/collision tests, reverse-map helper + dispatch-contract + wsflow suite) |
| risk.security_or_contract | moderate | the branch-naming change must not break resolution of already-live opaque-slug impl/* branches (confirmed present, e.g. impl/epic/refound/acid-fried-exile), and the merge-recommendation terminal couples to the not-yet-landed 260911-feat-ws-git-merge-lead-owned-merge-authority |

## Blocked (2026-09-11)

- [ ] Confirm the Phase 3 public query contract after stop (c): whether to
  replace the target-dependent `route.resolve_implement` output plan with an
  exclusive, read-only `tickets.query` branch-ownership mode available before
  ticket selection.

## Phases

### Phase 1: Fail the ahead-count guard closed

Change `aheadOfMergeRootCount` (and the guard that consumes it) so a git error
stops-and-surfaces rather than returning `0`. Independent of the later phases;
land it first as a standalone safety fix. Verify with a resolver test that injects
a git failure and asserts the stop path instead of a silent proceed.

### Phase 2: Readable + deterministic slug

Depends on nothing structurally, but sequenced after Phase 1 in the same file.
Replace the opaque `wskey.Derive(stem, 3)` slug with a readable-prefix +
short-hash-suffix derivation. Keep it a pure function of the stem (determinism
preserved). Confirm the derivation invariants still hold under the new slug
(same-ticket continue, different-ticket stop, no nesting). Verify old-scheme
branches still resolve by name. Add tests for the new derivation and for
collision-safety of the hash suffix.

### Phase 3: Branch → owning-ticket helper and the ticket-selector playbook

Depends on Phase 2 (the reverse map re-derives candidate slugs from stems and
matches the current branch).

- Extend an existing resolver output (`route.resolve_implement`), not a new MCP
  tool: from an `impl/<root>/<slug>` branch, return the owning stem by re-deriving
  candidate slugs from both `ready/` and `.done/` stems and matching. Scanning
  `.done/` too is what makes the closed-but-unmerged case below detectable — a
  closed ticket has left `ready/`, so a `ready/`-only scan could never produce its
  slug.
- Create the renderable `ticket-selector` playbook: on a base branch, current
  ordering (skip Blocked; in-progress > prerequisite > oldest over `ready/`); on an
  `impl/*` branch, select the owning ticket if it has an unfinished phase (continue
  on the same branch, do not consider other candidates), or — owning ticket closed
  and branch unmerged — raise a merge recommendation to the lead (the merge gate in
  `260911-feat-ws-git-merge-lead-owned-merge-authority`) instead of selecting. On an
  `impl/*` branch whose owner the reverse map cannot resolve (e.g. a legacy
  opaque-slug branch predating Phase 2's readable slug), stop-and-report an
  unrecognized owned impl branch rather than falling back to base-branch ordering:
  silently selecting a different ticket would strand the branch, the exact failure
  this ticket removes (the resolver's "different-ticket stop" invariant backstops it
  either way, so no wrong ticket executes). On a `goal/*` branch, preserve the
  routing the current Select carries (`lead-run.md#L26-27`): empty / all-blocked
  outcomes route to the goal-branch terminals. This routing must survive the
  extraction — dropping it would make the goal terminals unreachable and silently
  orphan the goal path that `260911-bug-lead-run-goal-branch-staging-not-created`
  restores.
- Repoint `lead-run`'s Select to render `ticket-selector`; remove the inlined
  ordering rules.
- Mirror into `agents-plugin-wsflow/`.

Verify: resolver helper tests (branch → correct owning stem, and the no-match /
ambiguous cases); the dispatch-contract suite in `agents-plugin/tests/` (lead-run
still satisfies its pinned assertions after the Select edit) and wsflow package
tests; run the full suite touching every edited shipped file.
