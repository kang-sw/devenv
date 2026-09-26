---
title: Hang-transport stop wait fails on the local Windows test host
---

# Hang-transport stop wait fails on the local Windows test host

## Background

On the local native-Windows test host (Go 1.26.2, Git for Windows 2.38.1,
OpenSSH_for_Windows 9.5p2), three hang-transport tests fail every run:

- `internal/wsindex` `TestA4UnreachableWithCacheIsBoundedAndPending`
- `internal/wsindex` `TestNeverSeenTimeoutsCacheAbsencePerPath`
- `internal/mcp` `TestSeenCloneReadTimeoutIsNotRefreshed`

All three fail in the Windows stop wait of the two hang-transport copies
(wsindex `stopHangTransports` in `storage_test.go`, mcp `ixCheckout.hang` in
`ticket_index_test.go`): "2 hanging ssh transports still running after the
stop file". The 30 s Windows wait expires
with two live markers left. TempDir removal then fails because a process still
holds the test directory.

Observed 2026-09-26 while verifying the merge of
`260924-chore-review-sweep-test-and-naming-minors` (fe26fda57). The failure
also reproduced 3/3 for each test on the pre-merge commit 340889a15, so it predates that
merge. The `ws-mcp CI` `windows-latest` leg passes the same tests, including
the v0.46.20 release gate. The difference is therefore host-specific. The
leading suspect is the old Git for Windows (2.38.1) and its bundled `sh`, whose
`kill -0` / background `cat` / `sleep` semantics may differ from the CI
runner's current Git for Windows. That is not verified.

Every other test in the suite passed on the same host; `internal/mcp` needs
CI's `-timeout 30m` there, since it exceeds go test's default 10 min package
timeout.

## Open questions

- Does upgrading the host's Git for Windows make all three pass? If it does,
  the tests or a manual should state the minimum Git for Windows they assume.
- If it does not: which part of the hang script keeps running after the stop
  file appears (the poll loop, the background `cat`, or git's own ssh child),
  and should the Windows wait kill the transports' processes instead of
  relying on the script to notice the stop file?
