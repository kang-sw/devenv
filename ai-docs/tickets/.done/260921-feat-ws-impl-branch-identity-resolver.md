---
title: "Impl-branch identity: single ticket↔branch resolver authority and lead-vouched override"
related:
  260915-bug-ws-route-resolve-implement-branch-handling-random-codename: superseded — its random-codename premise is stale; this ticket absorbs the real seam
  260915-bug-route-implement-branch-identity: superseded — its nonstandard-suffix recognition becomes Phase 2
  260910-feat-lead-run-worktree-parallel-route: prior art — its forward-note names the acquire/derive gap this ticket closes
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: fa58caa8680682c2
sage-review-completeness-reviewed: fa58caa8680682c2
completed: 2026-09-21
---

# Impl-branch identity: single ticket↔branch resolver authority and lead-vouched override

## Background

In a 2026-09-15 parallel `ws:lead-run` batch, three sibling workers diverged
three ways (`continue` / `rename` / `stop`) on impl branches the lead had
pre-provisioned via `worktree.acquire`. The 2026-09-15 dogfood ticket attributed
this to a "random codename" branch convention; that diagnosis is wrong against
current source and this ticket supersedes it.

The real mechanism is a seam between two tools that do not share a
branch-name authority:

- `route.resolve_implement` derives the impl branch **deterministically** from
  the ticket stem — `wskey.Derive(ticket_stem, 3)` (a SHA-256-seeded slug, not
  random), yielding `impl/<merge-root>/<slug>`. Its continue/rename/stop plan
  compares the observed current branch against this freshly re-derived name by
  string equality.
- `worktree.acquire` is **ticket-unaware**: it takes a raw `target_branch`
  string and provisions it verbatim, with no stem derivation, and its response
  does not return the branch it created.

Because the lead is an LLM and no tool exposes the derivation to it, the lead
cannot reproduce the SHA-256 slug ahead of time. It supplies a guessed branch
name to `worktree.acquire`; the worker later recomputes the canonical name
inside `route.resolve_implement`, finds a mismatch, and returns `rename` — or
`stop` when the guessed branch already carries unmerged commits. The
`260910` parallel-route ticket already recorded this as a forward note ("the
lead cannot compute `wskey.Derive` itself … left as a forward note for a future
acquire helper that returns the derived name").

The same derivation is also **duplicated** across the tree: the forward
stem→slug derive lives in the implement resolver, and an independent reverse
"which ticket owns this branch suffix" match (enumerate candidate stems and
compare `wskey.Derive(stem, 3)` against the suffix) lives in `git.status`.

Separately, a legitimate already-landed commit on a **manually named**
nonstandard branch (e.g. `widget-context-port`) is refused with a spurious
`stop` purely because the branch suffix does not match the derived name, even
when the commit's ticket metadata and changed paths clearly implement the
target ticket's phase.

## Decisions

- **Single resolver as the derivation authority (chosen).** Introduce a
  ticket↔branch resolver that owns both directions of the mapping, and route
  the lead, the implement resolver, and `git.status` through it.
  - Rejected — *add `ticket_stem` to `worktree.acquire`*: keeps a clean generic
    worktree primitive ticket-unaware, and does not consolidate the mapping that
    is already duplicated in `git.status`. The resolver approach absorbs that
    duplication as a first-class concern.
  - Rejected — *name-preview mode on `route.resolve_implement`*: that tool needs
    a checked-out-branch context to emit a continue/rename/stop plan; using it
    only to obtain a name overloads it. A thin dedicated resolver is cleaner.
- **Reverse direction is enumerate-and-match, not inversion.** `wskey.Derive`
  is a SHA-256-seeded hash and cannot be inverted; `branch → ticket` must take a
  candidate stem set and forward-derive each to match the observed suffix,
  exactly as `git.status` does today. The resolver's honest shape is therefore
  forward `ticket_stem → branch` (pure) and reverse
  `branch_suffix + candidate_stems → matched_stem`.
- **`worktree.acquire` is unchanged.** It stays ticket-unaware; the lead
  resolves the canonical branch first and passes it as `target_branch`
  verbatim. Branch-name authority lives in the resolver, not in acquire.
- **Phase 2 honors 260825's SAFETY/IDENTITY split (chosen).** The companion
  nonstandard-branch stop is fixed by repairing the lead-side identity override,
  **not** by teaching the resolver to read commit content. `260825`
  (`.done/`) explicitly rejected mechanical commit-content parsing inside the
  resolver on a risk asymmetry: a false positive (over-block) is cheap for the
  lead to recover, a false negative (silently adopting another ticket's unmerged
  work) is an expensive branch-mixing event. The resolver keeps answering only
  the *safety* question with a dumb reliable signal; *identity* ("which ticket
  owns this unmerged work?") stays lead judgment.
  - Rejected — *mechanical commit-content recognition inside the resolver*
    (metadata + changed-path matching): this is exactly the design `260825`
    rejected; it re-couples correctness to commit hygiene and frontmatter format
    and flips the safety bias.
  - The lead vouches identity; once vouched, the resolver performs its normal
    benign-mismatch action — rename to the derived canonical name and continue.
- **Non-goal: `merge_confirm` batch-uniformity.** The 2026-09-15 observation
  that `merge_confirm` was `skip` for two siblings and `ask` for a third is not
  a bug. `merge_confirm` is a per-ticket route-policy fact read verbatim
  (defaulting to the conservative `ask` when absent), not a batch-global
  tooling signal, so sibling variance is by design. This ticket deliberately
  does not touch it. (If a specific ticket's `ask` was a silently missing
  policy fact rather than a declared one, that is a fact-population concern for
  a separate ticket, not this one.)

## Constraints

This ticket edits native MCP tooling source (`agents-plugin-tool/internal/mcp/`)
and a shipped `lead-run` playbook (`agents-plugin/rsrc/lead-run/`). Each matching
Implementation Conventions manual below is read before editing a file under its
declared paths:

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Prior Decisions

- 260921-bug-route-implement-stale-branch-plan (2026-09-21, section "Resolution"): "route.resolve_implement selects the authoritative implementation slug; the dispatched branch is provisional. The observed rename was expected resolver behavior, not a defect." — bearing: constrains
- 260825-feat-impl-branch-single-ticket-scope-merge-timing (2026-08-25, commit e2b9605d): "Separate SAFETY (deterministic resolver) from IDENTITY (which ticket owns the work = model judgment, routed to the lead); rejected mechanical commit-content parsing inside the resolver." — bearing: constrains
- 260911-feat-impl-derivation-hardening-branch-aware-select (2026-09-11, section "Decisions"): "Keep the existing deterministic three-word slug contract: enumerate active ticket stems, compute wskey.Derive(stem, 3) for each, and exact-match the suffix; rejected reverse decoding." — bearing: supports
- 260910-feat-lead-run-worktree-parallel-route (2026-09-15, section "Result"): "target_branch points at the ticket's canonical impl/<parent>/<slug> so route.resolve_implement returns continue; the lead cannot compute wskey.Derive itself." — bearing: supports
- 260827-bug-impl-branch-stem-word-key (2026-08-27, section "Decisions"): "Fix at the resolver, not by convention: derive the impl-branch stem deterministically from ticket identity so it is identical across every phase of one ticket." — bearing: supports
- 260523-bug-implement-merge-target-discovery (2026-08-10, section "Result"): "Encoded impl/<merge-root>/<stem> in the implement branch resolver and drained the full coupling set." — bearing: supports
- 260707-feat-impl-branch-convention-autodelete (2026-07-07, section "Decisions"): "Rename the branch-creation convention from implement/<scope-slug> to impl/<stem>, with <stem> capped at a maximum of 15 characters." — bearing: supports
- c20c033e (2026-08-07, commit): "git ref D/F conflict between a legacy single-segment impl/<stem> and a new impl/<stem>/... requires the resolver to detect and clean up or warn on create; left as todo, not promoted." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/implement_resolver.go, agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/wskey/wskey.go, agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin-wsflow/rsrc/lead-run/lead-run.md |
| scope.surface | public-interface | new MCP tool exposed to the lead for forward ticket_stem to branch derivation, per Phase 1 |
| scope.new_public_symbol | yes | new MCP tool for ticket_stem to branch derivation; ticket does not name it |
| scope.new_type_contract | yes | new resolver tool request or response shape (forward ticket_stem in, branch out; reverse branch_suffix plus candidate_stems in, matched_stem out), not yet specified by the ticket |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/implement_resolver_test.go, agents-plugin-tool/internal/mcp/server_test.go, agents-plugin-tool/internal/wskey/wskey_test.go |
| complexity.reuse_points | confirmed | wskey.Derive reused, agents-plugin-tool/internal/wskey/wskey.go#L72-L86 |
| complexity.side_effect_risk | moderate | consolidates two existing derivation call sites, agents-plugin-tool/internal/mcp/implement_resolver.go#L741-L745 and agents-plugin-tool/internal/mcp/server.go#L2462-L2467, into one authority as a behavior-preserving refactor |
| risk.correctness | moderate | Phase 2 must add lead-vouch consumption to the currently unconditional AheadOfMergeRoot stop, agents-plugin-tool/internal/mcp/implement_resolver.go#L985-L992, without weakening the no-vouch reject case |
| risk.fit | moderate | Phase 2 explicitly rejects 260825's commit-content parsing design; no override-consumption code exists yet — target.ticket_stem is interpolated only into the stop Reason string, never checked, agents-plugin-tool/internal/mcp/implement_resolver.go#L985-L992,1038-1042 |
| risk.test | moderate | existing test files cover the touched sites but the new MCP tool and the lead-run playbook edit need new coverage, including the wsflow mirror per wsflow-mirroring.md |
| risk.security_or_contract | moderate | adds a new MCP tool contract to the shipped surface, shipped-surface-boundary.md and ws-mcp.md apply, and edits a shipped lead-run playbook |

## Phases

### Phase 1: Ticket↔branch resolver as the single derivation authority

Introduce a resolver that owns the ticket↔branch mapping in both directions and
make every current caller route through it.

- **Forward** `ticket_stem → impl/<merge-root>/<slug>` using the existing
  `wskey.Derive(ticket_stem, 3)` derivation, exposed to the lead as an MCP tool
  so the lead can obtain the canonical branch name before provisioning. The
  `<merge-root>` component must be obtained the same way the resolver computes it
  (it derives from the run's base/current branch, `implement_resolver.go:900`),
  so the lead-provisioned name matches what the worker's resolver later
  re-derives in the worktree; a merge-root mismatch alone reproduces the very
  rename/stop this closes, so the forward tool takes (or identically derives) the
  base rather than assuming one.
- **Reverse** `branch_suffix + candidate_stems → matched_stem` via
  enumerate-and-match (no hash inversion).
- **Consolidate** the two existing derivation sites — the implement resolver's
  forward derive and `git.status`'s reverse match — onto this single authority,
  removing the duplicated logic.
- **`lead-run` parallel route**: call the resolver before `worktree.acquire`
  and pass the returned canonical branch as `target_branch`. `worktree.acquire`
  is not modified.
- Keep one branch-name authority: the lead does not hand-author impl branch
  names; it uses the resolver output.

Verification expectations:

- Resolving the same ticket stem is reproducible (identical branch every call).
- A worktree the lead provisions on the resolver-returned branch yields
  `continue` from `route.resolve_implement` — no `rename`, no `stop` — in the
  pre-provisioned parallel-route case.
- The implement resolver and `git.status` produce identical results to today
  after being routed through the shared authority (behavior-preserving refactor).

### Result (065d86c2) - 2026-09-21

Implemented the shared ticket suffix authority and enumerate-and-match reverse
lookup in `impl_identity.go`; routing and active-ticket status now consume it.
The new `git.resolve_impl_branch` MCP tool takes the exact base and ticket stem
and returns the canonical branch before provisioning. `worktree.acquire` stays
unchanged. Lead-run and wsflow/Pi mirrors resolve before acquisition; runtime
inventories include the new tool.

Integration coverage proves repeatability, nested-goal merge roots, provisioned
worktree routing to `continue`, required-input validation, and compact default
output. Round-1 test review requested request-level boundary coverage; the fix
also exposed and corrected whitespace-padded `HEAD` validation in `b0f925b`.
Fit review was clean; test re-review was clean. Full Go suite, MCP smoke, and
13 wsflow package tests passed at `b0f925b` (leaf read complete output).

### Phase 2: Restore the lead's identity-override path for a stuck nonstandard branch

Depends on Phase 1's shared authority.

Honor the SAFETY/IDENTITY separation from `260825` (`.done/`): the deterministic
resolver keeps answering only the safety question and must **not** read commit
content to judge which ticket owns unmerged work. When a branch name mismatches
the derived canonical name **and** carries unmerged commits ahead of the merge
root, the resolver still emits its conservative safety `stop`.

The defect this phase fixes is that the lead's identity-override does not
actually clear that stop: in the companion case a lead re-invoked the route with
evidence and got the identical `stop`, blocking verify / review / record /
close. Make a lead-supplied identity assertion ("the unmerged work on this
branch belongs to ticket T") a consumable input that lifts the safety block,
after which the resolver applies its normal benign-name-mismatch action —
**rename the branch to the derived canonical name and continue**.

- **Judgment stays with the lead, never the worker, never the resolver.** On the
  `stop` the worker escalates to the lead; it does not read the commit and
  self-authorize. The lead resolves identity from its own cross-ticket context
  (or a dispatched explore) and supplies the override. This mirrors Phase 1's
  philosophy: the tool stays mechanical, the lead owns judgment.
- **Repair the existing override channel.** `260825` describes a "lead context"
  relation-detection layer in which the lead supplies the relation as an explicit
  fact/policy so the resolver reaches the right verdict without a stop, and
  `target.ticket_stem` is already a route input. But no resolver code consumes
  such an override today: the branch-mismatch-with-unmerged-work `stop` is
  unconditional and `target.ticket_stem` is only interpolated into the stop's
  reason string, never used to change the action — which is why the companion's
  re-invoke-with-evidence returned the identical `stop`. Phase 2 adds that
  consumption. The vouch must be a **distinct explicit signal**, not merely the
  presence of `target.ticket_stem`: that field is present on every ticket-targeted
  run, so keying the override off its presence would lift the safety stop for
  unrelated tickets and defeat the guard. The exact vouch input shape is a
  verification point for the worker.
- **Post-override action = rename-to-canonical + continue.** For a branch
  already merged/shared where a rename would be unsafe, continue in place
  instead; the worker confirms this boundary against actual tool behavior.

Verification expectations:

- Accepted: a lead-vouched nonstandard branch carrying a commit that
  legitimately implements the target proceeds (rename-to-canonical + continue,
  or in-place continue when a rename is unsafe).
- Rejected: without a lead vouch, an unrelated nonstandard branch with unmerged
  work still `stop`s (safety preserved).
- The worker never self-authorizes past the safety `stop` by reading commit
  content.

### Result (065d86c2) - 2026-09-21

Added distinct `policy.branch.identity_vouch` with exact `branch` and
`ticket_stem` strings. A matching lead assertion lifts only the unmerged-work
identity stop; no commit-content inference occurs. Without a matching vouch,
the safety stop remains. Target-name collisions remain blocking. A vouched
branch normally renames to canonical; existing tracking state or disabled
rename instead continues in place. Tracking observations are the existing
conservative signal for a shared branch, not a new remote-publication probe.

Correctness review found the worker's closed stop list lacked a valid encoding
for identity escalation. `b0f925b` explicitly classifies it as stop (b), aligns
lead-run handling, and keeps worker self-vouching forbidden across mirrors.
Correctness re-review and test re-review are clean; no unresolved findings.

Verification at `b0f925b`: targeted identity/request/playbook tests, `go test
./...`, `scripts/smoke-ws-mcp.sh ..`, manifest/mirror regeneration tests, and
`python3 -m unittest discover agents-plugin-wsflow/tests` all passed. Tests
cover no-vouch refusal, wrong branch/ticket assertions, accepted rename,
tracking continuation, malformed assertions, and canonical-name collisions.

Dogfood follow-up: reviewer startup initially failed after runtime inventory
changed. Rebuilding only the ignored local Pi runtime with exact release
version restored startup; blank-stderr error reporting was captured separately
as `260921-bug-pi-child-bootstrap-error-loses-diagnostic` in `9dc3b078`.
No merge or push performed.
