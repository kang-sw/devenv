---
title: "Pre-existing node test suite failures in claude-mod and agents-plugin-pi"
---

# Pre-existing node test suite failures in claude-mod and agents-plugin-pi

## Background

Two node test suites carry failures that predate recent merges and were never
triaged, so workers cannot treat a full run as a green check.

Evidence (a), `agents-plugin/hooks/claude-mod` and its wsflow mirror, on develop:

- Before the band-tests-cc merge (`cace4ccec`): 698 pass / 78 fail.
- After it: 715 pass / 61 fail.
- The 61 remaining failures are pre-existing and were not triaged.

Evidence (b), `agents-plugin-pi` broader suite:

- The pi-wait-mail-label worker observed 205 failures (merged in `223192e02`).
- Representative Windows ESM/symlink failures reproduce at the merge base.
- The rest are untriaged.

## Idea

Get both suites to a known-green baseline.

- Triage each failing group into one of: environment-only (Windows ESM/symlink,
  Claude Code version drift), stale test, or real bug.
- Fix real bugs; fix or delete stale tests; mark environment-only groups with an
  explicit platform skip or a documented precondition.
- Result: workers can run one passing full check instead of per-file subsets.

Compatibility: test-only and skip-marker changes, T0. Deleting tests is a
"deleting functionality" question, so ask before deleting any test that covers
shipped behavior.

## Open

- Run these suites locally on Windows, or add Node to the remote? w5090 has no
  Node installed today.
