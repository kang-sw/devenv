---
title: "ticket worker reports stop none while later phases remain unfinished"
related:
  260911-feat-ws-git-merge-lead-owned-merge-authority: surfaced when Phase 1 landed while Phase 2 remained ready
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0783af7a1911adf0
sage-review-completeness-reviewed: 0783af7a1911adf0
---

# ticket worker reports stop none while later phases remain unfinished

## Background

During a `lead-run` cycle, the elevated ticket worker implemented and recorded
Phase 1 of `260911-feat-ws-git-merge-lead-owned-merge-authority`, deliberately
left Phase 2 outside that invocation's earliest-unfinished-phase scope, and
returned `stop: none`.

The lead-run contract currently handles `stop: none` as a `close-on-impl`
report, while the fixed worker report schema has no completion field
(`agents-plugin/rsrc/lead-run/lead-run.md#L84-L95`; `agents-plugin/rsrc/worker-stop-protocol.md#L89-L102`). Authoritative ticket state instead remained
`ready`, with Phase 1 carrying a Result and Phase 2 unresolved. This makes the
terminal report ambiguous to its lead caller and can cause session notes or
queue-terminal decisions to be recorded against a ticket that is still active.

## Evidence

- Worker report: `stop: none` and `omitted: Phase 2 remains ready, outside this
  invocation's earliest-unfinished-phase scope`.
- `tickets.query` immediately after the report returned status `ready`,
  `result_present: true`, and unresolved Phase 2.
- The goal branch was clean at `20863462`, so the mismatch was not caused by
  uncommitted ticket state.

## Decisions

- Keep `stop:` as the worker's escalation outcome. `stop: none` means only that
  the worker has no escalation reason; it does not imply that the ticket is
  closed.
- Add this required terminal report field to the shared worker protocol:

  ```text
  completion: phase | ticket | ad_hoc | none
  ```

  - `phase`: the current phase Result was recorded, later phases remain, and
    the ticket remains in `ready/`.
  - `ticket`: every phase is complete and closing the ticket to `.done/`
    succeeded.
  - `ad_hoc`: the ad-hoc work contract completed without a ticket lifecycle.
  - `none`: no phase, ticket, or ad-hoc work unit completed.
- Make lead handling mechanical. A `stop: none` report with `completion: phase`
  merges the implementation branch and leaves the ticket active for a later
  cycle. A `stop: none` report with `completion: ticket` merges the branch and
  permits ticket-closure and epic-terminal handling. Missing, unknown, or
  incompatible completion data is a protocol mismatch and fails closed rather
  than being inferred from ticket paths or queried state.

## Constraints

- Keep ticket lifecycle ownership with the worker that records the phase Result
  and closes the ticket. Do not add lead-side ticket-state inference.
- Define the report field once in the shared worker stop protocol rather than
  duplicating bespoke wording across worker variants.
- Preserve the existing stop-code meanings; this change separates escalation
  state from completion state.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/worker-stop-protocol.md, agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin-wsflow mirrors, and existing package test suites |
| scope.surface | public-interface | worker terminal report is a shared lead-worker protocol shipped in agents-plugin/rsrc/worker-stop-protocol.md |
| scope.new_public_symbol | no | completion is a report field, not an exported symbol |
| scope.new_type_contract | yes | required completion field with four allowed values |
| scope.test_surface | existing | agents-plugin/tests/ and agents-plugin-wsflow/tests/ are named existing package test surfaces |
| complexity.reuse_points | confirmed | shared worker stop protocol and lead-run Handle the report path already exist |
| complexity.side_effect_risk | moderate | lead handling changes after a worker reports completion |
| risk.correctness | high | incorrect completion handling can close or retain an active ticket incorrectly |
| risk.fit | moderate | the shared worker protocol and wsflow mirror must remain aligned |
| risk.test | moderate | supported values and fail-closed cases require protocol and golden coverage |
| risk.security_or_contract | high | a required lead-worker protocol field changes fail-closed lifecycle behavior |

## Phases

### Phase 1: Make worker completion explicit

Extend the shared worker terminal-report contract with the required
`completion:` field and update `lead-run` to consume it without additional
ticket-state judgment. Carry the shared-playbook change through the wsflow
mirror and update the contract or golden tests that protect the shipped
surfaces.

Verify every supported completion value, the phase-versus-ticket lead paths,
and fail-closed handling for missing, unknown, or incompatible values. Run the
focused playbook and mirroring checks plus the relevant plugin package suites.
