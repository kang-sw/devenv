# Windows wswhiteboard CI round2 v0.50.0 ship blocker

## Authority

MAIN assigned one task 2026-11-15T14:03:20Z, ETA30min (~14:33Z). Read `/tmp/ci-red-round2-brief.md` fully; execute ONLY EXEC1 section. Source base develop159d1c3e3 or newer, acquired own full pool corporate-predator-shrug, key unrushed-freebee-hatchling, branch fix/windows-whiteboard-ci-v0500. Ordinary Pi worker only, no fork. Commit AI Context: v0.50.0 ship blocker, compatibility T0 unless a real surface change demands discussion. No push, merge, daemon swap or CI dispatch. HOLD checkout after final until MAIN's merge report; lead owns release.

## Scope and evidence

CI43721279531 failed-only log `/tmp/ci-43721279531-failed.log`; all reported reds postdate last green afe7f1f16. Investigate ONLY internal/wswhiteboard:
1. TestTopicStateHistoricalSchema2Writer and TestWaitingOnHistoricalWriter: exec historical-writer missing, likely Windows executable naming.
2. TestFolderChangedTopicMeta112, TestFolderChangedTopicMetaLifecycle, TestFolderChangedTopicMetaExactDiskBytes: folders_writes_test.go90 meta.json identity did not change after atomic replace on Windows.
3. TestBodyTimeVerificationRunnerEarlyExitCleanup and TestBodyTimeNextSupervisorReceipts: bodytime_ui_test.go52/67 Python unittest.mock AttributeError on Windows.

For each report test -> root cause with file:line -> introducing commit after afe7f1f16 -> fixture portability versus product defect -> precise fix. Source and Git history are authority; do not infer all three causes from the brief. Repair portable fixtures/test helpers and minimal behavior-preserving implementation bugs if proven. STOP before any product behavior/protocol change. Never delete/skip/weaken tests, never merely increase timeouts. Do not work EXEC2/EXEC3 sections or earlier Rail reds. Before editing read matching project manuals (shipped-surface-boundary and release-compatibility for internal source; skill-authoring when applicable).

## Verification and safety

Run ONLY the seven named tests with a precise anchored -run selection under `./internal/wswhiteboard`, -count=1, on macOS and the Windows host. No full wswhiteboard/mcp suite or unrelated packages. Every Go shell invocation explicitly absolute cd to OWN agents-plugin-tool (remote unique copy included), GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local; isolated HOME/config/runtime/temp, warm existing build/module caches. Offline, no downloads. Use bounded Go timeout rather than outer-killing tests. Historical-writer nested Go subprocesses need offline/isolation environment inherited as well.

Windows ssh ki608@192.168.33.6. New unique folder per run, copy/export own committed head. No default `%APPDATA%\Werkschaft`, no `C:\Users\ki608\devenv`, no old task roots. Never image-name kills, never signal2640; only owned recorded PID plus matching CIM UTC creation ticks. Protected2640 CIM639302110386172090 authority; Get-Process639302110386172097 is known+7tick artifact. Read-only before/after proof, no daemon/provider/shared runtime/config changes, no foreign keys/signals/witness holder/forks/outer Go kills. Never Electron on Mac (none needed). Optional unsupported probes are observations to skip; diagnose actual scoped test failures within task rather than blind retry or expanding execution.

## Handoff

Commit fix plus unchanged plan. Per-test root cause/introduction/verdict/fix table with citations; exact Windows/macOS final-head scoped ok lines and complete logs read; no new skips/deletions; source hashes/own cleanup/protected identity evidence. Store all large evidence outside checkout under fresh `/tmp` root. Report concise complete English with VERIFIED/INFERRED labels. Push material blocker, head-ready, or ETA slip; no routine acknowledgements. Lead skims diff/report, does not rerun your tests. Keep clean checkout HELD until MAIN reports merge; never resume worker after release/recycling. Only MAIN assigns next task.
