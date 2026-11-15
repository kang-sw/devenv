---
title: Native Windows ticket-index timeout regression test reports origin unreachable instead of not refreshed
related:
  260924-bug-ticket-index-read-timeout-below-ssh-roundtrip: prior timeout-versus-transport-failure contract
  260926-bug-wsindex-hang-transport-stop-wait-fails-on-local-windows-host: historical dropped transport-helper case; not edited or reopened
---

# Native Windows ticket read timeout reports unreachable

## Background

The native Windows 696 re-pass of exact source
`9854ff350fa1b0eccfb5c71649c5190c405925b9` failed
`TestSeenCloneReadTimeoutIsNotRefreshed` in two of three repetitions. The source
already includes cancellation repair `425b68b15`; the historical passing report
is not acceptance of this combined source.

Classification: confirmed native regression-test/acceptance failure. Whether the
root cause is production timeout classification or its hang-transport fixture is
unresolved. No baseline comparison or new source fix was attempted.

## Reproduction and evidence

From the Go module, the actual failing command selected this test alongside the
other A1-A6/C1-C3 tests:

```text
go test -count=3 -timeout 10m ./internal/wsdashboard ./internal/mcp ./internal/wswhiteboard ./internal/wsstate ./internal/wsindex ./internal/wsagent -run <recorded-focused-regexp>
```

The complete exact argument vector/regexp is retained in temporary artifact
`windows-696-9854ff350/native/focused.log`; parent evidence index:
`96d19897-01-windows-696-repass-evidence.md`. The corresponding native raw log is
`evidence/focused.log` in the owned Windows Temp fixture. Machine-specific
artifact locations remain untracked.

The test sets `ReadTimeout=200ms`, advances its fixture clock two minutes, and
activates the fixture's hanging transport. Expected compact output includes
`origin did not answer within 200ms` and `not refreshed`, retains cached ownership,
and does not say `unreachable`.

Actual, twice at `internal/mcp/ticket_index_view_test.go:615`: cached ticket rows
and ownership were retained, but the advisory was
`ticket-index: origin unreachable; ownership is from the cached index (age 2m0s)`.
The two failing test durations were 4.32s and 3.77s. The MCP portion failed after
39.504s; the complete focused command exited 1 after 63.047s. It started at
2026-11-14T13:48:27Z. No retry/repair was performed.

Native environment: ThinkPad P14s Gen 2a, Windows 11 Pro 10.0.26200, Go 1.26.2
windows/amd64, Git 2.38.1.windows.1, Python 3.12.10. Source was a pinned archive,
verified on both machines with SHA256
`542a67db53840af5f0c3d7bfafefc08d4576dcd5372564485c7bba80c8f35a87`.
Test HOME, cache/config, Go caches, Git system/global configuration and temporary
paths were isolated; no user checkout, installed plugin or shared daemon was
modified. The Go build passed before the test.

## Decisions

- Capture the current failure separately because matching historical tickets are
  done/dropped; do not rewrite their results.
- This capture neither attributes the failure to load nor weakens the expected
  timeout advisory. Diagnose transport result and context state first.
- No protocol/API/store change or compatibility tier is proposed by this evidence
  capture. No source fixes, promotions, merges or deployment are authorized.

## Phases

### Phase 1: Diagnose the native timeout classification failure

Reproduce under an approved isolated Windows test scope, distinguish transport
fixture failure from production timeout classification, and repair only the
established cause. Preserve cached ownership and the distinction between a read
deadline and actual origin unreachability. Verify the native regression without
altering a test merely to hide incorrect production behavior.
