---
title: "Ticket write-guardrails accept a non-enumerated category token (e.g. `test`) instead of enforcing the category enum"
related:
  260913-test-mailbox-and-landing-gate-coverage-gaps: source — the closed ticket whose invalid `test` category token surfaced this gap
---

# Ticket write-guardrails accept a non-enumerated category token

## Background

Surfaced by the ship-gate fit review of range `203e555c..develop` (2026-09-13).
The ticket `260913-test-mailbox-and-landing-gate-coverage-gaps` was authored,
`tickets.verify`-passed, committed, moved through `ready/` → `.done/`, and
shipped, all while carrying the category token `test` — which is NOT one of the
six enumerated categories the `ticket-conventions` doc defines (`bug`, `feat`,
`refactor`, `chore`, `research`, `epic`). No other ticket in repo history uses
`test`; it was a one-off authoring slip that nothing mechanical caught.

The lead accepted that specific closed ticket as a documented exception rather
than renaming it (its stem is an immutable absolute reference already cited by
commits e31576b8 / a6bcece1 / d448e421 and the review-ledger `ref`; a
convention-clean rename means drop+recreate, which would break `git log --grep`
history for a cosmetic gain). This ticket tracks the ROOT cause instead: the
guardrails do not enforce the category enum, so the next typo lands the same way.

## Investigation

`tickets.verify` (and the same checks `git.commit` runs before committing a
ticket-touching change) validate stem/status-dir shape, frontmatter fence
integrity, sage posture, and phase/Result heading well-formedness — but the
`<category>` segment of `YYMMDD-<category>-<name>` is not checked against the
enumerated set. Locate the stem/path guardrail (the same one that validates the
status directory and date prefix) in the ticket-tooling source
(`agents-plugin-tool/internal/mcp/` ticket verify path + any shared parser) and
determine where a category-enum assertion belongs so it fires at authoring
(`tickets.create_empty` / `tickets.verify`) rather than at review time.

## Outcome Ledger

### Verified Findings

- A ticket with category token `test` (not in the enumerated set) passed
  `tickets.verify`, committed, and shipped with no mechanical rejection; the
  deviation was caught only by a human/LLM fit reviewer at ship time.

### Proposals

- Add a category-enum check to the ticket write-guardrails: the `<category>`
  segment must be one of `bug|feat|refactor|chore|research|epic`. Fire it in
  `tickets.verify` (so it is a hard guardrail, same as stem/status-dir) and, if
  cheap, in `tickets.create_empty` so a bad category cannot be minted at all.
  Add a test that an out-of-enum category is rejected.

### Open Questions

- Hard-fail vs. soft-warn for a category typo. Stem immutability argues for a
  hard-fail at CREATE time (cheap to fix before the stem is referenced) but a
  soft-warn at verify time for pre-existing tickets, so the guardrail does not
  retroactively block work on already-committed tickets like the one that
  surfaced this. Decide the create-time vs. verify-time posture split.

### Rejected Alternatives

- Renaming the offending closed ticket — rejected (immutable stem already cited
  by four commits + the review ledger; drop+recreate churns history for a
  cosmetic taxonomy gain). The systemic guardrail is the correct fix.
