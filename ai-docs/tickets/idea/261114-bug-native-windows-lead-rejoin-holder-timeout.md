---
title: Native Windows lead rejoin still times out a holder call during long replay
related:
  261113-bug-ws-release-gate-sweep-fixes: completed predecessor that introduced the long-replay regression contract
---

# Native Windows lead rejoin holder timeout

## Background

The native Windows 696 re-pass at exact source
`9854ff350fa1b0eccfb5c71649c5190c405925b9` failed
`TestLeadRejoinDoesNotStallBehindALongReplay` once in three repetitions. The
historical Windows Go report recorded three native passes and explicitly left
the original A6 cause unresolved; those observations do not establish current
combined-source acceptance.

Classification: confirmed native regression-test/acceptance failure; production
causation versus fixture timing/load sensitivity remains unresolved. No baseline
comparison, source repair, timing relaxation or ad hoc retry was performed.

## Reproduction and evidence

Actual native command:

```text
go test -count=3 -timeout 10m ./internal/wsdashboard ./internal/mcp ./internal/wswhiteboard ./internal/wsstate ./internal/wsindex ./internal/wsagent -run <recorded-focused-regexp>
```

Complete regexp/argv and all output: temporary artifact
`windows-696-9854ff350/native/focused.log`, indexed by
`96d19897-01-windows-696-repass-evidence.md`. The corresponding native raw log is
`evidence/focused.log` inside the owned Windows Temp fixture. Machine-specific
artifact locations remain untracked.

The test configures a one-second holder-call timeout, restarts its fake holder,
queues 600 background-task records plus a final zero-task record, rejoins the
lead and sends its next input. Expected: that input is accepted, no holder call
waits out its timeout, and final background-task count/error state is clean.

Actual at `internal/wsagent/lead_runner_test.go:1316`:
`1 holder calls waited out their timeout during the rejoin`.
The failing test took 1.50s; the package failed after 4.011s. The complete focused
command started 2026-11-14T13:48:27Z and exited 1 after 63.047s.

Native environment: ThinkPad P14s Gen 2a, Windows 11 Pro 10.0.26200, Go 1.26.2
windows/amd64, Git 2.38.1.windows.1, Python 3.12.10. Source archive SHA256,
verified on both machines:
`542a67db53840af5f0c3d7bfafefc08d4576dcd5372564485c7bba80c8f35a87`.
The complete Go source build passed first. Test HOME, Go caches, ws cache/config,
Git configuration and temporary paths were isolated; no live owner harness,
installed plugin, shared daemon or user checkout was modified.

## Decisions

- Preserve the prior completed release-gate ticket rather than altering its
  results. Exact-name discovery found no active matching ticket.
- Treat the holder-call timeout as a failed acceptance assertion, not proof of a
  particular deadlock or justification for a larger arbitrary deadline.
- This evidence capture proposes no API/protocol/store change, promotion,
  deployment, merge or source fix.

## Phases

### Phase 1: Diagnose the native long-replay timeout

Under a separately approved isolated Windows scope, trace the timed-out holder
operation and event-loop progress, distinguish a production blocking path from
fixture scheduling sensitivity, and repair only the established cause. Preserve
the requirement that a rejoin must not synchronously wait behind replay records
that only its own loop can consume. Verify native behavior without weakening the
assertion to hide a product failure.
