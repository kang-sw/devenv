---
title: "proceed contract test pins pre-diet lead-proceed strings (fails on develop)"
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

## Follow-up worth considering

Add the `agents-plugin` python suite to the routine test-review surface for
playbook/rsrc-touching phases, so contract-test drift like this is caught
at review time rather than incidentally.
