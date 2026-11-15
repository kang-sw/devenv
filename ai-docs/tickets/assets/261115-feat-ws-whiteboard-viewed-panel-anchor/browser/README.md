# Offline browser leaf: STOP / UNVERIFIED checkpoint

## Pins and boundary

- Product tree: `48764f9374d579ab1dbc487e4b99c57704007d61`, including
  `2f8f34a82` and `ce6ad8116`.
- Comparison baseline: `72df121fcadb5e4e027b84c15474e402f9adfc34`.
- Branch: `impl/develop/bling-fog-scorn`; no checkout/ref changes.
- Only two new test files and this evidence directory were changed. No product,
  existing test, ticket, provider, holder, installed daemon or shared endpoint
  was modified. No install/download, merge, push, deployment or broad suite.

## Result and blame

**VERIFIED:** the opt-in Go test compiles, cached Chromium starts, and the
production-rendered disposable daemon responds. Both browser attempts failed
before any current-product geometry predicate ran. This is an unresolved
**fixture/interception failure**, not evidence of a product defect or pass.

1. `run-1.log`: exact inline replacement could not find current `stream.js`
   (`0 !== 1`). An unhandled Playwright route rejection bypassed Node's outer
   cleanup, although independent supervisor cleanup evidence shows every tracked
   owned PID absent. Route rejection handling was subsequently contained.
2. Browserless diagnostic runs (temporary instrumentation, removed afterward)
   found all four current assets exactly once in production HTML, including
   after sequential replacement. A Node native-fetch diagnostic likewise found
   all four exactly once. These checks establish source-byte availability, not
   browser acceptance. See `preflight.log`.
3. `run-2.log`: startup timed out after 8 seconds. `results.json` records one
   successful baseline document substitution, while `asset-mismatch.json`
   records another interception where even the `stream.js` prefix is absent.
   `failure.png` is the blank 1800px page and `failure.json` has no panels.
   The precise reason for that additional intercepted document is unresolved.

**INFERRED:** the recurrent baseline interception failure explains the second
startup failure. To honor the assignment's second-same-root stop rule, no third
browser attempt or speculative fix was made. The test remains opt-in and
UNVERIFIED. The parent must diagnose the intercepted document before resuming;
no current predicate was weakened and no baseline limitation was waived.

## Commands and caps

Working directory: `agents-plugin-tool/`.

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
WS_PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright \
WS_VIEWED_CAPTURE_DIR=<absolute path to this browser directory> \
go test ./internal/wsdashboard -count=1 -timeout=150s -run '^TestBoardViewedBrowser$' -v
```

- Run 1: Go package FAIL in 1.624s; outer elapsed 4.555s.
- Run 2: Go package FAIL in 9.704s; outer elapsed 11.092s.
- Replay supervisor: `python3 <this-directory>/supervise.py <new-run-label>`.
  It records exact expanded environment, command and process observations.
- Outer process-tree supervisor: 180s; Go test: 150s; Node child context: 125s;
  Node self-cap: 120s; default browser action/navigation: 8s. No cap fired.
- `gofmt`, `node --check .../viewed-panels.browser.cjs`, and `git diff --check`
  completed without errors. They are not behavioral acceptance.
- The three temporary browserless diagnostic invocations used the same targeted
  Go command with `-timeout=20s` and `WS_VIEWED_PREFLIGHT=1`, under a 30s tool cap;
  all returned PASS. That diagnostic switch was removed, and these were not
  browser passes. Their sole temporary Node subprocess used native fetch.

## Lifecycle and network evidence

The fixture uses `startPageFixture` with temporary cache/project directories,
plus a second ephemeral `httptest` endpoint for immutable rendered snapshots and
baseline asset bytes. Browser HTTP is allowlisted to those two exact origins;
all other requests are aborted. The production session stream receives full
`region` events through a browser-local WebSocket shim, with no network socket
or provider. Sandbox-height probes, if reached, post from registered child
frames without reading their DOM.

`launchServer` supplies the owned Chromium PID. Node and the independent outer
supervisor both sample descendants while live. Normal/signal cleanup closes
browser then browser server. The outer cap would signal only its owned group
and recorded descendants. No timeout signals were needed.

- `supervisor-1.json`: 10 tracked PIDs, including browser root/children;
  independent final `ps`: empty.
- `supervisor-2.json`: 12 tracked PIDs; independent final `ps`: empty.
- `browser-cleanup.json`: run 2 Node-side browser closure, all four tracked
  browser PIDs absent, no cleanup errors.
- `final-cleanup.json`: a further independent union-of-recorded-PIDs check.

## Unfinished scope

No matching baseline/current before/after acceptance screenshots or geometry at
1800px/420px were obtained. The only screenshot is a startup-failure capture,
not visual approval. No approved visual design exists.

The new script contains bounded scenarios for queued A/B changes, shared-bar
counts/click, exact 1px render deferral, changed-panel release with an unchanged
visible sibling, offscreen iframe growth on main/pins, visible height holding,
fetched closed-page transfer, local memo/receipt isolation, disappearing
reference, nested transactions, wheel cancellation/cooldown and a reduced-motion
late jump. **None ran far enough to establish their current predicates.**
Fixture geometry preconditions and selectors beyond startup remain unverified.
Additional explicit coverage still needed includes pinned relative reorders,
non-reading-line deferral as a separately asserted case, all input modalities,
momentum-specific real browser behavior, sticky-clearance selection, complete
history/focus/frame-origin boundaries, and per-visible-panel geometry during
automatic release. Full virtual timing coverage belongs to the parent's
existing deterministic tests, which this leaf did not rerun or reclassify.

This checkpoint is neither a review nor a phase completion record.
