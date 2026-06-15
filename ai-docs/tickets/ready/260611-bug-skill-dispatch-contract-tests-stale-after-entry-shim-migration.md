---
title: Python skill-dispatch contract tests are stale after the M2 entry-skill-shim migration
related:
  260609-refactor-ws-skill-text-playbook-conversion: M2 moved procedure text into playbook.print rsrc content and reduced entry skills to thin shims; the Python contract tests were not updated to match
  260609-refactor-ws-spawn-runtime-deletion-session-auth: surfaced during Phase 3 verification (the failures are disjoint from the Go/Rust Phase 3 surface and predate the whole M3 stack)
  260610-bug-wsflow-runtime-contract-playbook-tools-drift: sibling migration-drift concern (runtime contract, different surface)
spec:
  - 260610-entry-skill-surface-reduction
  - 260513-wsflow-agentless-skill-surface
---

# Python skill-dispatch contract tests are stale after the M2 entry-skill-shim migration

## Background

While verifying M3 Phase 3 (`go`/`cargo` surface), the two shipped Python test
suites were run and are RED:

- `python3 -m unittest discover agents-plugin/tests` — 1 failure + 2 errors in
  `tests/test_skill_dispatch_contracts.py`.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — 1 failure in
  `tests/test_wsflow_skill_bundle.py`.

Confirmed pre-existing: the failures reproduce identically at the epic base
`c917c9f0` (Phase 1 merged, before the 2a/2b/2c stack), and the Phase 3 diff
touches only `agents-plugin-tool/` (Go) and `ws-dashboard/` (Rust) — disjoint
from the failing skill/test surface.

## Symptoms

1. `test_skill_dispatch_contracts.py` reads
   `agents-plugin/skills/lead-workflow-manual/SKILL.md`, which does NOT exist in
   `agents-plugin/skills/` (only `agents-plugin-wsflow/skills/` has a
   `lead-workflow-manual`). → `FileNotFoundError`.
2. The same test asserts `lead-proceed/SKILL.md` contains
   `` "Always route code-editing work through `ws:lead-implement`" `` — but M2
   (`bd2e4be4`, "move 9 internal procedures off the entry surface to
   playbook.print rsrc content") reduced `lead-proceed` to a thin
   `ws/playbook.print(name: "lead-proceed")` shim; that routing text now lives in
   the rsrc playbook body, not the SKILL.md. → assertion fails.
3. `test_wsflow_skill_bundle.py::test_skill_files_do_not_reference_full_ws_agent_surface`
   flags `skills/lead-workflow-manual/SKILL.md: full ws dotted namespace` in the
   wsflow bundle — a real content offense (wsflow skill text references a full-ws
   dotted tool name).

## Why it matters

These are SHIPPED package test suites; a red baseline hides real regressions and
makes every future implementer's verification step ambiguous (a Phase-3
implementer correctly diagnosed them as pre-existing only by stash-and-rerun).
The contract intent is still valid — entry skills should route correctly and
wsflow text should avoid full-ws namespaces — but the assertions encode the
pre-migration skill shape.

## Possible follow-ups

- Update `test_skill_dispatch_contracts.py` to the post-M2 shipped surface:
  assert the routing/contract text where it now lives (the rsrc playbook body via
  `playbook.print`, or the shim's actual content), and stop requiring a
  `lead-workflow-manual` SKILL.md under `agents-plugin/skills/` if that procedure
  is intentionally wsflow-only / playbook-only now. Confirm the intended home of
  `lead-workflow-manual` first (agents-plugin shim vs wsflow-only vs playbook).
- Fix the genuine wsflow offense: rewrite the full-ws dotted namespace reference
  in `agents-plugin-wsflow/skills/lead-workflow-manual/SKILL.md` to the
  namespace-neutral / wsflow form the bundle test requires.
- Decide whether these belong with `260609-refactor-ws-skill-text-playbook-conversion`
  (M2 closeout debt) or a small standalone test-realignment ticket.

## Notes

- Distinct from `260610-bug-wsflow-runtime-contract-playbook-tools-drift`
  (runtime capability contract drift). This ticket is purely the Python
  skill-text/dispatch contract tests lagging the entry-shim migration.

## Phases

### Phase 1: Realign skill-dispatch tests with playbook-backed entry shims

Update the shipped Python skill-dispatch contract tests so they assert the
post-M2 source of truth: thin entry skills route through `playbook.print`, while
procedure contracts live under `agents-plugin/rsrc/` playbooks. Do not recreate
removed full skill files under `agents-plugin/skills/`.

Also fix the wsflow workflow-manual namespace offense that the existing
wsflow package test reports, using the canonical wsflow-facing or
namespace-neutral form rather than relaxing the forbidden-reference check.

Verification boundary:

- `python3 -m unittest discover agents-plugin/tests` exits 0.
- `python3 -m unittest discover agents-plugin-wsflow/tests` no longer fails on
  `skills/lead-workflow-manual/SKILL.md: full ws dotted namespace`.
- The tests continue to verify that proceed remains route-only and implement
  owns implementation execution.
