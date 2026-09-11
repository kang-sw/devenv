---
title: "Impl-branch guard hardening and branch-aware selection via a ticket-selector playbook"
related:
  260911-research-impl-lifecycle-merge-authority-goal-loop-rehoming: context; the closed design this ticket derives from (research, stays in todo/, not a code prerequisite)
  260911-feat-ws-git-merge-lead-owned-merge-authority: adjacent; an impl branch with no active owner stops selection and hands branch exit or merge inspection to the lead lifecycle owned there
  260911-refactor-lead-run-ticket-only-delegate-implementer: prerequisite; it rewrites the same lead-run.md Select region (removes the ad-hoc Select-skip and the intro/Spawn ad-hoc rows) and regenerates the same exact-prose goldens — land it first and rebase this Select rewrite onto its result
  260910-feat-lead-run-worktree-parallel-route: adjacent; the future fan-out mode's batch selection is a superset of the branch-aware Select added here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: db7ab4e6eb60e376
sage-review-completeness-reviewed: db7ab4e6eb60e376
---

# Impl-branch guard hardening and branch-aware selection

## Background

Diagnosis (recorded in the research ticket) established that the impl-branch
derivation invariants hold. The current `develop` and `epic/refound` copies of
`agents-plugin-tool/internal/mcp/implement_resolver.go` are not byte-identical
(`git diff --quiet develop epic/refound -- agents-plugin-tool/internal/mcp/implement_resolver.go` returns 1).
One narrow residual defect remains, and one downstream capability is missing:

1. A safety guard fails **open**: `aheadOfMergeRootCount` returns `0` on any git
   error, silently disabling the "different ticket on an impl branch stops" guard
   exactly when git misbehaves.
2. `lead-run`'s Select ignores the git branch, so it can pick a different ticket
   while sitting on an in-progress impl branch — a wasted spawn the resolver then
   stops. Select should finish the branch's active owning ticket, or stop with an
   inspection nudge when the impl branch has no active owner instead of spawning.

The existing `wskey.Derive(stem, 3)` slug is already a deterministic function of
the full ticket stem. Active ownership therefore needs candidate re-derivation and
exact matching, not a branch-naming change or reverse decoding.

## Decisions

- **The ahead-count guard fails closed.** A safety guard that self-disables on
  error is backwards. On a git error, the resolver surfaces the unverifiable state
  (stop-and-report) rather than proceeding as if ahead=0. Rejected: leaving it
  fail-open — it defeats the very invariant it guards precisely when state is
  uncertain.
- **Keep the existing deterministic three-word slug contract.** Active ownership
  does not require decoding a stem from the branch suffix: enumerate active ticket
  stems, compute `wskey.Derive(stem, 3)` for each, and exact-match the suffix.
  Rejected: replacing it with a readable-prefix + hash scheme — human legibility is
  unrelated to deterministic ownership matching and would create a new durable
  branch contract plus needless compatibility work.
- **Branch-aware Select is extracted into a renderable `ticket-selector`
  playbook.** The branch-aware logic is shared by lead-run's serial mode, the
  future fan-out mode's batch selection (a superset), and the wsflow mirror; one
  rendered playbook beats inlined duplication and unifies the cognition point.
  Split of concern: `ticket-selector` **orchestrates** (branch detect →
  active-owner status → ordering or stop); MCP-enriched `git.status` computes the
  **deterministic** branch → active-ticket match. Rejected: inlining the rules
  directly into lead-run — duplicated across serial/fan-out/wsflow, and encodes
  non-trivial branch logic in prose.
- **`git.status` carries the impl active-ticket context; no new tool or query mode.**
  `ticket-selector` already needs branch and worktree status first, so the MCP
  handler enriches that one observation when the current branch is `impl/*`.
  It scans only the active inventory (`idea/`, `todo/`, `ready/`), computes the
  existing deterministic three-word slug for each candidate stem, and exact-matches
  the current branch suffix. Text mode
  appends `active ticket: <stem> (<status>)` for one match, or
  `nudge: current branch is impl/* but no active ticket matches; inspect before
  selecting another ticket` for none. Multiple matches append
  `nudge: current branch is impl/* but multiple active tickets match; inspect before
  selecting another ticket`. JSON adds an optional top-level
  `impl_ticket` object with `state: active|missing|ambiguous` and, for `active`,
  `stem`, `path`, and `status`. An inventory/index read failure errors instead of
  reporting `missing`. The combination belongs in the MCP handler; keep
  `internal/wsgit.StatusResult` free of ticket/document dependencies.
- **No archive reverse scan and no inferred completion.** `.done/` and `.dropped/`
  are deliberately excluded because they grow without bound and are unnecessary
  for choosing an active ticket. No active match means only that the impl branch
  has no active owner: selector stops and asks the lead to inspect or exit that
  branch. It does not infer completion, auto-merge, or fall back to base-branch
  ordering. Rejected: scanning `.done/` to distinguish closed-but-unmerged — the
  normal close-to-merge path belongs to lead report handling, while recovery from
  a stranded impl branch is safer as an explicit nudge.
- **The existing implementation resolver stays the final backstop.**
  `route.resolve_implement` remains target-dependent and unchanged; after Select
  chooses a ticket, its existing same-ticket/different-ticket branch guard still
  prevents a wrong implementation from starting.
- **The playbook is the only new named surface.** `ticket-selector` is a new
  shipped playbook surface; `git.status` receives additive output, but no new MCP
  tool or query mode is introduced.

## Constraints

- `agents-plugin-tool/internal/mcp/` — read `ai-docs/manuals/ws-mcp.md`; the
  fail-closed change remains resolver-owned, while the
  workflow-aware `git.status` enrichment is composed in the MCP layer. Do not add
  an `internal/wsdoc` dependency to `internal/wsgit`. Preserve the derivation
  invariants that hold (same-ticket continue; different-ticket stop; never nests).
- `agents-plugin/rsrc/`, `agents-plugin/skills/` — read
  `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/shipped-surface-boundary.md`; the `ticket-selector` playbook and
  the lead-run Select edit ship downstream and must not depend on repo-only facts.
- `agents-plugin-wsflow/` — read `ai-docs/manuals/wsflow-mirroring.md`; mirror the
  Select extraction and the new playbook into the wsflow derivative.
- Do not change impl branch naming. Existing and newly created ticket branches keep
  using `wskey.Derive(stem, 3)`; an impl branch receives the no-active-owner nudge
  only when no active stem re-derives to its suffix, and never falls through to
  another ticket.
- **Shared `lead-run.md` Select region (prerequisite ordering).** Phase 2's Select
  rewrite occupies the same section as `260911-refactor-lead-run-ticket-only-delegate-implementer`,
  which removes the ad-hoc Select-skip paragraph and the intro/Spawn ad-hoc rows and
  regenerates the same exact-prose goldens in
  `agents-plugin/tests/test_skill_dispatch_contracts.py` (and the wsflow mirror).
  That ticket lands first (declared as `prerequisite`); implement Phase 2 against the
  post-refactor `lead-run.md` body — do not target the pre-refactor line numbers — and
  regenerate the shared goldens once, against the then-current body.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/implement_resolver.go, agents-plugin-tool/internal/mcp/server.go and status tests, agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin/rsrc/ and agents-plugin/skills/ (new ticket-selector playbook), agents-plugin-wsflow/ |
| scope.surface | public-interface | Phase 2 additively enriches git.status text/JSON output on impl branches and adds a renderable ticket-selector playbook; existing git.status input and non-impl output stay unchanged |
| scope.new_public_symbol | yes | Phase 2 creates the renderable ticket-selector playbook name; the resolver and git.status enrichment remain internal to the existing MCP tool surface |
| scope.new_type_contract | yes | Phase 2 adds the optional git.status impl_ticket response field/text nudge and a new renderable ticket-selector playbook contract |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/implement_resolver_test.go, server/status tests, agents-plugin/tests/test_skill_dispatch_contracts.py, and agents-plugin-wsflow/tests/ already exist and are the suites the ticket names for extension |
| complexity.reuse_points | confirmed | parseImplBranchRoot (implement_resolver.go#L831-L842) and wskey.Derive (agents-plugin-tool/internal/wskey/wskey.go#L70-L86) are the existing derivation primitives Phase 2 builds on |
| complexity.side_effect_risk | moderate | Phase 1's fail-closed change and Phase 2's status nudge/Select rewrite change when a worker spawns vs. stops on live workflow runs; the new status observation itself is read-only |
| risk.correctness | moderate | must preserve the 3 derivation invariants (same-ticket continue, different-ticket stop, never nests) while reusing the existing slug for active-owner matching |
| risk.fit | low | follows the existing playbook.render / resolver split already used by route.resolve_implement |
| risk.test | moderate | each phase names concrete test additions; Phase 2 must cover text/JSON parity, active/missing/ambiguous/error outcomes, non-impl no-scan behavior, sparse-hidden active tickets, dispatch contracts, and wsflow mirroring |
| risk.security_or_contract | moderate | git.status gains additive workflow output on impl branches; the existing branch-name contract stays unchanged, an unmatched impl branch must receive a fail-closed nudge rather than route to another ticket, and normal close-to-merge behavior stays with lead report handling |

## Phases

### Phase 1: Fail the ahead-count guard closed

Change `aheadOfMergeRootCount` (and the guard that consumes it) so a git error
stops-and-surfaces rather than returning `0`. Independent of the later phases;
land it first as a standalone safety fix. Verify with a resolver test that injects
a git failure and asserts the stop path instead of a silent proceed.

### Phase 2: Active-ticket status and the ticket-selector playbook

Uses the existing deterministic `wskey.Derive(stem, 3)` contract unchanged.

- Enrich the MCP `git.status` response only when HEAD is on an
  `impl/<root>/<slug>` branch. Keep `internal/wsgit.StatusResult` pure; the MCP
  handler scans `idea/`, `todo/`, and `ready/`, re-derives the existing three-word
  slug for each active stem, and exact-matches the current suffix. One match emits the
  active ticket stem/path/status; zero emits the no-active-owner inspection
  nudge; multiple emits an ambiguous result; an inventory or sparse-index
  failure returns an error. Do not scan `.done/` or `.dropped/`.
- Text mode appends exactly one compact line: `active ticket: <stem> (<status>)`
  for a match, the confirmed no-active-owner nudge for zero matches, or
  `nudge: current branch is impl/* but multiple active tickets match; inspect before
  selecting another ticket` for multiple matches. JSON adds
  optional top-level `impl_ticket: {state, stem?, path?, status?}` with
  `state: active|missing|ambiguous`; omit it on non-impl branches. Preserve every
  existing non-impl text and JSON shape.
- Create the renderable `ticket-selector` playbook: on a base branch, current
  ordering (skip Blocked; in-progress > prerequisite > oldest over `ready/`); on an
  `impl/*` branch, consume `git.status`'s active-ticket context before considering
  the ready queue. A `ready` match selects only that ticket (unless its Blocked note
  stops it); an `idea` or `todo` match stops because it is not executable; missing
  or ambiguous stops with the emitted inspection nudge. Never fall back to
  base-branch ordering from any impl result. `route.resolve_implement` remains the
  final same-ticket/different-ticket backstop after selection. On a `goal/*` branch, preserve the
  routing the current Select carries (`lead-run.md#L26-27`): empty / all-blocked
  outcomes route to the goal-branch terminals. This routing must survive the
  extraction — dropping it would make the goal terminals unreachable and silently
  orphan the goal path that `260911-bug-lead-run-goal-branch-staging-not-created`
  restores.
- Repoint `lead-run`'s Select to render `ticket-selector`; remove the inlined
  ordering rules.
- Mirror into `agents-plugin-wsflow/`.

Verify: git.status handler tests for active idea/todo/ready matches, missing,
ambiguous, inventory/index error, sparse-hidden active tickets, and non-impl no-scan
behavior; assert text/JSON parity and unchanged non-impl output. Run the
ahead-count failure and branch-invariant resolver tests, the dispatch-contract
suite in `agents-plugin/tests/`
(lead-run still satisfies its pinned assertions after the Select edit), and wsflow
package tests; run the full suite touching every edited shipped file.
