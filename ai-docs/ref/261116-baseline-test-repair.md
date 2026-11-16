# Baseline test repair - 2026-11-16

## Scope and verdict

Owner-authorized ad-hoc repair on `impl/develop/baseline-test-repair`, initially at develop `4feda9b3b`. Integrated develop `0b1d06db8`, then the iframe-cache work through `62b168e34`, and finally the column-grab work through `cf0dcc9a0` plus documentation `9277525d0` before the final run. No merge into develop or push.

All ten originally reported baseline failures and the requested topic-rail motion failure are repaired. Own changes are tests/fixtures and source-faithful generated Pi guidance, not dashboard CSS, JS, templates or runtime Go behavior. Incoming develop visual changes were integrated as requested, not modified by the repair.

The **unfiltered full Go suite is not green**: precisely two unchanged tests still require Node, which is not installed on w5090. Their guards were not weakened or skipped. The owner accepted these two environment-only reds on 2026-11-16 at 17:58.

## Diagnosis by failure

| Failure | Classification and introducing drift | Repair |
|---|---|---|
| `TestCSSClassesDoNotCollideAcrossModules` (`.board-updated`, `.oq-pill`, `.dw`) | Stale ownership assertion after `fe8eda6e8` deliberately added cross-module animation-pausing and containment-condition selectors. | Recognize only the exact intended selectors in their exact rules; keep other occurrences in the collision scan. |
| `TestDependencyPopoverCSSScope` | Stale section boundary after `fe8eda6e8` inserted unrelated clock styles. | End the popover-only scan at the new section boundary; preserve scope checks. |
| `TestPagesCarryTheTracker` | Stale inline-script expectation after `c658c4798` externalized session-frame dashboard scripts. The index remained inline. | Explicitly check each page's intended mode, exact assembled asset/source and script ordering; keep renderer/CSS/string hooks in HTML and adapter hooks in the assembled script. |
| `TestRefreshCacheNonBoardWake` | Stale skip-counter expectation after `fcf912cd6` added the two-second shared stamp memo. | Check shared hits and no stamp/page reads within the memo, explicit expiry/revalidation, and full reads on semantic wakes. |
| `TestPagesCarryTheToastStackAndTitleHost` | Same intentional script externalization in `c658c4798`. | Check the title adapter in the correct assembled script while retaining inline toast, tracker and title-host assertions. |
| `TestConfigListAgentProfileKnob` | Stale builtin count after `d609726e2` added `explore`. | Pin all nine builtin names/provenance plus the repo profile, including the previously unlisted lead names. |
| `TestHostedConnectionSeeingALANChangeReplacesTheDaemon` | Fixture exit assertion originated in `1d4691d9c`; helper migrated in `117826a44`. Remote evidence showed an exited Linux zombie, not a running predecessor. | Test-only Linux terminal-state predicate; preserve the existing timeout, non-Linux behavior and production `ProcessAlive`. Add parser boundary cases and bounded failure-only diagnostics. |
| `TestLauncherRunsMailboxWaitThroughTheDaemonLikeTheDirectCLI` | Stale fixture after `ab1460d41` added the resource sentinel wait and `45be54fbb` stopped CLI/inspection commands from starting daemons. | Stage the resource sentinel and explicitly start the mailbox fixture daemon. Keep the separate cold serve/bootstrap test cold. |
| `TestLauncherMailboxStreamEndsWhenPOSIXParentIsKilled` | Same launcher fixture drift. | Explicit daemon setup; retain parent-death, EOF and marker-cleanup assertions. |
| `TestLauncherMailboxStreamSurvivesArrivalsAndTimeoutThenSignal` | Same launcher fixture drift. | Explicit daemon setup; retain reminder, added-mail, signal and marker-cleanup assertions. |

Additional failures exposed by the required current-tree full run:

- `TestTopicRailBarCapsuleMotion`: stale keyframe assertions after `fe8eda6e8` deliberately moved from animated layout properties to destination geometry plus transform animation. Pin that implementation and reject layout-property keyframes; do not alter motion code.
- `TestWhiteboardStreamMetadataSharesFileAndFolderAuthority`: stale rendering assumption after `f057d07aa` deliberately omitted redundant panel-post system events from the view while retaining them in the store/query. Independently verify both literal fixture events remain stored and unrendered; retain every other locator, attribution and immutable panel-version anchor.
- `TestPiPreparedSourceDriftWithoutRebuiltBundleUsesBridge` and `TestPiPreparedPackageOnlyLayouts`: a **generated-artifact regression**, not stale assertions. `319393501` changed the canonical Pi lead guide without refreshing prepared first-party copies. Regenerated only the guide and first-party manifest in both products using the official producer's generation and validation logic, with a bounded publish adapter that avoided rewriting unchanged closures/bundles. Both payload/source checks passed. No bundle rebuild, npm/install or launcher invocation; no schema/key change (T0 artifact synchronization).

## Verification

All Go execution was remote on w5090 through `D:/sys_lock.sh remote`; no local Go tests/builds, test executables, ws executables or launchers ran.

- Diagnostic job `2c017cb1b5234d54a00f0925a50c73e4`: all three mailbox launcher tests passed after fixture repair. The predecessor exit failure captured `/proc/<pid>/status` as `State: Z (zombie)`, `FDSize: 0`, one thread, proving the remote supervisor had not reaped an already-exited process.
- Final job `8d2856cab0f3449ca0c77891cd2311cd`, after current develop integration, ran:

```sh
bash D:/sys_lock.sh remote w5090 run exec10 25 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./... -count=1'
```

The command used no filters or exclusions. `cmd/ws-mcp` passed (134.042 s), `internal/mcp` passed (182.969 s), `internal/wsagent` passed (96.203 s), and `internal/wsdashboard` passed (202.981 s). Every other tested package passed except `internal/wswhiteboard`, whose only failures were:

1. `TestTopicStateBadgeResearchingMetadata` - `node required for research badge contract`, `topic_state_badge_test.go:52`.
2. `TestTopicStateBadgeClockAndReconciliation` - `node required for badge clock contract`, `topic_state_badge_test.go:112`.

These tests were introduced by `94406db99`; their source is untouched. The final remote prerequisite check explicitly confirmed that `node` is not installed. No installation was attempted. The full command therefore exited 1; no all-green claim is made.

## Late child-report settlement - 2026-11-16 17:58

The delayed child message was stale, not a new uncommitted change. The Linux zombie-aware predicate, parser cases and bounded diagnostics in `daemon_proxy_test.go` were already committed in `533e115bd` and included in `a26a1b934`; the worktree had no pending source diff. The fix remains necessary for the observed exited-but-unreaped Linux predecessor.

Re-ran its entire package once on w5090 as requested: `go test ./cmd/ws-mcp/... -count=1` passed in 144.513 s, job `34f305d0e7594f5596e9d18f540d8cde`. No duplicate source commit or discard was needed. All nine no-longer-needed idle child agents were stopped.

## Review and Forward

Independent R1 and a supplemental R1 limited to the newly discovered full-suite repairs were clear on static review. No substantive finding required a fixes-only R2.

- Forward (minor R1): the memo fixture still uses the production two-second real-clock window and could flake if its goroutine is descheduled for longer than that window. No new clock abstraction was added in this bounded repair.
- Forward (owner accepted 2026-11-16 17:58): `TestTopicStateBadgeResearchingMetadata` and `TestTopicStateBadgeClockAndReconciliation` remain red solely because w5090 lacks Node; provide that verifier prerequisite before rerunning them, without weakening their guards.
- Worker-created temporary scripts, messages and review reports under `D:/Temp/baseline-test-repair/` are removed at handoff after preserving conclusions here.
