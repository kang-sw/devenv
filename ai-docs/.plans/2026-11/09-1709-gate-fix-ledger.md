# Release-gate fix, unit A: wsledger / wstrackerhost / mcp

The ws 0.48.13 release gate (lead-review of `11ef849000956f9e0282def355e3298b51f9faf9..889db46fb`) returned NEEDS FIX. User decisions (board, 2026-11-09): (146) fix locally; (147) three parallel worktree units, then re-review the whole range including the fixes. This unit owns the Go findings in `internal/wsledger`, `internal/wstrackerhost`, `internal/wstracker`, `cmd/ws-mcp`, and `internal/mcp`. Units B (wsdashboard) and C (whiteboard / wsagent / shipped text) run in parallel on other branches; do not edit their files.

Full findings file (read-only reference): `/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/e20afafa-01-findings.md`, section "Unit A".

## Fixes (each with a test that fails before the fix)

- **A1** `internal/wsledger/remote.go:758-775` Restore: restoring another member's event ref fails whenever that member's attachment ref must also go back (clone fetched it with `--filter=blob:none`; with `GIT_NO_LAZY_FETCH=1` the `--atomic` push cannot pack the missing blobs). Push the attachment ref only when this clone holds every blob its tree needs; otherwise restore the event ref alone and report a warning through the existing result/warning channel. Test: Restore of another member's refs where the attachment ref was deleted remotely and its blobs are absent locally - the event ref is restored and a warning is reported.
- **A2** `internal/wsledger/attach.go:356` + `internal/wstrackerhost/attachments.go:166`: ReadAttachment's on-demand `fetchObject` has no network timeout and runs under `l.mu` (and the host's `st.mu`). Bound the fetch with the existing network timeout the way `h.attempt` does.
- **A3** `internal/wsledger/store.go:1041` + `wiki.go:1165` + `remote.go:160`: a `pushLocalWikis` error returns from Sync before `fetchRemote`, `refresh`, `reissue`, `clearDeadline`; WikiSave pushes all wikis so one rejected local-only wiki refuses saves to every other wiki; a wiki fetch failure at the end of `fetchRefs` skips `absorbRemote` for already-fetched event tips. Wiki push/fetch failures must not abort the ledger's own sync steps or other wikis' saves; surface them as a wiki-level error/warning. A test per path.
- **A4** `internal/wstrackerhost/wiki.go:225` (via `host.go` `attempt`): a wiki save records a full successful sync (`SyncedAt`/`AttemptedAt` set, `Error` cleared), hiding a standing ledger sync error and throttling the next PreRead. A wiki save must not overwrite the ledger sync state. Test.
- **A5** `internal/wsledger/wiki.go:1288-1310` renameGuard: on delete, any `.md` in theirs with the same blob as the base page counts as a rename. Count a rename only when the same blob appears at a path not present in base. Test.

## Tests to add (coverage gaps)

- **A6** `internal/wstracker/state.go:1052`: the guard dropping a "review" register winner while the item holds open records has no test (mutation `if false` passes). Add a review-stage case reaching it.
- **A7** `internal/wsledger/attach.go:376`: "fetch failure is not 'not found'" untested; the route claim that a non-sentinel host error answers 500 is untested. Add tests.
- **A8** `cmd/ws-mcp/tracker_host.go:323-325`: the real adapter's error mapping (ErrNotImage->415, ErrAttachmentTooLarge->413, ErrAttachmentNotFound->404) is untested. Add tests.
- **A9** `internal/mcp/server.go:723`: the access gate on wiki.query / wiki.update (spawned-key and unknown-key refusals, `trackerSessionKey` pinning in `wikiStore`) is untested beyond one case. Extend the refusal loops to the wiki tools.

Minors you touch on contact may be fixed (e.g. `ErrTrackerWikiNotFound` mapped to 500 on the render route, `wiki_test.go:808` checking Kind only); do not go looking for more.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/ws-mcp.md` before editing.
- Do not touch the user's dashboard daemons on ports 29417 / 45743. Never run `git stash`. Do not push.
- Do not edit `ai-docs/tickets/`.

## Verification

`cd agents-plugin-tool && go test ./internal/wsledger/... ./internal/wstrackerhost/... ./internal/wstracker/... ./internal/mcp/... ./cmd/ws-mcp/...`, then the full `go test ./...` once at the end; `go test -race` on the packages you changed. One or more commits on branch `fix/gate-0.48.13-ledger`, this plan committed with the first.
