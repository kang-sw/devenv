# Release-gate fix, round 2: tests the re-review found missing

The ws 0.48.13 release gate re-review (range `11ef849000956f9e0282def355e3298b51f9faf9..f7de37da9`, after fix units A/B/C) found one Important test gap and a few minor ones in the round-1 repair. The user authorized autonomous fix rounds until the gate clears (board, 2026-11-09). This round adds tests only, unless a test exposes a real defect; then fix the defect minimally and say so in the commit. A correctness reviewer is still running; its findings, if any, may arrive as a follow-up message to you in this same assignment.

## Important

- **R1** `agents-plugin-tool/internal/mcp/tracker_tools.go:991` (commit ed75b8d3a): `formatTrackerSyncReport` passes warning details through `wstrackerhost.RedactCredentials`, because a `WarnWikiSyncFailed` warning carries git's error text, which can name a remote with credentials. No test covers it: removing the call leaves every mcp test passing. Add a test (a `formatTrackerSyncReport` unit test on a `SyncReport` with such a warning is enough, or a sync-report tool test) whose warning detail contains `https://user:token@host/...` and asserts the token is absent from the output. Confirm it fails with the redaction call removed.

## Minor (do them; each is small)

- **R2** `internal/wstrackerhost/wiki.go:221` (A4, ad11abc68): the `networkTimeout` bound on a wiki save is untested; passing the caller's `ctx` instead of `saveCtx` passes every `TestWiki*`. Pin it with a deadline-recording runner like A2's `TestReadAttachmentBoundsTheFetch`.
- **R3** `internal/wsledger/wiki.go:410` (A3): "the other wikis are still pushed" after one wiki's push fails is untested; `continue` -> `break` passes. Add a case with a failing wiki ordered before a pushable local-only wiki and assert the second one reaches the remote.
- **R4** `internal/wsledger/remote.go:801` (A1): the `len(specs) == 0` branch calling `warnNotRestored()` (event ref already on the remote, only the attachment ref missing, blobs absent locally) has no test. Add one.
- **R5** `internal/mcp` spawned-key refusal loops for wiki.query / wiki.update: assert the JSON-RPC -32601 error envelope from the call gate (`server.go:723`, `isWikiTool`) so removing the gate layer alone fails a test (today `trackerSessionKey` in `wiki_tools.go:63` refuses with the same text as a tool error, masking it).

Each new test must fail when its target line is reverted or broken; check that and report it.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/ws-mcp.md` before editing.
- Do not touch the user's dashboard daemons on ports 29417 / 45743. Never run `git stash`. Do not push. Do not edit `ai-docs/tickets/`.

## Verification

`cd agents-plugin-tool && go test -count=1 ./internal/mcp/... ./internal/wsledger/... ./internal/wstrackerhost/...`; `go test -race` on the packages you changed. One or more commits on branch `fix/gate-0.48.13-round2`, this plan committed with the first.
