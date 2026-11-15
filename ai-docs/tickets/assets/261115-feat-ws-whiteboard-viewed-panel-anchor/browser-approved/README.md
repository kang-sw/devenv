# Approved paired fixture — STATIC AUTHORING ONLY

## Status and authority

**VERIFIED (file authoring/static reads only):** The fixture has been rewritten
as a server-only `TestBoardViewedFixture`, a single paired browser driver, and
`run.py`. No build, test, syntax check, formatter, browserless diagnostic,
process probe, or browser run was executed during this continuation. Executable
verification, including formatting, remains pending the parent's ONE approved
batch. The prior `browser/` evidence subtree has not been edited.

Parent-reported integrated source merge: `7589b5910`, importing exactly
`5585844a794a031a770ce4171c5e05daf1bd3353`. Strict comparison baseline:
**`5585844a794a031a770ce4171c5e05daf1bd3353`**, never an earlier feature frontier.
Parent-reported product checkpoint: `2a2a6d73dd3d93e21d731793c30a20db01c6367a`
(initial visible hydration exception only for `wbFirst && !wbKeep && !height`).
Its deterministic regressions are deliberately unrun. The final result pin is
NOT hardcoded here: the parent freezes a full commit after reading the authored
fixture/driver/runner completely. MAIN's advisory `088ecf679` is a future
lead-owned integration interaction; this leaf does not import or repin it.

## Exact invocation design — DO NOT START WITHOUT PARENT AUTHORIZATION

From the retained repository, after the parent's full static inspection/freeze:

```sh
python3 ai-docs/tickets/assets/261115-feat-ws-whiteboard-viewed-panel-anchor/browser-approved/run.py \
  --result <FULL_40_HEX_FROZEN_RESULT_COMMIT> \
  --batch approved-001
```

The batch name must not already exist. There is no retry or overwrite flag.
The command itself is the one batch, not a preliminary preflight. Do not execute
syntax checks, formatting, focused tests, or browserless probes before it.

Before start, the parent reports the integrated merge head, final full result
pin, static fixture/source-manifest plan, cached module, exact invocation,
bounds, process ownership and initial-hydration predicate. This README is not
that approval/start report.

## Pinned inputs and retained artifacts

1. `git show <result>:<fixture/driver/runner>` saves exact frozen authored bytes.
   The runner refuses a working runner differing from its frozen bytes.
2. `git archive <baseline> agents-plugin-tool` and the matching full result
   archive construct disposable complete tooling-source snapshots. No checkout,
   worktree, branch or ref operation occurs. Identical frozen fixture Go and
   driver bytes are overlaid into both snapshots; no product source is overlaid.
3. Each source snapshot gets a full per-file SHA-256/size manifest, explicitly
   identifying asset/template files, plus the original archive digest and an
   immutable exact-build-input archive. Snapshot files become read-only. Every
   source digest is checked again after its build. Both snapshots and archives
   are retained; nothing is deleted on failure.
4. Each snapshot independently runs:
   `go test -c ./internal/wsdashboard -o <absolute retained binary>`.
   The compiled binaries and their SHA-256 hashes are retained. Builds do not
   execute tests. Offline environment is explicit: `GOTOOLCHAIN=local`,
   `GOPROXY=off`, `GOSUMDB=off`, `GOWORK=off`, `GOENV=off`; caller WS/provider/dev
   settings are not inherited.
5. Both binaries run only `^TestBoardViewedFixture$` with their own source pin,
   atomic ready-file path, and stop-file path. Each fixture uses test-owned
   temporary cache/project directories, `Port: 0`, loopback binding, no agents,
   holders, provider, installed daemon or shared endpoint. Ready JSON records
   both exact origins, PID, complete embedded asset ID and source pin.
6. ONE Node driver consumes both ready files. Every page, template and hashed
   frame asset comes from its own compiled snapshot. There is no asset-text
   substitution, mixed-version template, or special four-file interception.
   Normal `/s/livesession` shell navigation is followed by
   `iframe.sframe.on.contentFrame()`. All product APIs, readiness, state delivery
   and geometry use that session content frame; screenshots use its outer page.
7. Raw subprocess stdout/stderr lives in per-step `.txt` files. `manifest.json`
   records exact commands, environment, pins, phase results and hashes.
   Document request/response ledgers include URL, resource type, frame identity,
   status, headers, data-assets identity and trusted shell/session body prefix.
   Failures retain the last/failing request and response, exception, available
   trusted-session geometry and screenshot. Route errors are latched and raced
   against waits so they are not replaced by generic readiness timeouts.

## Bounds, lifecycle and first-failure rule

- Overall work plus cleanup: **600 seconds**, starting before source extraction.
- Work deadline: 570 seconds; final 30 seconds reserved for cleanup. A main-thread
  timer also covers Python archive extraction/hashing, not just subprocess waits.
- Each archive command: at most 20 seconds; frozen-source reads: 10 seconds each.
- Each `go test -c`: **at most 180 seconds**; builds are serial.
- Each fixture readiness: 15 seconds; fixture lifetime 240 seconds; binary test
  timeout 245 seconds. Fixtures start only after both builds succeed.
- Browser subprocess: **at most 180 seconds**, also clipped by remaining overall
  work budget. Node self-cap 160 seconds leaves diagnostic/cleanup room. Browser
  launch/connect, navigation, default action and readiness: 8 seconds. Every
  positioning loop is finite (4 or 5 iterations).
- Caps are maxima, not an additive promise of 180 seconds for every phase when
  the overall budget is already consumed. On the first failure/cap/early fixture
  exit, progression stops. No second browser launch, repair or rerun exists.
- Cached module only:
  `/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright`.
  Chromium must already be cached; no installation/download fallback exists.
- All browser HTTP is restricted to the union of the four exact fixture origins.
  A browser-local WebSocket shim delivers production full-region events without
  a real socket/provider. Local message POSTs get a fixture-only receipt response.
- Node `launchServer` supplies the owned Chromium root. Node and outer Python
  record PID + process start identity and descendants while live, including the
  detached browser tree. Node publishes an atomic live browser ownership file.
- Cleanup first writes fixture stop files, then waits, TERM and KILL only owned
  matching identities. Browser/context/server closure is bounded. Fresh final
  `ps` observations record every surviving matching identity independently of
  parent exit; no shared process, port or daemon is signaled. Logs, snapshots,
  binaries, partial manifests and cleanup evidence survive failure.

## Authored predicates (ALL UNRUN)

At 1800px and 420px, identical paired baseline/result queue captures measure
visible A/B, multiple queued external updates, EVERY exposed panel's position,
canonical frame payloads, and the same bottom Korean count/names/click bar.
Baseline feature differences are observations only; baseline availability,
fixture geometry and hydration preconditions are still hard assertions.
No baseline observation waives a result predicate.

Result-only bounded scenarios cover:

- Normal first visible content hydrates with real positive iframe heights and
  **no pending bar**. The driver never clicks an initial pending bar to proceed.
- Oversized p1 enters through production `loadTarget`, with the real history
  budget/header/chunks retained. Its admission jump is canceled/settled before
  exposure tests. `positionHistory` measures `min(100, clipTop - topicTop)` rather
  than assuming 100px. No history clip CSS is overridden.
- Exact 1px exposure (including sticky/toolbar/clip clearance), off-reading-line
  B rendering, multiple queues and explicit click. Changed-panel automatic
  release keeps an unchanged sibling exposed and compares EVERY remaining
  exposed panel. Setup uses an owner write, not an active-user cooldown loophole.
- Independent 1800px pins and inline 420px pins, queued pinned recency/content
  updates, shared count/names, explicit release and immediate owner pin reorder.
- Actual iframe content growth above AND below the viewport, main and pin
  scrollers, with measured feasible placement intervals and hard geometry
  preconditions. Authored sandbox fixture code writes its own content in response
  to a stimulus; the production bridge reports actual height. The parent driver
  reads only host geometry, never opaque child DOM. Visible height updates are
  held and explicitly released; wrong docKey and wrong source-window reports
  are rejected.
- A fetched closed-page topic stages transfer/rendering while exposed, admits
  eligible rendering in place once only its changed panel leaves, retains panel
  membership/history boundaries, then transfers once the whole card leaves.
  A real older `PastTail` panel supplies scroll range instead of artificial CSS.
- Immediate owner memo editing, local send grants that do NOT admit an external
  same-topic rewrite, then receipt-only partial admission with the concurrent
  external rewrite still staged.
- Actual sliver and measured reading-line gap cases, short nested outer apply,
  removed real reference/no fallback, focus preservation, history membership,
  native anchoring suppression and frame identity boundaries.
- Wheel, touchstart, pointerdown and scroll-key cancellation, 200ms cooldown
  extension through representative user-associated momentum scroll, resumption,
  and production `loadTarget` explicit jump with late correction, reduced motion,
  inspection during jump and immediate cancellation.

**INFERRED design limitations / omitted:** Real OS touch inertia, full native
input device fidelity, inner iframe text/content-point preservation, every
virtual timing permutation and the non-renewing cap exhaustiveness remain
outside this browser batch. Input modalities other than wheel are synthetic DOM
intents. Pinned order is user-authored: external recency does not request a
server-authored permutation of that order, so the driver does not patch `weave`
or manufacture a false pinned placement contract. It tests actual pinned
recency updates and explicit owner reorder instead. Source-level/deterministic
coverage remains parent-owned and unrun here. No approved visual design exists;
all screenshots concern outer panel guarantees, not visual design approval.

This is authored test infrastructure, not acceptance, a review, a phase Result,
a ticket closure, a deployment, or permission to start the batch.
