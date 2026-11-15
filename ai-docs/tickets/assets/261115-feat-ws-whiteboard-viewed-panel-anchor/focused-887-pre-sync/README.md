# Owner-887 focused pre-sync evidence — 2026-11-15

## Scope and outcome

VERIFIED: The single lead-approved pre-sync focused batch ran against
`d040b74164bebc463d9d473ee5f78230fc6535c5` on
`impl/develop/bling-fog-scorn`. All expected 21 tests actually ran and passed:
four whiteboard unit/history tests and seventeen dashboard tests, including the
private SSE/WebSocket transport integration. No test skipped; neither selector
matched a browser or server-only browser fixture. This is current pre-sync
focused evidence, not a reclassification of the older checkpoint.

Both commands exited 0. Complete raw outputs were read: whiteboard reported
`ok` in 1.084 seconds, dashboard in 11.502 seconds. The whole supervised batch
finished in 27.748 seconds including cleanup. No cap expired. No separate build,
vet, syntax check, browser invocation, retry, dependency install or product/harness
repair ran; Go's normal compile/vet work within these exact `go test` commands is
visible in the process receipt.

## Exact execution

The outer command was:

```sh
python3 ai-docs/tickets/assets/261115-feat-ws-whiteboard-viewed-panel-anchor/focused-887-pre-sync/supervise.py
```

Both child commands used cwd
`/Users/kang-sw/.ws-worktrees/devenv/symptom-sandbox-swan/agents-plugin-tool`:

```sh
/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go test ./internal/wswhiteboard -count=1 -timeout=90s -run '^(TestViewKitUnderNode|TestViewKitVisibilityUnderNode|TestViewKitJumpFollowUnderNode|TestHistoryScriptPanelChunksAndSwapUnderNode)$' -v
/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go test ./internal/wsdashboard -count=1 -timeout=90s -run '^(TestBoardKeep(Wiring|UnderNode)|TestBoardFrameHeightsBatch|TestBoardFramesMountLazily|TestBoardTopic(Motion|RevealUsesFoldOwner|Pins|ResizeKeepsTheView)|TestBoardTopics(DoNotSkipRendering|ClientSnapshots|ClientHiddenAndRecovery|Streams)|TestBoardViewed(RenderingAndRelease|PlacementAndQueuedCounts|InternalGeometryAndHeightRelease|FetchedPageTransfer|BoundaryAndBatchWiring))$' -v
```

The supervisor supplied a constructed environment without inheriting caller
variables (the `env -i` equivalent). `manifest.json` records its complete value:
fixed Go 1.26.2/GOROOT, cached Node, `GOTOOLCHAIN=local`, `GOPROXY=off`,
`GOSUMDB=off`, `GOWORK=off`, `GOENV=off`, and `GOFLAGS=-mod=readonly`; fixed
`GOMODCACHE=/Users/kang-sw/go/pkg/mod`; fresh batch-owned HOME, CODEX_HOME,
CLAUDE_CONFIG_DIR, XDG directories, GOPATH, GOCACHE, TMPDIR and GOTMPDIR. Readonly
module mode does not mean that a new OS sandbox or network firewall was added.
The supervisor itself remains retained locally, untracked; its SHA-256 is in
`source-hashes.json`. Binaries and private caches are not committed.

Limits were 120 seconds per command, each Go test timeout 90 seconds, and
300 seconds overall including a 30-second cleanup reserve. Commands ran serially
in separate owned process groups. PID/start identities and descendants were
tracked; the expected test-name/pass/skip sets were checked before progression.

## Evidence

- `whiteboard.txt`: complete four-test raw output, exit 0.
- `dashboard.txt`: complete seventeen-test raw output, exit 0.
- `manifest.json`: exact argv, cwd, isolated environment, tool/source bindings,
  expected and actually run/pass sets, exits, timings, and protected-path inventory.
- `source-hashes.json`: source pin, module Git tree and SHA-256 of relevant
  source/test/helper files; these bytes were checked unchanged during the batch.
- `cleanup.json`: complete tracked owned identities and independent final scoped
  process observation. `final_ps`, `errors` and `actions` are all empty. Both
  per-command final scoped process lists are also empty. The complete receipt
  was read. No unexpected provider process was detected in the scoped observations;
  this is not a global negative audit or proof of complete runtime isolation.

The supervisor verified that all 61 previously protected untracked status entries
remained present. It did not delete old evidence or claim an exhaustive content
audit of those archives. Source pin and branch were unchanged. Only the authorized
ticket appendix was dirty during test execution.

## Landing boundary

Browser geometry: not run (setup positioning precondition failed at 004, 32px residual; not a product finding)

Existing evidence remains under `../browser-approved/approved-004/`, particularly
`results.json:345-382`, `driver.cjs:138-150`, `browser.txt`, `manifest.json`,
`cleanup.json` and `browser-cleanup.json`. No 005 or other browser run occurred.

Fresh independent one-pass lite review remains the next authorized step. No
product repair is authorized if findings arise. The lead alone owns merging
`55b50bd53` into the retained impl branch and issuing the next focused re-run gate;
this batch authorizes no sync, repin, post-sync run, Phase Result or closure.
MAIN owns the merge/runtime swap, followed by the owner's on-screen verification.
