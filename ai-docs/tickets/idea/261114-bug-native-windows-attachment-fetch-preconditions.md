---
title: Native Windows attachment tests fail their absent-blob fetch and busy-read preconditions
related:
  261110-bug-ws-tracker-store-lock-network-split: completed busy-read contract predecessor; its result is not changed by this capture
---

# Native Windows attachment fetch preconditions

## Background

The native Windows 696 re-pass of source
`9854ff350fa1b0eccfb5c71649c5190c405925b9` reproduced two historical
`internal/wstrackerhost` acceptance failures. Matching active tickets were not
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

The same failures are recorded in the prior Windows Go revalidation temporary
report, but no active ticket owned them. This pass did not repeat the earlier
10-minute package cutoff: it used the already-approved 25-minute package budget.

Environment: native ThinkPad P14s Gen 2a, Windows 11 Pro 10.0.26200, Go 1.26.2
windows/amd64, Git 2.38.1.windows.1, Python 3.12.10. Source archive SHA256 was
verified before extraction:
`542a67db53840af5f0c3d7bfafefc08d4576dcd5372564485c7bba80c8f35a87`.
Test roots, homes, Go caches, Git configuration, ws stores and subprocesses were
isolated. No user checkout, installed plugin, shared daemon or host configuration
repair was part of this command.

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
