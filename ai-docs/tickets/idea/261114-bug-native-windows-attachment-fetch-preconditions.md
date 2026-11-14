---
title: Native Windows attachment tests fail their absent-blob fetch and busy-read preconditions
related:
  261110-bug-ws-tracker-store-lock-network-split: completed busy-read contract predecessor; its result is not changed by this capture
---

# Native Windows attachment fetch preconditions

## Background

The native Windows 696 re-pass of source
`9854ff350fa1b0eccfb5c71649c5190c405925b9` reproduced two historical
`internal/wstrackerhost` and four `internal/wsledger` acceptance failures. Matching active tickets were not
found by exact test-name and attachment-blob discovery.

Classification: confirmed native test/acceptance failures, with an apparent
missing-blob fixture precondition mismatch. This does NOT establish incorrect
production attachment behavior. The older Git version, fixture construction,
production fetch behavior and transient file holds must be distinguished before
choosing a fix. No baseline comparison, repair or retry was performed.

## Reproduction and evidence

Actual native command from the Go module:

```text
go test -count=1 -p=2 -timeout 25m ./internal/gitbatch ./internal/wsindex ./internal/wsdashboard ./internal/wsstate ./internal/wswhiteboard ./internal/wsnote ./internal/wstrackerhost ./internal/wstrackerhook ./internal/wsledger ./internal/wsagent ./internal/execjob
```

The complete raw output is retained in temporary artifact
`windows-696-9854ff350/native/packages.log` (native counterpart
`evidence/packages.log`), indexed by
`96d19897-01-windows-696-repass-evidence.md`. Machine-specific paths remain
untracked. The command started 2026-11-14T13:54:44Z; trackerhost completed rather
than timing out, failing after 815.843s.

Observed assertions:

- `TestReadAttachmentBoundsTheFetch`, 16.23s, at
  `internal/wstrackerhost/attachments_test.go:245`:
  `the read fetched nothing; the test needs the blob absent`.
- `TestRemoteAttachmentReadAnswersBusyWhileNetworkHeld`, 19.60s, at
  `internal/wstrackerhost/netsplit_test.go:214`:
  `bounded read of a not-yet-local attachment in the checking process = <nil>, want ErrStoreBusy`.
  Its TempDir cleanup also failed to remove fixture `net.lock` because another
  process held the file. This secondary cleanup symptom is recorded, not assumed
  to share a proven root cause with blob availability.

Expected: fixtures leave the attachment blob absent until the tested bounded
fetch, so the read must exercise the network/busy path. Actual observations fail
that precondition or return nil instead of the intended busy result. They do not
prove that a legitimately local cached attachment should report busy.

The same command subsequently completed `internal/wsledger` after 983.554s,
without a package timeout, and recorded four matching absent-blob/fetch-path
precondition failures:

- `TestAttachPartialCloneFetchesBlobOnRead`, 12.33s, `attach_test.go:294`:
  `the filtered fetch brought the attachment blobs`.
- `TestAttachRestoreWithoutBlobsRestoresEventRefAlone`, 11.58s,
  `attach_test.go:488`: `the filtered fetch brought the blob; the test needs it absent`.
- `TestAttachRestoreWithoutBlobsAttachmentOnly`, 11.07s,
  `attach_test.go:532`: the same absent-blob assertion.
- `TestAttachFetchFailureIsNotNotFound`, 11.18s, `attach_test.go:652`:
  `read over a failing fetch = <nil>, want the fetch's error and not ErrAttachmentNotFound`.

These ledger failures are grouped by the same missing-blob fetch precondition,
not by a claimed proven common implementation cause. The same six failures are
recorded in the prior Windows Go revalidation temporary report, but no active
ticket owned them. This pass did not repeat the earlier 10-minute package cutoff:
it used the already-approved 25-minute package budget.

Environment: native ThinkPad P14s Gen 2a, Windows 11 Pro 10.0.26200, Go 1.26.2
windows/amd64, Git 2.38.1.windows.1, Python 3.12.10. Source archive SHA256 was
verified before extraction:
`542a67db53840af5f0c3d7bfafefc08d4576dcd5372564485c7bba80c8f35a87`.
Test roots, homes, Go caches, Git configuration, ws stores and subprocesses were
isolated. No user checkout, installed plugin, shared daemon or host configuration
repair was part of this command.

## Additional full MCP acceptance evidence (2026-11-14 UTC)

MAIN 15:00:19 expressly requested the three final MCP failing subcases be recorded
in this SAME Windows IDEA, alongside the attachment failures, without a second
MCP ticket or inferred common diagnosis. This section groups an acceptance run,
not a proven common root cause. A separately drafted uncommitted route IDEA was
removed before commit to honor the single-writer capture boundary.

Actual command: `go test -count=1 -timeout 25m ./internal/mcp`, starting
2026-11-14T14:34:44Z. It completed within budget with exit 1 after 1356.290s;
Go reported package duration 1354.882s. Full retained output:
`windows-696-9854ff350/native/mcp.log`, native counterpart `evidence/mcp.log`,
under the same temporary evidence index and exact source/toolchain pins above.

All three observed failing subcases:

1. `TestImplIdentityResolveProvisionRouteIntegration/develop`, 1.51s, at
   `impl_identity_integration_test.go:142`: route response JSON parsing failed
   (`invalid character 'r' looking for beginning of value`). The returned text
   said branch preflight failed while checking implementation ahead state:
   `git rev-list --count <base>..impl/develop/goofy-chief-many` returned 128,
   `fatal: failed to stat '<revision-range>': Filename too long`.
2. `TestImplIdentityResolveProvisionRouteIntegration/nested_goal`, 1.58s, same
   assertion: the corresponding
   `git rev-list --count <base>..impl/goal/develop/topic/army-lanky-wipe` returned
   128 with `Filename too long`. The two-case outer test took 3.09s. Exact object
   IDs/ranges and refusal text remain in the raw log.
3. `TestSeenCloneReadTimeoutIsNotRefreshed`, 3.77s, at
   `ticket_index_view_test.go:615`: expected a 200ms read-timeout/not-refreshed
   advisory retaining cached ownership; actual cached rows/ownership remained,
   but the advisory said
   `ticket-index: origin unreachable; ownership is from the cached index (age 2m0s)`.
   This same contract already has the focused-run capture
   `261114-bug-native-windows-ticket-read-timeout-reports-unreachable`; this is
   additional settled full-suite evidence, not a new test retry or duplicate
   ticket.

These are observed native acceptance failures. No diagnosis, baseline comparison,
source fix, timeout extension, host long-path/config change or rerun was
performed. The full MCP command is NOT green.

## Decisions

- Preserve the completed predecessor's results; capture fresh native evidence in
  an IDEA rather than editing done/dropped tickets.
- Diagnose the fixture's absent-blob assumption before classifying production
  behavior as wrong. Do not weaken the assertions merely to obtain green output.
- This capture authorizes no source fix, promotion, merge, protocol/store change
  or release action.

## Phases

### Phase 1: Establish the native attachment fetch preconditions

Under an approved isolated Windows scope, establish which clone/fetch operation
makes the blob local, compare supported Git behavior and production semantics,
and diagnose the lock-cleanup hold separately when necessary. Repair only the
established implementation or fixture cause, then verify both genuinely absent
and already-local attachment paths without altering owner Git settings.
