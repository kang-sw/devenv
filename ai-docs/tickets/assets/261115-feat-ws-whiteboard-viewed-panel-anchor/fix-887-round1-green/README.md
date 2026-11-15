# Viewed cross-topic insertion — bounded RED/GREEN round, 2026-11-15

## Authorized delta

The base is `fe19766cf5d21c49535f36edd369d5223694af0e`. Only
`internal/wsdashboard/board_viewed_test.go` and `assets/dashboard.js` under the
same package changed. `source.patch` is the complete frozen two-file delta.
The dashboard change is byte-for-byte the lead-approved
`../fix-887-round1-red/product-proposal.diff`; the RED test stayed unchanged.
No shared harness/helper, other product code, scroll writer, protocol, sandbox,
geometry policy, owner-input exception or release policy was changed.

The existing blocker compares the canonical span of a changed topic across the
exposed interval of its actual scroller, extending toward exposed units in other
topics and excluding units owned by independent nested scrollers. Existing
admission and pending-release paths consume the blockers. This addresses the
reported cross-topic insertion gap without revising the ticket contract; fresh
independent review remains pending at this checkpoint.

## RED proof before product editing

The lead audited the complete retained `../fix-887-round1-red/` evidence before
clearing the product patch and GREEN batch. Only the new test was dirty then;
dashboard bytes matched the base before and after the single RED command.
`TestBoardViewedCrossTopicInsertion` actually ran, exited 1, and failed only at
`cross-topic insertion must be staged before reference-B correction`. The fixture
checks preceding it passed: unchanged A remains exposed by one pixel, B is the
reading reference, and A gains only a new 40px panel after its existing panel.
Assertions after that staging check were not reached in RED. Total supervised
RED time was 17.539 seconds; its complete raw output and cleanup were read.

## One GREEN batch

The supervisor froze `source.patch`, `source-hashes.json` and `freeze.json` before
launching the first Go child, verified the exact approved product bytes and
frozen test patch, and rejected any other tracked source change. Exact commands,
from `/Users/kang-sw/.ws-worktrees/devenv/symptom-sandbox-swan/agents-plugin-tool`:

```sh
/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go test ./internal/wswhiteboard -count=1 -timeout=90s -run '^(TestViewKitUnderNode|TestViewKitVisibilityUnderNode|TestViewKitJumpFollowUnderNode|TestHistoryScriptPanelChunksAndSwapUnderNode)$' -v
/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go test ./internal/wsdashboard -count=1 -timeout=90s -run '^(TestBoardKeep(Wiring|UnderNode)|TestBoardFrameHeightsBatch|TestBoardFramesMountLazily|TestBoardTopic(Motion|RevealUsesFoldOwner|Pins|ResizeKeepsTheView)|TestBoardTopics(DoNotSkipRendering|ClientSnapshots|ClientHiddenAndRecovery|Streams)|TestBoardViewed(RenderingAndRelease|PlacementAndQueuedCounts|InternalGeometryAndHeightRelease|FetchedPageTransfer|BoundaryAndBatchWiring|CrossTopicInsertion))$' -v
```

VERIFIED: all expected 22 tests actually ran and passed (4 whiteboard, 18
dashboard), with no skips. Both commands exited 0. Complete raw outputs were
read: package times 1.138 seconds and 11.677 seconds; whole batch 30.296 seconds
including cleanup. There was no retry or separate build/vet/browser invocation;
Go's normal compile/vet work within the approved commands remains in the receipt.

The complete explicit environment is in `manifest.json` and `freeze.json`: no
inherited caller variables; fixed Go 1.26.2 and cached Node; private HOME/auth/XDG,
GOPATH/build-cache/temp directories; existing fixed module cache; offline
GOTOOLCHAIN/GOPROXY/GOSUMDB/GOWORK/GOENV settings and `GOFLAGS=-mod=readonly`.
Readonly module mode is not a newly added OS sandbox or network firewall.
Caps remained 120 seconds per command, 90-second Go timeouts and 300 seconds
for the batch including 30 seconds reserved for cleanup. No cap expired.

`cleanup.json` was read in full. Both per-command and final scoped process lists,
errors and actions are empty. No unexpected provider process was detected in
scoped observations; this is not a global negative audit or proof of complete
runtime isolation. The 64 prior untracked entries plus the complete RED directory
were verified present. No previous artifact was deleted or bulk-staged.

## Bindings and limits

- Dashboard SHA-256: `aa5641fc2a42e1f96057dc9957ee49b022c6087f0c35893844bee32efd55de55`.
- Unchanged RED test SHA-256: `5afedbed392ecb77dbdc6bdffb54162a2bcbea430f34068938c03b415f90f73b`.
- Complete two-file patch SHA-256: `c9dd00887cc855387685bbb548128b858a43cb2cb92885dee824a70dabba1773`.

Full raw logs: `whiteboard.txt`, `dashboard.txt`; full command/pass sets and
cleanup: `manifest.json`, `cleanup.json`; pre-start bindings: `freeze.json`,
`source-hashes.json`, `source.patch`. Supervisors, binaries and private caches
remain local and are excluded from the explicit evidence commit.

Browser geometry: not run (setup positioning precondition failed at 004, 32px residual; not a product finding)

The earlier Critical report and this RED/GREEN proof go to one fresh independent
Lite reviewer. MAIN's later owner policy (899)=a supersedes the earlier remaining
step gates: Minor findings become follow-ups; only Critical/Important findings
block, with one additional bounded contract-preserving fix round and relevant
`-run` tests pre-granted. The lead skims the diff and owns one sync of `874280de2`
immediately before head handoff; conflict-free sync needs no acceptance re-run.
No extra lead-facing raw-log/manifest/cleanup/byte-equality audit, broad suite,
Windows run, browser, screenshot, worker sync, shared-runtime action, Phase Result
or closure is included. Owner (887)'s not-run/no-005 boundary remains unchanged.
