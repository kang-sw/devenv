---
title: "internal/mcp test suite takes about 5.5 minutes"
---

# internal/mcp test suite takes about 5.5 minutes

## Background

User question on the 2026-11-09 board: test runs seem excessively long, is
there a reason? Observed during the 0.48.13 release gate:
`go test ./internal/mcp/...` took 331.992s locally (Linux, WSL2), the
largest package by far (internal/wsdashboard about 50-70s, the rest
seconds). CI already splits internal/mcp into four windows-latest shards
(`.github/workflows/ws-mcp-ci.yml`, `args: "mcp <n> 4"`), which hides the
cost in CI but not in local verification, where every delegate and worker
that touches internal/mcp pays it, often several times per fix round.

## Idea

Measure before changing anything:

- `go test -json ./internal/mcp/ | ` aggregate per-test elapsed; list the
  slowest tests and what dominates them (spawned processes or daemons,
  fixed sleeps, polling intervals, network or lock timeouts, repeated
  repo setup).
- Then pick fixes by evidence: shared fixtures, injectable clocks or
  shorter test intervals, `t.Parallel()` where tests are independent, or
  moving slow end-to-end cases behind a flag that CI still sets.

Constraint: CI coverage must not drop; a test moved behind a flag still runs
in CI.
