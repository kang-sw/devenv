# Phase 1 payload and render audit

## Method

Reused scenario (a) and the generated board of `scripts/board-perf.mjs` /
`internal/mcp/TestBoardPerfServe`, not a checked-in board snapshot. The new
`scripts/session-payload.mjs` drives cached headless Chromium against that
isolated fixture. No shared daemon, SSH, Electron, push or merge was used.

Baseline production source: `f66857935`. Both binaries use the same updated
fixture helper: topic bodies reserve panel IDs, the browser receives a temporary
login link, and framed-audit authoring uses a fixed clock. This repairs stale
fixture assumptions without changing baseline production behavior. The shell
owns the actual frame; navigating directly to `?frame=1` would redirect back to
the shell and is not an adequate browser test.

Five handler samples are timed inside `Handler().ServeHTTP` with an authenticated
request and recorder (no network/browser timing included). Samples 1–2 include
cold render/memo stabilization; the last three are warm. HTML bytes below are
from the browser's authenticated framed-document response. Handler response sizes
are also retained in JSON; they differ slightly because the git badge settles
later. All numbers are raw/uncompressed. This is one sequential observation per
side, not an interleaved latency benchmark; timing is noisy and has no pass target.

## Results

| Generated shape (open,closed,panels,versions,logs) | Before HTML | After HTML | First handler ms before/after | Warm median ms before/after |
|---|---:|---:|---:|---:|
| Default history-heavy `8,6,4,2,60` | 2,944,449 B | 985,210 B | 217.880 / 159.659 | 16.278 / 2.809 |
| 32-topic `24,8,0,0,0` | 1,991,993 B | 252,920 B | 82.751 / 82.846 | 3.116 / 1.174 |

The 32-topic fixture meets the 0.3 MB target. The history-heavy default **does
not**; its remaining eager panel versions and timeline rows belong to Phase 2.
Do not generalize the 32-topic result to history-heavy boards or claim the default
target passed. The same two shapes should be retained for Phase 2 comparison.

The prerequisite bundle is 1,045,730 B; shared CSS is 91,631 + 393,708 B;
the English page table is 80,660 B; Web/SelectAll are 22,921 / 3,843 B; the main
script is 628,344 B. Together with the existing topic CSS/JS and the new 2,465 B
public topic table, the unique referenced assets total 2,415,009 B, versus
773,635 B before. Every listed asset returns 200 with immutable one-year caching;
JSON retains each URL, MIME type and length. This trades additional cold static
requests for smaller documents and reusable assets, not zero-byte cold loads.
The subset stops repeating in every topic document; sprites deliberately remain
inline for same-document icon references. Optional terminal resources are covered
by route/order tests, not by this no-terminal fixture's byte total.

### Stream baseline and recovery

Before: initial WebSocket traffic includes one full board region of 1,736,551 B,
after the board was already present in the framed HTML. Thus the code reading
that predicted a duplicate, not the previous hidden-cache audit claim, was right.
After: no first board event. The next board change is `board-topics` (110,575 B
on the default shape); the browser's board hash advances, with zero reconnects
and no page errors. Both transports have focused held-current, missing-hash,
stale-hash and next-delta tests. Node contracts cover page-seeded and hidden-update
canonical baselines, reconnect accumulation, invalid-patch recovery and immutable
snapshots. Row-region first-send semantics are unchanged.

### Visual comparison

Compared [after.png](after.png) vs [before.png](before.png): **match**, zero
differing RGB pixels out of 1,600,000. Inspected [side-by-side.png](side-by-side.png)
as well. Captures show the framed document inside its real shell at 1600×1000,
including topic panel bodies and diff controls. Only dynamic shell header/rail
and `<time>` fields are masked; board timestamps are deterministic fixture data.
No CSS/DOM layout or visible content change is intended or observed.

## Verification

Every Go invocation used `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Browser
invocations used `PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright`
and `WS_PLAYWRIGHT_MODULE` set to a cached npx Playwright package. No downloads.

- Baseline: `TestDashboardServedPageRendersTopicBodyInAFrame`,
  `TestDashboardReadClosedTopicsPastTheCap`, topic-cache and unchanged-Seq
  invalidation tests all passed; the ticket's named historical reds were green.
- Scoped dashboard asset/order tests passed: all 11 languages, immutable bytes,
  native-cookie and LAN-token authorization, cookieless fixed frame GET/HEAD,
  wrong hash, POST and nested-route negative checks, terminal/Web ordering,
  tracker/toast/SelectAll placement, fresh-session Node tests.
- Scoped page, shell, stream, asset-ID and dev-source-fragment tests passed;
  `TestBoardTopicsStreams` and `TestFramedSessionAssets` passed under `-race`.
- Scoped MCP page/watch/contact/mail/language/reask/memo/topic-cache tests passed,
  including metadata/log/panel edits at unchanged Seq and full-render equality.
- Full `internal/wswhiteboard` and `internal/wsi18n` tests passed. The leaf's
  revision/cache and subset tests also passed under the race detector.
- Cached Chromium `TestScopedSelectAllBrowser` and `TestPanelDiffBrowser` passed
  after the final subset change (standalone/dashboard/preview/shell selection;
  light/dark diff marks, hover/pin/keyboard/compare/prior/live-version behavior
  and sandboxed topic frames).
- Before/after `scripts/session-payload.mjs` runs passed on both shapes. After
  runs used `--expect-skip 1`; both applied a real update without reconnecting.
- Independent review found one Important stale inline-subset test assertion;
  it was fixed without dropping negative checks. The fixes-only second round
  independently confirmed the test, labeled measurements and pixel comparison.
  No Critical/Important finding remains. `server.go` was not edited.
- `git diff --check` passed.

### Reproduction

Build only the test binary, then run the narrow opt-in fixture through the harness:

```sh
cd agents-plugin-tool
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -c -o "$OUT/after.test" ./internal/mcp
cd ..
PLAYWRIGHT_BROWSERS_PATH="$HOME/Library/Caches/ms-playwright" \
WS_PLAYWRIGHT_MODULE="$CACHED_PLAYWRIGHT" \
node scripts/session-payload.mjs --bin "$OUT/after.test" --out "$OUT/after" --expect-skip 1
```

Use `--shape 24,8,0,0,0` for the 32-topic case. The baseline binary is built
from the baseline production tree with the same `board_perf_serve_test.go` helper;
omit `--expect-skip` for it. Raw reports: [before.json](before.json),
[after.json](after.json), [before-32-topics.json](before-32-topics.json),
[after-32-topics.json](after-32-topics.json). No HTML, credential, process ID or
machine-specific absolute path is stored in these artifacts.

## Omitted / limitations

Phase 2, whole MCP/dashboard-package runs and real Electron/LAN-device launches
were deliberately omitted. Native cookie and LAN token behavior is verified at
the actual HTTP guard; Electron itself was never launched. Route/workflow-manual,
acquire and review-render tools were not exposed to this worker; a conservative
independent single-review allocation with a fixes-only second round was used.
