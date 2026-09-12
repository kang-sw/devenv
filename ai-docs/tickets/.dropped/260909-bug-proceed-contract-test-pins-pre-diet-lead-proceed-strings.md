---
title: "proceed contract test pins pre-diet lead-proceed strings (fails on develop)"
dropped: 2026-09-11
---

## Summary

`agents-plugin/tests/test_skill_dispatch_contracts.py`'s
`test_proceed_keeps_implementation_route_only` asserts several strings that
no longer exist in `agents-plugin/rsrc/lead-proceed/lead-proceed.md`. The
test fails on `develop` today (confirmed 2026-09-09), independent of the
`260908-bug-shipped-surfaces-carry-devenv-only-content` work — it was
surfaced while running the full `agents-plugin` python suite during that
ticket's Phase 3, but reproduces at `develop` with all leak-ticket commits
absent.

## Evidence

- `git show develop:agents-plugin/rsrc/lead-proceed/lead-proceed.md | grep -c
  "Route only; do not implement or plan here."` → 0 (string absent on
  develop).
- The same string is still asserted by the test on develop (`grep -c` → 1).
- Running the test on a fresh `develop` worktree → `FAILED (failures=1)` at
  the first stale assertion.

## Root cause (suspected)

The `refactor(proceed)` diet passes (`9b329c5c` rename direct-execution
judge to free-form, `9d8a7b6b` close free-form judge precedence gaps)
rewrote `lead-proceed.md` into its current leaner form but did not update
`test_proceed_keeps_implementation_route_only`, which still pins the
pre-diet wording. The `agents-plugin` python suite is not part of the
default review gate (reviews run `go test` + the wsflow python bundle), so
the drift went unnoticed.

## Stale assertions to reconcile

At minimum these no longer match the current body:
- `"Route only; do not implement or plan here."`
- `"Always route code-editing work through \`lead-implement\`"`
- `"Follow \`Next:\` from \`route.resolve_proceed\` exactly"`

The fix is to re-derive the contract assertions from the current
`lead-proceed.md` structure (invariants, `judge: free-form`, the
`route.resolve_proceed` call shape, the `scope_blocked=*` values, and the
negative "no verdict rendering" assertions) — or delete the ones the diet
made obsolete.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/tests/test_skill_dispatch_contracts.py (exists), agents-plugin/rsrc/lead-proceed/lead-proceed.md (absent at current HEAD, see risk.fit) |
| scope.surface | internal | test-file-only edit; no exported symbol changes |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin/tests/test_skill_dispatch_contracts.py; at current HEAD the file no longer contains test_proceed_keeps_implementation_route_only or the three named assertions (read L1-47); only a RETIRED_SKILL_NAMES entry "lead-proceed" remains (L35) |
| complexity.reuse_points | not-applicable | none named |
| complexity.side_effect_risk | low | isolated python unittest assertion edit; no runtime or behavior code touched |
| risk.correctness | high | Phase 1's target does not exist at current HEAD: no agents-plugin/rsrc or agents-plugin/skills path matches lead-proceed (find returned none), and test_skill_dispatch_contracts.py L1-47 has no test_proceed_keeps_implementation_route_only or the three pinned strings; ai-docs/tickets/.done/260909-refactor-lead-surface-collapse-worker-stop-protocol.md L513-516 and L832-838 records the file and test were deleted, not reconciled, when lead-proceed retired |
| risk.fit | high | ai-docs/tickets/.done/260909-refactor-lead-surface-collapse-worker-stop-protocol.md L836-838 states this bug ticket still stands in idea/ describing a test that no longer exists and that dropping it is a ticket-inventory decision for the lead; python3 -m unittest discover -s agents-plugin/tests run here reports 56 tests OK with zero failures, independently confirming nothing on this tree reproduces the claimed failure |
| risk.test | high | the full agents-plugin python suite already passes at current HEAD (56 tests OK, verified directly) with none of Phase 1's described edits applied, so Phase 1's stated exit check ("verify the full suite passes") is trivially true today for reasons unrelated to any fix; the failure this ticket describes only reproduces on develop (see Evidence), so which branch any remaining work targets is unresolved |
| risk.security_or_contract | low | test-only surface; no schema, API, or security boundary touched |

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Phases

### Phase 1: Re-derive the proceed contract assertions from the current body

Reconcile `test_proceed_keeps_implementation_route_only` in
`agents-plugin/tests/test_skill_dispatch_contracts.py` with the current
`agents-plugin/rsrc/lead-proceed/lead-proceed.md`. Drop the pre-diet strings
the body no longer contains (at minimum `"Route only; do not implement or
plan here."`, ``"Always route code-editing work through `lead-implement`"``,
and ``"Follow `Next:` from `route.resolve_proceed` exactly"``) and re-derive
the surviving assertions from the current structure — the invariants,
`judge: free-form`, the `route.resolve_proceed` call shape, the
`scope_blocked=*` values, and the negative "no verdict rendering" assertion —
so the contract pins what the diet actually ships. Verify the full
`agents-plugin` python suite passes on the branch. Keep the test a contract
check: assert current behavior, do not re-loosen it into a tautology.

## Follow-up worth considering

Add the `agents-plugin` python suite to the routine test-review surface for
playbook/rsrc-touching phases, so contract-test drift like this is caught
at review time rather than incidentally. Out of this ticket's scope; captured
as a separate concern, not part of Phase 1.


## Resolution (2026-09-11)

Moot on epic/refound. The fix target — agents-plugin/rsrc/lead-proceed/lead-proceed.md and the pinned test test_proceed_keeps_implementation_route_only in agents-plugin/tests/test_skill_dispatch_contracts.py — was deleted entirely by the lead-surface collapse (ce5f30ef, ticket 260909-refactor-lead-surface-collapse-worker-stop-protocol), whose own text left dropping this ticket as an inventory decision for the lead. The full agents-plugin python suite passes (56 OK) at current HEAD. The develop-side failure this ticket described is resolved by the epic/refound -> develop merge itself, which deletes lead-proceed and the stale test on develop. Nothing left to fix.
