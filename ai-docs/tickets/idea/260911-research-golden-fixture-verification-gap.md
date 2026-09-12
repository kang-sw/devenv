---
title: "A ticket that edits a file guarded by an exact-prose golden should surface that golden in fact population / worker verification"
related:
  260911-refactor-lead-delegate-executor-tier-resolution: the trigger — its landed change edited a flagship playbook whose exact prose is pinned by a golden fixture, but neither its Route Facts test_surface named that golden nor its worker ran the suite, so it landed the release-gate branch red
---

# Golden-fixture verification gap

## Background

Ticket `260911-refactor-lead-delegate-executor-tier-resolution` intentionally
changed the body of `agents-plugin/rsrc/lead-delegate/lead-delegate.md`
(sage-reviewed and approved). That body is pinned byte-for-byte by an exact-prose
golden — `agents-plugin/tests/fixtures/lead_delegate_contract.json`'s `delegate`
key, asserted by `test_delegate_and_sibling_exact_prose` in
`agents-plugin/tests/test_skill_dispatch_contracts.py`. The change did not update
the golden, so the flagship dispatch-contract suite went red — and it stayed red
through landing because:

- the ticket's `## Route Facts` recorded `scope.test_surface: existing` without
  naming the golden or its test, so no reviewer or worker was pointed at it; and
- the worker's verification ran the wsflow suite and the Go suites but not
  `agents-plugin/tests`, the flagship suite that pins the very file it edited.

The regression was found only later, by an unrelated worker running the flagship
suite, and reconciled by a follow-up ad-hoc fix (commit `0c884ce4`). This
ticket was the declared release gate, so the gap shipped red on the branch the
release depends on.

## The gap

A file can be guarded by a golden/fixture that lives far from it (a JSON fixture
under `tests/fixtures/`, keyed by content, asserted by a test in another
directory). Editing the file silently invalidates the golden, and nothing in the
current fact-population or worker-verification path reliably connects "I edited
file X" to "golden Y and test Z pin file X's content." Route Facts
`scope.test_surface` is authored from judgment, not from a mechanical
file -> guarding-test map, so it can miss exactly this.

## Directions to weigh

- **Fact population surfaces guarding goldens.** When population grounds a
  ticket that edits a shipped file, mechanically look for fixtures/tests that
  read or embed that file's content and name them in `scope.test_surface`, so
  the worker inherits a concrete "run these" list.
- **Worker verification discovers guarding goldens.** Before reporting, a worker
  that edited a shipped file greps for tests/fixtures referencing that path (or
  embedding its content) and runs them, rather than choosing a suite by
  judgment.
- **A deterministic file -> guarding-test index.** A small map (or a test that
  builds one) from shipped source files to the goldens/tests that pin them, so
  any of the above can be exact rather than heuristic. Weigh maintenance cost
  against the recurrence risk.
- **Scope check:** is this specific to exact-prose goldens of shipped
  playbooks/skills, or the general class of content-embedding fixtures? Decide
  how wide the mechanism should reach.

Land with a concrete recommendation the follow-up implementation ticket can
execute, including whether the fix belongs in fact population (ticket-time),
worker verification (run-time), or a standing test (CI-time) — or a pair of them.
