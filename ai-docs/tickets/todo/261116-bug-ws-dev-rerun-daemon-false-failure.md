---
title: "dev.py rerun daemon reports unavailable after successful daemon replacement"
---

# dev.py rerun daemon reports unavailable after successful daemon replacement

## Background

Dogfood finding #5, 2026-11-16. `scripts/dev.py rerun daemon` prints
`error: daemon-start exited 1: {"outcome":"unavailable"}` even though the new
daemon comes up and replaces the old daemon. This false failure makes successful
recovery look unsuccessful and invites unnecessary retries. The observation is
owner-reported; exact process IDs/timings and raw stdout/stderr were not supplied.

`scripts/dev.py:194-226`, `run_daemon_start`, raises immediately on a nonzero
subprocess return before treating the printed report as a result. Investigate
both the daemon's one-shot handoff report and the helper's observation window;
this capture does not establish which side owns the race.

## Reproduction

During separately authorized isolated implementation, run the rerun workflow
against an older daemon. Capture daemon-start exit/stdout/stderr, endpoint and
health before/after, replacement identity and readiness timing. Reproduce the
nonzero `unavailable` report while the intended successor actually serves.
Also retain genuinely failed and wrong-successor controls. Do not restart the
shared daemon to reproduce this ticket during docs capture.

## Candidate fix directions

- Correct one-shot daemon-start handoff/readiness classification if it declares
  unavailable before the replacement has finished becoming observable.
- Reconcile the helper's result with bounded endpoint/health evidence of the
  exact intended successor, rather than treating any answering daemon as success.
- Keep genuine startup errors explicit; do not simply ignore exit status 1.

## Decisions

Compatibility tier: **n/a** for a helper/reporting correction preserving the
existing daemon-start report and health contracts. If those cross-version
contracts must change, assess their producers/consumers and tier before readiness.

## Open questions

- Is the incorrect result emitted by daemon-start, or inferred too early by the
  caller during replacement?
- What bounded successor-identity/readiness evidence should constitute success?
- How should a successful replacement with a failed report differ from a true
  failure or a concurrent unrelated replacement?

## Phases

### Phase 1: Reconcile daemon replacement outcome with observed successor readiness

Trace the handoff and helper error path, settle the success criterion, and fix
the false failure without masking real errors. Verify delayed readiness,
successful replacement, true failure, concurrent replacement, and timeout in
isolated fixtures with exact successor identity. No build, test, executable
probe or daemon replacement is performed by this docs-only capture.
