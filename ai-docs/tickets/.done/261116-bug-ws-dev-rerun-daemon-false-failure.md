---
title: "dev.py rerun daemon reports unavailable after successful daemon replacement"
completed: 2026-11-16
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


## Resolution (2026-11-16)

Absorbed by 261116-bug-ws-exe-launch-elimination under coordinator direction at 2026-11-16 03:22:51Z; this duplicate is closed to prevent a second assignment, not to claim live acceptance. Reviewed code HEAD d286b8eaf9fffc4dbfb2ae1ab86096de8e6d7a05 changes only scripts/dev.py and scripts/test_dev.py. The helper reuses LauncherStartLock through explicit handoff and bounded expected-content-hash REST confirmation; only parsed exit 1 / outcome unavailable permits nonzero reconciliation, with original diagnostics, one starter and one shared lifecycle deadline. Old/no/PID-only health, malformed or unrelated failures and expiry still fail. The daemon-start report/health contracts and package-root/env selection remain unchanged.

Focused fully mocked Python verification: TMP=D:/Temp/exe-dev-repair TEMP=D:/Temp/exe-dev-repair DEV_TEST_TMP=D:/Temp/exe-dev-repair PYTHONDONTWRITEBYTECODE=1 python -B -m unittest scripts.test_dev.DevLifecycleTest -v; all 12 tests passed (0.593s), full output read at D:/Temp/exe-dev-repair/dev-lifecycle-1.txt. Independent focused R1 CLEAN for 04e06b7b5..d286b8eaf, no findings or fixes; conditional fixes-only R2 was not applicable. Full report: C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac@392ca909/review-paths/61cdf1a0-01-exe-dev-repair-r1.md.

These source/test results settle this duplicate's reporting correction only. No live reproduction, executable fixture, Go/ws/test executable, native PID query, build, install, runtime probe, swap or sampling was performed. Actual live acceptance remains coordinator-owned and is not claimed. The primary ticket remains open without a phase Result pending that acceptance.
