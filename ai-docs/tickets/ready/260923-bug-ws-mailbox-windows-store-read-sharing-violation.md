---
title: Retry transient Windows sharing violations on mailbox store reads
related:
  261022-feat-mailbox-recent-peers-and-correspondents: context
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ac9edf211a1cc6cf
sage-review-completeness-reviewed: ac9edf211a1cc6cf
---

# Retry transient Windows sharing violations on mailbox store reads

## Background

Windows dogfooding reported a Pi warning from `ws-mcp mailbox wait`: `read mailbox store ...\mailbox.json: open ... The process cannot access the file because it is being used by another process.` This is a read/open failure, not evidence that the separate mailbox writer lock timed out. The user subsequently observed the waiter retrying.

`agents-plugin-tool/internal/wsmailbox/store.go` `Load` reads the store with a bare `os.ReadFile`, without a retry or the writer lock. Writers persist through temp-write plus `replaceFile` (`wsmailbox/replace_file_windows.go`), which already retries `ERROR_ACCESS_DENIED` / `ERROR_SHARING_VIOLATION` for about two seconds because unlocked readers in other ws-mcp processes hold the destination. The read side has no counterpart: a reader that opens the file while a replace is in flight fails at once. `wait.go` terminates an individual wait invocation on a peek error; `cmd/ws-mcp/mailbox.go` surfaces that error. In Pi, `agents-plugin-pi/src/mailbox-waiter.ts` restarts the subprocess after the error backoff (default 5 seconds), consistent with the observed retry.

The same failure fails Windows CI: `TestMailboxRecvConcurrentCallsNeverDoubleDeliver` (`internal/mcp/mailbox_recv_concurrency_test.go:111`) reports `mailbox.recv: named-inbox owner check: read mailbox store ...mailbox.json: ... being used by another process`. It failed on roughly half of the recent runs (9f6e139fc, ba6328dfd, 8600a9c54, 869003e09), and on every attempt after `261022-feat-mailbox-recent-peers-and-correspondents` (CI runs 39719908503, 39720636638, 39721167198 attempts 1 and 2). That feature adds an unlocked `mailbox.json` read per send to a named address (`mailboxContactFor`, `internal/mcp/mailbox_runtime.go`) and a reply-registry write on recv (`recordMailboxRecvContacts`), so reads and replaces overlap more often. The v0.48.1 release is held on this fix.

## Decisions

- Phase 1 is the read-retry fix rather than an investigation, and it lands on develop before v0.48.1 ships; the v0.48.1 publish re-pins to the commit that lands it and requires green Windows CI, including `TestMailboxRecvConcurrentCallsNeverDoubleDeliver`. The re-pin, tag, and any version step belong to the lead's release gate, not to this phase; the `ws@0.48.1` bump (a206b62e4) already exists. User choice on 2026-10-22 after the v0.48.1 Windows CI failures. Rejected: shipping v0.48.1 by re-running the flaky job until green; holding the release.
- Transient means `ERROR_SHARING_VIOLATION` or `ERROR_ACCESS_DENIED`; retry for about two seconds with 5 ms back-off doubling to a 200 ms cap, mirroring `replaceFile` in `wsmailbox/replace_file_windows.go`, which retries the same contention on the write side.
- Non-Windows builds read with a plain `os.ReadFile` and no retry, through the existing `replace_file_windows.go` / `replace_file_other.go` build-tag split; sharing violations are Windows-only.
- All four unlocked `os.ReadFile` sites in `wsmailbox` (`Load`, `LoadReplyStore`, `ReadListeningMarker`, `readHookNotifyState`) go through the one helper: each reads a file that `replaceFile` replaces, and one helper keeps them from drifting. Rejected: fixing only `Load` - `LoadReplyStore` sits on the recv and lookup_peers paths that 261022-feat-mailbox-recent-peers-and-correspondents made busier.
- The retry loop is verified through an injected read function and classifier on every platform. Rejected: a Windows-only real-contention test - Linux cannot produce a sharing violation, and the seam keeps budget and classification checkable everywhere; Windows CI remains the real-contention acceptance.

## Constraints

- Out of scope: the Pi waiter restart/backoff (`agents-plugin-pi/src/mailbox-waiter.ts`), the write-side `replaceFile`, and the `wsstate` / `wsconfig` replace helpers.
- Missing-file handling and parse/decode errors keep their current behavior and messages.
- `agents-plugin-tool/` is a shipped surface: read `ai-docs/manuals/shipped-surface-boundary.md` before editing.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 29f4e8d9 (2026-10-08, commit): "User hit 'atomic rename mailbox reply registry: ... Access is denied.' on native Windows and asked for a hotfix in this release." (introduced the wsmailbox replaceFile retry this ticket mirrors) — bearing: supports
- 261022-feat-mailbox-recent-peers-and-correspondents (2026-10-22, Decisions): "Contact recording on `mailbox.send` folds into the same reply-registry lock write `publishReplyID` already performs... A named counterparty's presence record is read from its scope store before taking the reply lock, never nested inside it." — bearing: constrains
- 261008-bug-windows-mailbox-wait-outlives-pi-host (2026-10-08, Result 2186143ab): "a transient Windows sharing violation on the clear's read would likewise leave the marker until the next arm." — bearing: supports
- 67b443dd (2026-09-30, commit): "wsstate's atomicReplaceFile retries only ERROR_SHARING_VIOLATION, which would not have caught this failure, so wsconfig gets its own small helper instead of importing that one." — bearing: supports
- b318a127 (2026-09-21, commit): "Used deterministic hooks for retry tests instead of timing-dependent Windows process contention." — bearing: supports
- 92ece2bf (2026-09-23, commit): "Clarify that one Load/Wait invocation still returns on the read error, and leave the necessity of additional read-side retry open pending investigation." — bearing: supports
- 260622-chore-windows-shipping-hardening (2026-06-22, Result 8461b4cf): "`replaceFile` (wsagent + wsstate) delegates to a build-tagged `atomicReplaceFile` — Windows uses `MoveFileEx(REPLACE_EXISTING|WRITE_THROUGH)` with a bounded `ERROR_SHARING_VIOLATION` retry; Unix stays `os.Rename`." — bearing: constrains
- 260913-test-mailbox-and-landing-gate-coverage-gaps (2026-09-13, Result e31576b8): "`store_test.go` `TestLoadRejectsCorruptStore` — malformed JSON surfaces a `parse mailbox store` error, not a silently-zeroed store." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsmailbox/store.go, replyid.go, listening.go, hook_peek.go, replace_file_windows.go, replace_file_other.go plus tests |
| scope.surface | internal | unexported read helper inside package wsmailbox; exported Load, LoadReplyStore, ReadListeningMarker keep their signatures |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none; helper is unexported |
| scope.test_surface | new-files | no existing read-retry test; pattern in agents-plugin-tool/internal/wsmailbox/replace_file_test.go and replace_file_windows_test.go |
| complexity.reuse_points | confirmed | replaceFile loop and isTransientReplaceError at agents-plugin-tool/internal/wsmailbox/replace_file_windows.go#L21-L42 |
| complexity.side_effect_risk | moderate | every Windows mailbox read can now block up to about 2s, including reads made under WithLock or WithReplyLock |
| risk.correctness | moderate | missing-file and parse errors must pass through unchanged and ERROR_ACCESS_DENIED may also be a persistent permission failure |
| risk.fit | low | mirrors the existing build-tag split and retry budget in the same package |
| risk.test | moderate | real contention is reproducible only on Windows CI; Linux tests cover the loop only through the injected seam |
| risk.security_or_contract | low | no protocol, API, or error-message change per Constraints |

## Phases

### Phase 1: Retry transient sharing violations on unlocked mailbox reads

Give `wsmailbox` a read helper that retries a transient Windows open failure with the same classification and budget as `replaceFile`, and route all four `os.ReadFile` sites in the package through it, whatever their lock context (`Load` also runs inside `WithLock`), so a reader that collides with an in-flight replace waits briefly instead of failing. Persistent and unrelated failures (missing file handling, parse errors, other I/O errors) behave exactly as today.

The parameterized retry loop lives in a file without a build tag, taking the read function, transient classifier, clock, and sleep as inputs (as `WaitOptions` already injects time); only the wiring - which classifier is used, and whether a retry applies at all - is split across `replace_file_windows.go` / `replace_file_other.go`. Inside `WithLock` no cooperating writer can be replacing the file, so a retry there fires only on an outside holder such as an AV scanner; note at the site that this can lengthen the lock hold beyond `LockTimeout`.

Verification: unit tests drive the retry loop through an injected read function and transient-error classifier on every platform - transient errors N times then success, a non-transient error returned at once, a missing file passed through unchanged, and budget exhaustion returning the last error (driven by the injected clock and sleep, not real time). `go test ./...` passes on Linux and `GOOS=windows go vet ./...` passes; Windows CI, including `TestMailboxRecvConcurrentCallsNeverDoubleDeliver`, is the real-contention acceptance.
