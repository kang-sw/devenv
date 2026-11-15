# 777 verification slot checkpoint — incomplete acceptance

Ticket: 261115-feat-ws-whiteboard-committed-effects-in-conversation.
Branch: `impl/develop/fetal-olive-colt`.
Source candidate: `2ea2afce31398e5999a6a8763c5209168a3933a2`.
Integrated exact develop: `1223530580f90942524b613c949fa57bfa7eb335`, via lead-owned merge `f2a869e56a676390d43eaa760a24df545048ef48`.
MAIN slot: 2026-11-14T16:38:33Z–16:53:33Z. Worker execution began at 16:40:21Z. No browser acceptance was run. This checkpoint is not a successful phase Result, ticket closure, merge-ready verdict, or implementation acceptance.

## Raw evidence

All commands, exact heads, outputs and UTC start/end stamps are retained at `/tmp/ws-777-verification.aMhgUO/`. The original failed log is preserved, not overwritten. The full 832-line `07-full-mcp.log` was read in full. Only the test fixture changed during this slot; no product behavior changed.

Every Go command ran in `agents-plugin-tool/` with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=2`, `-p=1 -parallel=1`, existing caches only, no outer kill timeout.

| Step | Exact command after environment prefix | Source HEAD | UTC interval | Observed result / log |
|---|---|---|---|---|
| 1 | `go test -p=1 -parallel=1 ./internal/wswhiteboard -run 'TestSemanticEffects\|TestFolderCommitObservation' -count=1 -timeout=2m` | f2a869e56 | 16:40:21–16:40:25 | Exit 0, 2.409s; `01-wswhiteboard.log`. |
| 2 | `go test -p=1 -parallel=1 ./internal/wslive -run TestWhiteboardEffects -count=1 -timeout=2m` | f2a869e56 | 16:40:35–16:40:36 | Exit 0, 0.449s; `02-wslive.log`. |
| 3 | `go test -p=1 -parallel=1 ./internal/wsdashboard -run 'TestWhiteboardEffects\|TestSavedConversation\|TestHistory\|TestTools\|TestAdapterPresence\|TestBoardFirst' -count=1 -timeout=3m` | f2a869e56 | 16:40:46–16:40:52 | Exit 0, 1.074s; `03-wsdashboard.log`. |
| 3b | `go test -p=1 -parallel=1 ./internal/wsdashboard -run '^TestLeadPopoverLaunchBehavior$' -count=1 -timeout=2m` | f2a869e56 | 16:41:00–16:41:01 | Exit 0, 0.705s; `03b-popover.log`. |
| 4 original | `go test -p=1 -parallel=1 ./internal/mcp -run 'TestWhiteboardEffects\|TestWhiteboardPublication' -count=1 -timeout=3m` | f2a869e56 | 16:41:09–16:41:22 | Exit 1: html injection subtest did not trigger; `04-mcp.log`. |
| 4 authorized single retry | Same exact command/environment as original step 4. | 2ea2afce3 | 16:44:00–16:44:10 | Exit 0, 5.322s; `04-retry-mcp.log`. |
| 5 | `go test -p=1 -parallel=1 ./internal/wswhiteboard -count=1 -timeout=2m` | 2ea2afce3 | 16:44:20–16:44:40 | Exit 0, 19.700s; `05-full-wswhiteboard.log`. |
| 6 | `go test -p=1 -parallel=1 ./internal/wslive -count=1 -timeout=2m` | 2ea2afce3 | 16:44:48–16:44:50 | Exit 0, 0.993s; `06-full-wslive.log`. |
| 7 | `go test -p=1 -parallel=1 ./internal/mcp -count=1 -timeout=4m` | 2ea2afce3 | 16:45:06–16:49:07 | FAIL/TIMEOUT, exit 1, 240.761s; `07-full-mcp.log`. Cause unclassified. |

The backslashes before table-cell regex pipes are Markdown escaping, not shell arguments; raw logs retain the command strings actually executed.

## Diagnosed step-4 fixture failure

`newLiveFixture` retained raw `t.TempDir()` root, copied by `effectsWhiteboardFixture`. On this host raw TMPDIR was `/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/`, while `pwd -P` resolved `/private/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T`. Session login canonicalizes through Git and `EvalSymlinks`; publication derives the output directory from that stored session root. The exact predicate `path == wb.pageDir()` therefore missed the canonical path. The original failed temporary suffixes were not logged, so this is source/environment proof, not a fabricated runtime trace of those suffixes.

MAIN authorized one test-fixture-only fix and identical-command retry. Commit `2ea2afce3` resolves the existing key and gives the test fixture its actual `entry.root` before installing the unchanged hook. No assertion was weakened/removed and no test was skipped. Detailed diagnosis: `04-fixture-diagnosis.md` in the raw evidence directory.

## Unclassified full-MCP timeout

Observed: Go's package-wide four-minute alarm fired while `TestNoRefPathIsByteIdentical` had been running for one second. The stack includes `resolveIndexCaller` waiting for Git, `indexPiggyback`, and many dashboard-nudge goroutines. These are observations only: they do not prove that the timeout is pre-existing, baseline behavior, an environment fault, or independent of this ticket. No retry, timeout increase, product fix or browser run followed it. MAIN/root owns routing that failure.

## Commitment, compatibility and limitations

The executed focused contracts cover semantic effects, failed intent installation, installed durable intent followed by materialization/publication errors, append failure, the accepted commit-to-ring gap, session isolation/permissions, no-adapter/stale history, bounded escaped rendering, and pinned old reader/renderer behavior. They do not guarantee projection replay/delivery or establish browser geometry/interaction acceptance.

Authoritative boundary remains synced `pending.json` installation (`wswhiteboard/folders.go:700–712`, `865–903`); recovery materializes that existing decision before reads (`295–305`). Apply success and final handler error are not commitment evidence. Directory-sync failure after rename is unconfirmed, not proof of refusal. HTML publication and conversation append remain separate observed outcomes; no outbox, new outer store shape or stronger delivery guarantee was added. T0, selected 801a and MAIN-assumed 805a are preserved.

Independent static review reports remain at:
- `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@a93cfe66/review-paths/9dcace61-01-whiteboard-effects-static-lite.md`.
- `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@a93cfe66/review-paths/4cc041db-01-whiteboard-effects-critical-fix-static.md`.

Their Critical implementation fix and Important coverage fix were committed before execution. No fresh broad review occurred in this slot.

## Process ownership and omissions

An initial `ps -axo pid,ppid,pgid,lstart,command` snapshot is retained as `processes-before.txt`; a post-test snapshot and candidate comparison are retained as `processes-after-tests.txt` and `new-process-candidates.txt`. The 16:49:52Z scan found no surviving new Go test/build/fixture process candidates. New unrelated candidates were MAIN's deadline reminder (PID 23258, parent root Pi 2782) and another Pi session's launcher (PID 29979, parent Pi 29941); neither was killed. The worker Pi was PID 19944. A final scan is required before SLOT DONE; its output will remain beside these files. No cleanup kill was necessary at the post-test scan.

Omitted: browser fixtures/captures and interaction acceptance; full wsdashboard (explicitly prohibited because of holder leaks); a successful full MCP result; runtime/daemon rebuild, restart, swap, smoke or profiling; provider calls; dependencies/download/install; phase Result, ticket closure, merge and push. The ticket remains incomplete and is not merge-ready.
