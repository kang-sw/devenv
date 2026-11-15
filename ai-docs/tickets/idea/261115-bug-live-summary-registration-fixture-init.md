---
title: Initialize live-summary fixture registration before its first read
related:
  261114-feat-ws-live-reasoning-summary: scoped test-fixture follow-up
---

# Initialize live-summary fixture registration before its first read

## Background

VERIFIED: both named intake tests fail with missing `registration.json` before
behavior assertions on F `f19cab5241ee3c8ad4b2c9db5db6c122067a7707` and receiver
R `5d018785c8caddc237cf24bb5229c871aac8783b`. All four cells actually ran once;
R is not F's parent. Evidence: `/tmp/ws-live-summary-675-regdiag-01/result.json`
and `logs/{07-f19-freeze,12-f19-owner,21-5d-freeze,26-5d-owner}.stdout` there.

INFERRED: a preexisting fixture initialization gap. `newSummaryIntakeFixture`
reads the row before its own write; its direct intake helper bypasses keyed
registration (`summary_intake_test.go:37–78`, `server.go:781–783`). This is not
proof of production correctness, flakiness, or T0 compatibility.

## Constraints

- Change only the fixture in `agents-plugin-tool/internal/mcp/summary_intake_test.go`;
  preserve all behavior assertions. Product changes require a new disposition.
- Preserve the seven protected preparation artifacts and all prior evidence.
  Reuse the two audited F failures as RED; do not repeat the R comparison.
- The recorded names are `TestSummaryIntakeFreezesRealEndBeforeAcceptanceAndRetriesOnlyCandidate`
  and `TestSummaryIntakeRetiredConnectionAndHostedTakeoverCannotWrite` (R declaration
  receipt: `logs/01-R-static-testnames.stdout`). The earlier `TestLiveSummaryIntake…`
  spelling was a lead relay error, not the owner grant. No rename is proposed.

## Phases

### Phase 1: Initialize the fixture row before reading it

Propose invoking the existing synchronous registration hook before the first
read, rather than polling or adding another serve-loop lifecycle. At the lead's
fix-batch gate, run only the two recorded tests once each in the private offline
environment, then obtain independent lite review. Record the authorized batch's
actual outcome in a dated checkpoint and keep this ticket open.

#### Checkpoint - 2026-11-15

Applied only the approved comment and `dashboardTouch` call before the fixture's
first registration read. Product code and behavior assertions are unchanged.
The `TestLiveSummaryIntake…` spelling was a lead relay error, not owner intent.

Reused audited F RED outputs `07` and `12` from the diagnostic matrix. Both exact
recorded tests then ran once and passed in the authorized offline fix batch:
`/tmp/ws-live-summary-675-fixture-fix-01/logs/09-GREEN-freeze.stdout` and
`logs/11-GREEN-owner.stdout` (each exit 0, actual named RUN/PASS). List-only
formatting was clean. The batch took 33.294734333 seconds; all 19 started handles
were reaped with exit 0, no signals or unknown-survivor observations. The 1,200
source hashes, seven protected artifacts, frozen draft and 296 prior evidence
files matched START/END (`result.json`, `logs/lifecycle.jsonl:14–47`).

Tested fixture SHA-256:
`d44aca7f04fd6d5c82257e7fe830f3894c43ffe215237d83abb3e0686baf1497`.
This checkpoint was appended after the frozen batch. A fresh one-pass
independent lite review was clean, with no Critical, Important or Minor findings:
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@721a59b4/review-paths/dc640067-01-261115-live-summary-fixture-fix-lite.md`.
Its scope was the fixture fix and relevant retained integration, not full-feature
or T0 acceptance. The lead owns integration and separately gated post-sync checks.
