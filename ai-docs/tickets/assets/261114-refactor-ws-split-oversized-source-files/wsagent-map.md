# wsagent split map: `runner.go` and `lead_runner_test.go`

Planning asset for `261114-refactor-ws-split-oversized-source-files`. Same
binding rules as Phase 1's Decisions: compatibility T0, move-only within
`package wsagent`, no new packages, no renames, no extracted helpers or
wrappers, no logic cleanup or test redesign, declaration comments move with
their declarations, only package/import scaffolding and `gofmt` changes, one
move-only commit per destination file, no size-guard limit changes.

Snapshot: `616271b51` (the target files are byte-identical at `c36321736`;
`git diff 616271b51 c36321736 -- agents-plugin-tool/` is empty). Line ranges
are 1-based and inclusive in the snapshot file, from the first line of a
declaration's doc comment to its closing line. Every cut point is a blank line
(`runner.go` 1290, 2177, 2799; `lead_runner_test.go` 780, 1089, 1500).
Resulting sizes below were measured on a dry run of this exact map in a /tmp
copy of the tree (after import pruning and `gofmt`), not estimated.

Destination names were checked against the package: no existing file has these
names, and none ends in a GOOS/GOARCH suffix that would add a build constraint.
Existing `runner_recovery.go`, `runner_summary.go`, `runner_test.go`,
`lead_recovery_test.go` and `lead_takeover_test.go` are not touched.

## Target 1: `agents-plugin-tool/internal/wsagent/runner.go` (3555 lines)

Notation: `conv.x` is a method on `*conv`, `Runner.x` a method on `*Runner`.

### `runner_events.go`: snapshot lines 2800-3555, 770 lines after the move

Turn events, approvals, process exit, idleness, settings and close for one
conversation.

- Methods: `conv.handle`, `conv.changed`, `conv.beginTurn`, `conv.endTurn`,
  `conv.addApproval`, `conv.checkAction`, `conv.actionDue`,
  `conv.dropApproval`, `conv.forgetAnswered`, `conv.clearApprovals`,
  `conv.showApprovals`, `conv.answerApproval`, `conv.processExited`,
  `conv.processGone`, `conv.processEnded`, `conv.releaseHolder`,
  `conv.stopProcess`, `conv.letGo`, `conv.finishLine`, `conv.idleNow`,
  `conv.checkInterrupt`, `conv.holdForLever`, `conv.setSettings`,
  `conv.applySettings`, `conv.armIdle`, `conv.stopIdle`, `conv.close`.
- Variable: `actionGrace` (3062-3066).
- Constants: `actionNeededText` (3068-3069), `actionNeededTool` (3071-3072),
  `leverHoldCap` (3436-3438).
- Types/funcs: none.
- Final imports: `encoding/json`, `fmt`, `net/http`, `slices`, `time`,
  `wsdashboard`, `wsinbox`, `wslive`.

### `runner_process.go`: snapshot lines 2178-2798, 636 lines after the move

Process start, worker/lead launch, holder reattach and replay recovery.

- Methods: `conv.runCommand`, `conv.startProcess`, `conv.startWorker`,
  `conv.startLead`, `conv.retireHolder`, `conv.reattach`,
  `conv.checkReattach`, `conv.replayDone`, `conv.recoveryFailed`,
  `conv.probeWorkerRecovery`, `conv.probeWrapHold`, `conv.wrapProbed`,
  `conv.lineEnd`, `conv.persistSeq`.
- Function: `composeInstructions` (2772-2798; its only callers are the
  start paths in this range).
- Types/vars/constants: none.
- Final imports: `encoding/json`, `errors`, `fmt`, `os`, `path/filepath`,
  `strings`, `wsdashboard`, `wsholder`, `wsinbox`.

### `runner_conv.go`: snapshot lines 1291-2176, 901 lines after the move

The conversation type, its loop, registration/row ownership, live-event
apply, and the inbox kick.

- Type: `conv` (1291-1463, with its doc comment).
- Methods: `conv.do`, `conv.signalInbox`, `conv.signalMail`,
  `conv.mailChanged`, `conv.inboxChanged`, `conv.steer`, `conv.loop`,
  `conv.settle`, `conv.state`, `conv.record`, `conv.update`,
  `conv.setStatus`, `conv.writeRegistration`, `conv.lockRow`,
  `conv.checkRow`, `conv.release`, `conv.dropHeld`, `conv.apply`,
  `conv.kick`, `conv.voidCancelled`.
- Functions: `rowForeign` (1828-1840), `nextTurn` (2159-2176).
- Vars/constants: none.
- Final imports: `fmt`, `reflect`, `slices`, `strings`, `sync`, `time`,
  `wsdashboard`, `wsinbox`, `wslive`.

### Retained `runner.go`: snapshot lines 1-1289, 1287 lines after the series

Package API and the Runner: interfaces, options, the Runner and its request
methods, request errors, and the public `State` projection.

- Types: `KeyStore`, `LeadKeyStore`, `LeadSession`, `LeadMail`, `Options`,
  `Runner`, `requestError`, `LeadOptions`, `State`.
- Constants: `EnvHostedSession`, `EnvHostedSessionKey`, `EnvHostedSessionRow`
  (one group), `EnvAllowedTools`, `EnvNoDashboard` (one group),
  `StatusIdle`, `StatusRunning`, `StatusClosed`, `StatusHandedOff` (one
  group). Keep the existing grouped declarations.
- Variables: `mailboxIdentityEnv`, `errUnknown`, `errClosed`, `errHandedOff`.
- Functions: `NewRunner`, `badRequest`, `endedError`, `leadPluginIsolation`,
  `closedState`.
- Methods: `Runner.SetLiveHost`, `Runner.Start`, `Runner.markScanned`,
  `Runner.holdersScanned`, `Runner.scanDone`, `Runner.Close`,
  `Runner.InboxChanged`, `Runner.conv`, `Runner.startConv`,
  `requestError.Error`, `Runner.Create`, `Runner.CreateLead`,
  `Runner.create`, `Runner.lastCommands`, `Runner.resolve`, `Runner.backend`,
  `conv.startBackend` (832-839, stays in place among the Runner methods),
  `Runner.Get`, `Runner.List`, `Runner.Send`, `Runner.Interrupt`,
  `Runner.Stop`, `Runner.Restart`, `Runner.IsLead`, `Runner.AnswerApproval`,
  `Runner.SetPermissionMode`, `Runner.setPermissionMode`,
  `Runner.CloseConversation`, `Runner.closeOne`, `Runner.SetSettings`,
  `Runner.open`, `Runner.act`, `Runner.closedState`.
- Final imports drop `reflect` and `wsholder` (their last uses leave with
  `runner_conv.go` / `runner_process.go`); intermediate commits drop whatever
  `go vet` reports for that commit.

Retained size is above the soft 1000 line but this series only shrinks it
(3555 -> 1287), so the soft rule (`before <= soft && lines > soft`,
`internal/mcp/git_merge_size.go:229`) cannot fire. An optional fourth move,
`Runner.Create` .. `Runner.act` plus the request-error block (542-1119,
~580 lines) into a `runner_requests.go`, would bring it to ~710; it is not
required by the < 2000 rule (see Lead decisions).

### Commit order

Move bottom-up so the snapshot ranges of later commits stay exact in the
shrinking file: (1) `runner_events.go` 2800-3555, (2) `runner_process.go`
2178-2798, (3) `runner_conv.go` 1291-2176. Each commit removes exactly its
range plus one adjacent separator blank line, adds the destination with
`package wsagent` and the imports it uses, and removes now-unused imports from
`runner.go`.

## Target 2: `agents-plugin-tool/internal/wsagent/lead_runner_test.go` (2206 lines)

### `lead_lifecycle_test.go`: snapshot lines 1501-2206, 722 lines after the move

Daemon-restart persistence, holder start/scan/shutdown, idle stop, Pi turns
and mapper, failed sends, background-task turn end.

- Tests: `TestLeadAnsweredPromptStaysAnsweredAcrossADaemonRestart`,
  `TestLeadProcessThatExitedWhileNoDaemonRan`,
  `TestLeadRejoinDoesNotStallBehindALongReplay`,
  `TestLeadShutdownFinishesTheLineItStandsIn`,
  `TestLeadStartEndsThePreviousHolderFirst`,
  `TestLeadOrphanHolderIsEndedAfterTheFirstScan`,
  `TestLeadShutdownDuringTheFirstScanEndsNoHolder`,
  `TestLeadRejoinWaitsForTheFirstScan`,
  `TestLeadIdleStopWaitsForBackgroundTasks`,
  `TestPiLeadIdleStopWaitsForBackgroundShell`, `TestPiLeadIdleStopAndMailWake`,
  `TestPiLeadTurnsAndCompact`, `TestPiLeadSubagentRows`, `TestPiLeadMapper`,
  `TestLeadFailedSendShowsTheMessageOnce`,
  `TestLeadFailedMailOnlySendRetriesOnce`, `TestPiLeadFailedCommandRetriesOnce`,
  `TestLeadTurnEndsAtResultWhileABackgroundTaskRuns`.
- Fixtures/helpers: type `replayStatusGate` (1581-1589), method
  `replayStatusGate.Status` (1591-1595), func `ringKinds` (2054-2063; also
  used by `lead_claude_test.go:959`).
- Final imports: `encoding/json`, `errors`, `slices`, `strings`, `sync`,
  `testing`, `time`, `wsdashboard`, `wsholder`, `wslive`.

### `lead_rejoin_test.go`: snapshot lines 781-1499, 731 lines after the move

Claude-lead and Pi-lead rejoin, wrapped-ring holds, and replay probing.

- Tests: `TestLeadRejoinsItsProcessAfterADaemonRestart`,
  `TestLeadRejoinOfAWrappedRingHoldsInput`,
  `TestLeadRejoinOfARingThatWrappedWhileAttachedHoldsInput`,
  `TestLeadRejoinOfAWrappedRingTrustsATurnBoundary`,
  `TestLeadRejoinGapHoldsInputAndDropsAPartialLine`,
  `TestPiLeadRejoinOfAWrappedRingAsksWhetherATurnRuns`,
  `TestPiLeadRejoinProbeKeepsNativeRunAcrossCommandAnswers`,
  `TestPiLeadWrappedReplayCommandAnswerStillProbesNativeRun`,
  `TestPiWorkerCompleteReplaySettlesDetachedCommandAnswer`,
  `TestPiLeadCompleteCommandOnlyReplayKeepsIdleCompletion`,
  `TestPiLeadRejoinDoesNotRestorePastCommandError`,
  `TestPiLeadRejoinProbeIdleAnswerAfterATurnStartKeepsTheHold`,
  `TestPiLeadRejoinProbeAnswerForAnotherProcessIsDropped`,
  `TestPiLeadRejoinProbeThatSaysRunningOrFailsKeepsTheHold`.
- Helpers: funcs `piHeldRejoin` (1090-1127), `piPrompts` (1129-1141; also
  used by `common_contract_test.go:30`), `piStateAnswer` (1143-1145).
- Final imports: `encoding/json`, `reflect`, `slices`, `strings`, `testing`,
  `wslive`.

### Retained `lead_runner_test.go`: snapshot lines 1-779, 778 lines after the series

- Helpers: `specEnv` (also `lead_takeover_test.go:173`), method
  `leadHarness.controls` (also `common_contract_test.go:196`,
  `lead_recovery_test.go:36,60`, `lead_http_test.go:99,159`),
  `shortActionGrace`, method `leadHarness.steerHeld` (also
  `lead_command_test.go:455`).
- Tests: `TestLeadSpawnRunsUnderAHolder`,
  `TestLeadProfileEnvSetsSessionStateEvents`, `TestLeadApprovalIsAnsweredOnce`,
  `TestLeadHeldSendFlushesAtIdleNotAtResult`,
  `TestLeadHeldSendFlushesAtAnUnsolicitedTurnsEnd`,
  `TestLeadSteeredMessageReachesTheRunningTurn`,
  `TestPiLeadSteeredMessageIsTheSteerCommand`,
  `TestSteeredMessageWaitsInAProfileConversation`,
  `TestLeadMailIsDeliveredAsATurn`, `TestLeadMailClaimedDuringATurnTellsThePage`,
  `TestLeadMailWaitsForTheRejoin`, `TestLeadSettingsChangeInStream`,
  `TestLeadSettingsRestartOnlyWithoutAnInStreamForm`,
  `TestLeadResumesInTheModeTheHarnessReports`, `TestLeadWaitWithNoPromptIsShown`.
- Final imports drop `sync` only.

Commit order: (4) `lead_lifecycle_test.go` 1501-2206, (5)
`lead_rejoin_test.go` 781-1499. The test-file and `runner.go` commits are
independent of each other; either target may go first.

## Hazards (read from code at the snapshot)

1. **Package var mutated by tests.** `actionGrace` (`runner.go:3066`, moves to
   `runner_events.go`) is reassigned by `shortActionGrace`
   (`lead_runner_test.go:40-44`, retained) and read at
   `lead_runner_test.go:177`. Same package, so the move is safe; it is a
   plain `var`, not a const, and must stay one.
2. **Package init order.** No `init()` in either target. Package-level vars
   are `mailboxIdentityEnv` (105-107), `errUnknown`/`errClosed`/`errHandedOff`
   (554-556), `actionGrace` (3066): all constant/literal initializers with
   no cross-dependencies, so file placement cannot change init results. Only
   `actionGrace` moves.
3. **Embeds and build tags.** None in either target. Package embeds are
   `builtin.go:116,119` (`instructions/*.md`) and `pi_lead.go:13,16`
   (`*.mjs`); build-tagged files are `claude_settings_owner_unix.go:1`,
   `replace_file_other.go:1`, `replace_file_windows.go:1` (plus the
   `_windows.go` name suffix). Untouched. New names carry no GOOS/GOARCH
   suffix.
4. **Shared test helpers across files.** Listed per destination above
   (`specEnv`, `leadHarness.controls`, `leadHarness.steerHeld`, `piPrompts`,
   `ringKinds`). All stay in `package wsagent` test scope; no visibility
   change. `leadHarness` itself lives in `lead_fake_test.go`.
5. **Source-reading tests: none affected.** `permission_store_test.go:213-260`
   parses Go source with `go/parser`, but only pinned fixtures in
   `testdata/permission-legacy/` and the historical commit `90eacda7e`
   (`record.go`, `children.go`, `replace_file_*`), never `runner.go`.
   `internal/wslive/testdata/historical-summary/wsagent.json:51` pins a
   sha256 of `runner.go`, but for revision `54b252036`'s archive
   (`summary_compat_test.go:121-187` verifies the archive, not the working
   tree). No `runtime.Caller` use under `agents-plugin-tool/`. CI
   (`.github/workflows/ws-mcp-ci.yml`) selects packages, not files.
6. **Pre-existing gofmt drift.** `gofmt -l internal/wsagent` already lists
   `snapshot_idle_test.go` and `wake_test.go` at baseline. Gate `gofmt -l`
   on the touched files only; do not reformat those two.
7. **Soft size rule on new files.** A new file counts as 0 lines at the merge
   base, so a destination over 1000 lines would raise `size_soft_limit`
   (`internal/mcp/git_merge_size.go:177-178,229`). All destinations here are
   <= 901 lines; keep it that way if the map is refreshed.
8. **Comment-only and doc references (note only, do not fix in move
   commits).** `pi_daemon_workflow_test.go:34` says "runner.go state
   projection" (`conv.state` moves to `runner_conv.go`). Live docs citing
   `runner.go:NNN`: `ai-docs/manuals/ws-mcp.md:967,992`; ready tickets
   `261115-feat-ws-permission-profile` (lines 247-899, many),
   `261115-feat-ws-whiteboard-agent-card` (111-338),
   `261115-feat-ws-session-hire-approval` (125-182),
   `261115-feat-ws-worker-profile-session-spawn` (219),
   `261114-feat-ws-live-reasoning-summary` (462, 554); todo/idea tickets
   `261116-feat-ws-daemon-plugin-package-select:72`,
   `261116-research-ws-hosted-session-first-tier:176`,
   `261113-bug-ws-daemon-replace-stops-hosted-sessions:51`,
   `261115-bug-ws-claude-binary-provenance-and-resume-model:44`,
   `261114-bug-native-windows-lead-rejoin-holder-timeout:41`
   (`lead_runner_test.go:1316`). Most are already drifted. `ai-docs/.plans/`
   entries are historical.
9. **Open branch overlap.** `impl/develop/shove-stem-disk` (unmerged, last
   commit 2026-11-15, base `171bf6b26`) edits `runner.go` in `startConv`,
   `State`, `closedState` (retained) and in the `conv` struct, `conv.state`,
   `conv.apply` (moving to `runner_conv.go`). Land, park, or plan to rebase
   it before commit (3). No unmerged branch touches `lead_runner_test.go`.

## Verification gates

Run from `agents-plugin-tool/` with
`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. All work locally on macOS; the
baseline was measured on this machine at the snapshot.

Per move commit:

1. `gofmt -l internal/wsagent/runner*.go internal/wsagent/lead_*_test.go`
   prints nothing for the touched files.
2. Import pruning: loop `go vet ./internal/wsagent`, delete each
   `"X" imported and not used` line it reports, until clean (vet reports
   these one at a time); add a missing import only if vet reports undefined.
3. `go build ./...`, `go vet ./internal/wsagent`,
   `go test -run '^$' ./internal/wsagent` (baseline: all three together
   ~4 s wall).
4. `go test -list '.*' ./internal/wsagent | sort` equals the baseline list
   (463 tests/examples at the snapshot); this catches a lost or duplicated
   test.
5. Narrow sample (baseline 4.1 s wall):
   `go test -count=1 -run '^(TestCreateWritesRecordRegistrationAndSnapshot|TestSendRunsTurnAndHeldSendsGoAsOneTurn|TestRecordSurvivesDaemonRestart|TestSettingsApplyAtTurnBoundary|TestRunnerSuiteOnPi|TestLeadSpawnRunsUnderAHolder|TestLeadApprovalIsAnsweredOnce|TestLeadRejoinsItsProcessAfterADaemonRestart|TestPiLeadRejoinOfAWrappedRingAsksWhetherATurnRuns|TestPiLeadTurnsAndCompact|TestLeadFailedSendShowsTheMessageOnce)$' ./internal/wsagent`
6. Declaration-identity proof (below) against the pre-series base.

Once at the end of the series (and optionally per commit), in the real
worktree, not a /tmp copy:
`go test -count=1 ./internal/wsagent`, then `git status --short`. Baseline at the snapshot: `ok` in
88.9 s (89.4 s wall).

### Declaration-identity proof

Keep tooling outside the repo, e.g. `/tmp/wsagent-split/`:

- `declinv` (small Go program, `go/parser` with `ParseComments`): for each
  top-level declaration except imports, print kind, names (`Type.method` for
  methods, all spec names for grouped const/var/type), and the sha256 of the
  exact source bytes from the doc comment's start to the declaration's end.
- Export the base package once:
  `git archive <base> agents-plugin-tool/internal/wsagent | tar -x -C /tmp/wsagent-split/before`.
- `prove.sh BEFORE_DIR AFTER_DIR` compares, as multisets across all package
  files (ignoring which file holds each): (a) `declinv -hash` rows,
  (b) every comment line (catches floating comments not attached to a
  declaration), (c) every non-blank line outside package/import scaffolding
  (catches any stray edit between declarations). All three must be identical.

Dry run of this exact map at the snapshot: 1638 declarations before and after,
DECLS / COMMENT LINES / BODY LINES identical; `gofmt`, `go build ./...`,
`go vet ./internal/wsagent`, compile-only test, and the `-list` comparison all
passed. Full-package runs in /tmp `git archive` copies matched the unmodified
base copy exactly (identical 121-test failure set when the copy lacked the
sibling `agents-plugin*/` packages; with them, only
`TestPiProbeCacheConfigNormalizationIsConservative` fails in both, from the
macOS `/tmp` -> `/private/tmp` symlink at `pi_probe_cache_test.go:500`).
The real-repo baseline passes, so run the full gate in the real worktree.

Environment notes for the full gate: wsagent tests read sibling packages
(e.g. `claude_partial_test.go:23` reads
`agents-plugin/.claude-plugin/plugin.json`), and in the archive copy a full
run rewrote `agents-plugin*/native-pi/first-party*` payload files whose
bundles it judged stale. In the real worktree they are in sync and the
baseline run left `git status` clean; still check `git status --short` after
the full run and keep any such rewrite out of move commits.

## Ordering relative to other groups

Phase 1 (`impl/develop/server-go-split`) changes 13 files, all under
`internal/mcp/`; it does not touch `internal/wsagent`. `internal/mcp` imports
`wsagent` (`server.go:21`, `wsagent.BackendRegistration` at `server.go:2866`),
but this series changes no exported or unexported identifier, so the two
groups have no textual or API conflict and may run in parallel on separate
branches in either merge order. Phase 1's `go vet ./...` also compiles
`wsagent`; a red wsagent build there would come from this series only if a
move commit were broken.

## Lead decisions

- Whether to accept a retained `runner.go` of 1287 lines (under 2000, no
  advisory) or add the optional fourth `runner_requests.go` move.
- Disposition of `impl/develop/shove-stem-disk` before commit (3).
- Whether the ~89 s full package test runs per commit or once at the end.
