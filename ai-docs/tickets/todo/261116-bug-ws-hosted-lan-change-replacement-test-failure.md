---
title: "Linux w5090에서 hosted LAN 변경 데몬 교체 테스트가 반복 실패하는 문제"
related:
  261116-bug-ws-exe-launch-elimination: "separate lead-owned process-launch effort; coordinate fixture assumptions, do not duplicate its scope"
---

# Hosted LAN-change daemon replacement test fails repeatedly on Linux w5090

## Background

Pre-existing failure reported on 2026-11-16: package `cmd/ws-mcp`,
`TestHostedConnectionSeeingALANChangeReplacesTheDaemon`, failed **3/3** on Linux
`w5090` at `develop` commit `deb8289d2`. This is baseline evidence, not a failure
introduced by today's documentation or proof of a new regression. The assertion
text and raw logs were not supplied; recover them rather than inventing a cause.

Test anchor:
`agents-plugin-tool/cmd/ws-mcp/daemon_proxy_test.go:386`,
`TestHostedConnectionSeeingALANChangeReplacesTheDaemon`. Several closed tickets
record the test as a known exclusion; duplicate discovery found no active ticket
specifically owning this repeated baseline failure.

## Reproduction

During separately authorized implementation on Linux w5090, use a disposable
checkout pinned to `deb8289d2` and run the exact named package test three times
with isolated HOME/ws cache/config and process/port cleanup. Retain each failed
assertion, full output, timing, environment and process/replacement identities.
Compare a candidate and a supported non-Linux fixture if needed. These are future
steps only: no go command, build, test binary or daemon is run by this capture.

## Candidate fix directions

- Diagnose the LAN-change replacement/reconnect sequence and readiness contract.
- Replace brittle ordering/time budgets with observable fixture synchronization
  if evidence shows a test-only race.
- Correct actual replacement behavior if the test exposes a product defect;
  do not hide a real failure with an unconditional skip or timeout increase.

## Decisions

Compatibility tier: **n/a** for a test synchronization correction preserving
protocol/store behavior. If diagnosis requires a replacement protocol or hello
change, classify the concrete old/new paths before readiness.

## Open questions

- Which assertion fails in each of the three runs, and does it fail in isolation?
- Is the defect in replacement/reconnect behavior, fixture environment, or timing?
- How does the separate exe-launch elimination change the fixture's legitimate
  process assumptions?

## Phases

### Phase 1: Diagnose and repair the repeated LAN-change replacement failure

Recover the baseline logs, isolate the cause, and implement the smallest
product or fixture fix justified by evidence. Future acceptance repeats the
named test, covers LAN-on/off handoff and reconnect identity, and records process
cleanup without suppressing legitimate failures. Keep unrelated suite failures
separate; this ticket capture runs no tests or executable probes.
