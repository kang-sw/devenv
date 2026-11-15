---
title: "Isolate dashboard file explorer instances"
parent: 261114-epic-ws-dashboard-file-explorer
related:
  261110-feat-ws-dashboard-file-preview: "current read-only Files-tab and preview behavior to preserve"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 49d333d8218e9f10
sage-review-completeness-reviewed: 49d333d8218e9f10
completed: 2026-11-15
---

# Isolate dashboard file explorer instances

## Background

The current dashboard shares one `#files` instance between the conversation
surface and toolbar references (`dashboard.js` around the `#files` setup;
`cards.js` around the conversation and toolbar references). The reusable
explorer requires isolated instances so one card's state cannot overwrite
another card's state. This is the first technical slice of the approved file
explorer epic.

## Decisions

- Isolate explorer instance ownership while preserving the current user-visible
  behavior and security boundary.
- Preserve the existing normal Files guarded roots and read-only filesystem
  behavior. This refactor does not widen roots, add filesystem mutation, or
  turn linked-file capabilities into browser authority. The separately approved
  workspace picker is local-viewer-only and directories-only; this refactor
  leaves that future picker consumer's boundary intact but does not implement
  the picker.
- Keep browser-owned state separate per instance. Do not introduce a daemon or
  shared persistence store for this refactor.
- Compatibility tier: T0 for this browser-instance refactor. It has no
  protocol, on-disk shared-store, or root-policy change.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- The current Files tab and toolbar entry points must continue to work while
  instances become independent. Preserve their current appearance and compare
  matching baseline/result states. The parent epic's approved mockup governs the
  later first user-facing feature delivery, not a redesign in this technical
  refactor; do not implement that deferred UI to satisfy this child's acceptance.
- The refactor must not decide the later explorer's unconfirmed UI grouping,
  rendering mechanism, storage schema, endpoint shape, or dependency choice.
- No future backend compatibility tier is assigned here. Any backend proposal
  must be additive when necessary, with semantics and authorization settled
  separately before classification; no existing root or API semantic rewrite is
  authorized.

## Prior Art

- Current singleton references: `dashboard.js` around the `#files` setup and
  `cards.js` around the conversation and toolbar references.
- Current browser card state: `dashboard.js` around its storage and card-state
  helpers.
- Existing read-only root and file behavior: the `/fs` list, preview, raw, find,
  and search surfaces. Browse-root validation is discoverable in `fs_routes.go`
  through `browseRootParam` and `containedDir`; normal Files root guards remain
  unchanged. The separately approved workspace picker is local-viewer-only and
  directories-only, not blanket file-content root widening. Host-action
  checks remain in `fs_act_route.go`; preview capability checks are in
  `agents-plugin-tool/internal/wsdashboard/fs_capability.go`, with narrow
  outside-linked-file grants in `fs_outside.go`
  (`agents-plugin-tool/internal/wsdashboard/fs_act_route.go#L171-L180`,
  `agents-plugin-tool/internal/wsdashboard/fs_capability.go#L22-L58`,
  `agents-plugin-tool/internal/wsdashboard/fs_outside.go#L30-L88`); do not add
  new symlink permissions.

## Prior Decisions

- 261114-epic-ws-dashboard-file-explorer (2026-11-14, Cross-Child Decisions): "Explorer instances are isolated. No child may restore the current singleton as the state owner for all explorer cards." — bearing: supports
- 261110-feat-ws-dashboard-file-preview (2026-11-11, commit): "261110-feat-ws-dashboard-file-preview: Files tab explorer (Phase 1 tree, View, menu, keyboard, name search; Phase 2 content search UI and git marks)" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.js and assets/cards.js wire the shared Files pane; assets/files.js owns its singleton state. |
| scope.surface | cross-module | window.WsFiles mount/shown/hidden is consumed by dashboard.js; cards.js maximizes the same Files DOM node. |
| scope.new_public_symbol | unknown | No new component symbol or factory API is specified; the current window.WsFiles interface is browser-internal. |
| scope.new_type_contract | unknown | Per-instance ownership is required, but its interface/signature is not selected. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/files_test.go, board_toolbar_test.go, fs_routes_test.go, fs_act_route_test.go, fs_capability_test.go, fs_outside_test.go and fs_search_test.go cover existing preservation surfaces. |
| complexity.reuse_points | confirmed | Read assets/files.js module state and mount/shown/hidden, dashboard.js storage adapters and mount, and cards.js toolbar Files maximize path. |
| complexity.side_effect_risk | moderate | Moving module-level DOM, search cancellation, scroll and storage ownership can disturb the existing tab and toolbar lifecycle. |
| risk.correctness | high | Independent async generations, requests, handlers and selection must not overwrite another instance's state; files.js currently keeps all of these at module scope. |
| risk.fit | moderate | Instance ownership crosses explorer, page and shared card framework while the later feature contract remains deliberately deferred. |
| risk.test | high | files_test.go covers page placement and pure row/storage helpers, not multiple mounted explorers or concurrent request isolation. |
| risk.security_or_contract | moderate | Instance-specific root and session routing must preserve local-viewer checks and capability separation without changing server authority. |

## Phases

### Phase 1: Isolate explorer instances without regression

Separate explorer instance ownership and state so multiple instances can
coexist without cross-talk, while retaining the current Files-tab and toolbar
behavior, read-only operations, normal Files root guards, and existing security
checks. Do not implement the separately approved workspace picker or add its
picker UI here; do not add the later feature slice's view modes, search
expansion, preview formats, or planned child scope here.

Verification should demonstrate independent instance state and preservation of
current entry points and access checks. Compare matching before/after captures
of the current Files tab and toolbar presentation; mockup conformance belongs
to the later user-facing feature slice. Product source changes, tests, and
runtime verification belong to the later implementation run; this draft
records no implementation or verification result.

### Result (64ba9308c) - 2026-11-15

- VERIFIED: independent `WsFiles.create` closures and explicit tab/toolbar
  lifecycle owners landed in `8129e7bc6`; retained commits preserve sheet paint,
  lazy toolbar construction, existing browser record shapes and guarded storage
  adapters. No backend, filesystem authority, root guard or capability changed.
  The existing one-pass lite review's Important root-refusal race is fixed in
  `1f7363b4f`, with its controlled-refusal regressions retained. No re-review ran.
- VERIFIED: the elevated retry changed only the browser fixture (`64ba9308c`).
  Playwright's default screenshot preparation rewrites every input/textarea's
  inline `caret-color`, even after explicit blur. The cached source is
  `/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright-core/lib/coreBundle.js`,
  lines 19074–19087; line 19222 bypasses this with `caret: 'initial'`.
  The fixture now uses that setting, waits for fonts, mock responses and
  animations to settle, and asserts exact before/after DOM, computed styles,
  layout, requests, focus and scroll. Cross-revision logical/layout contracts
  and unchanged-composer assertions supplement the unchanged strict zero-pixel
  comparison. No root hide/restore, field normalization or tolerance was added.
- VERIFIED: all three failed diagnostic snapshot comparisons were inspected
  recursively without suppressing fields. Only empty input/textarea style
  attributes and root-style presence differed; computed properties, rectangles,
  focus, scroll, pending requests and animations were equal. The root repaint
  experiment was rejected. Its original pixel assertion passed, but its added
  snapshot-restoration check failed; it is diagnostic evidence, not acceptance.
  Exact differences and screenshot-window composer Paint events are retained in
  `/tmp/ws-files-elevated-evidence/go/raw-snapshot-diffs.json` and
  `/tmp/ws-files-elevated-evidence/go/original-paint-windows.json`.
- VERIFIED: selected offline verification passed on unchanged product/test
  sources at `2f428729f`:
  `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS=-p=2 GOMAXPROCS=2 go test ./internal/wsdashboard -count=1 -timeout=80s -run '^(TestFiles|TestToolbar|TestBoardToolbar|TestFS)'`
  from `agents-plugin-tool` (4.294s). The approved 120s total wrapper reserved
  cleanup time; its entire batch, including static evidence comparison, took
  9.27s. Log and source hashes: `/tmp/ws-files-elevated-evidence/go/`.
- VERIFIED: `python3 /tmp/ws-files-elevated-wrapper.py final` passed within
  9.48s total, including cleanup, versus a 120s total cap. The actual baseline
  remains `e86f22a82189ac6bb556e7cad84352b566a65b8b`; all product and Go-test
  input hashes match the clean Go run. Final harness SHA256:
  `a07a13762a7bee235c6fa2eea92a19e46ccd944126efa73e8c3be42ecf402990`.
  All four states have **0/1296000** differing pixels, all eight exact snapshot
  checks pass, and all four cross-revision logical/layout contracts pass.
  The final source manifest records the exact harness diff and cached dependency
  hash before launch. Evidence: `/tmp/ws-files-elevated-evidence/final/`
  (`command.log`, `source-manifest.json`, `results.json`, `state-checks.json`,
  `contracts.json`, eight `*-state.json` files and captures).
- VERIFIED: all four final side-by-side comparison images were read:
  - compared `/tmp/ws-files-elevated-evidence/final/result-tab-tree.png` vs
    `/tmp/ws-files-elevated-evidence/final/baseline-tab-tree.png`: match.
  - compared `/tmp/ws-files-elevated-evidence/final/result-tab-view.png` vs
    `/tmp/ws-files-elevated-evidence/final/baseline-tab-view.png`: match.
  - compared `/tmp/ws-files-elevated-evidence/final/result-toolbar-tree.png` vs
    `/tmp/ws-files-elevated-evidence/final/baseline-toolbar-tree.png`: match.
  - compared `/tmp/ws-files-elevated-evidence/final/result-toolbar-view.png` vs
    `/tmp/ws-files-elevated-evidence/final/baseline-toolbar-view.png`: match.
- VERIFIED: the one elevated retry consumed exactly four approved executable
  batches and two browser launches; none remain. Diagnostic driver/Chromium
  groups 79572/79721, Go group 33907 and final driver/Chromium groups 49715/49837
  are empty, with no cleanup signals or timeouts (`diagnostic/`, `go/`, and
  `final/lifecycle.json` under the evidence root). Each browser had an 85s work
  deadline within the total 120s wrapper, leaving teardown time inside the cap.
- Limitation: historical four-pixel Files-tab View causality remains unproven.
  The original failing captures and all diagnostic artifacts remain untouched;
  final success proves current strict equality and equivalent nonmutating states,
  not that the earlier mismatch was a Chromium or product bug. MAIN owns final
  integration/acceptance/deployment.
- Omitted by the authorized bounded scope: full-package tests/build, other
  viewport/theme combinations, new review sweep, real holders/providers,
  shared daemon/endpoint 29417, dependency install/download, merge/push and
  worktree cleanup. Only workflow ticket validation follows the final run;
  no executable verification is repeated after this documentation-only record.

## Execution checkpoint — retained implementation, acceptance incomplete

Historical checkpoint from the previous worker, superseded by the Result above.

- Implementation is retained on `impl/develop/haste-serve-defy` through
  `1f7363b4f`: per-instance factory/lifecycle and separate tab/toolbar ownership
  (`8129e7bc6`), deterministic/browser fixtures (`1e3352b2e`), preserved sheet
  paint and lazy toolbar construction (`dcc033133`), and the lite-review
  root-refusal race correction (`1f7363b4f`). No backend or authority change.
- Browser storage uses the existing root/View record shapes. The tab retains
  legacy keys; the toolbar gets a separate guarded adapter namespace. This
  browser-internal ownership choice does not select the later explorer's schema.
- Independent one-pass lite review found one Important issue and no Critical or
  Minor issues: candidate-root attempts invalidated valid pending current-root
  work even on refusal. A separate candidate sequence and controlled HTTP-refusal
  regressions fix it. No re-review is allocated under lite.
- Latest bounded offline verification passed:
  `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS=-p=2 GOMAXPROCS=2 go test ./internal/wsdashboard -count=1 -timeout=120s -run '^(TestFiles|TestToolbar|TestBoardToolbar|TestFS)'`
  from `agents-plugin-tool` (5.348s, owned process group retired). Log:
  `/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/ws-files-review-fix-fv774rao/go.log`.
- Appearance captures use actual baseline assets at `e86f22a82` and actual result
  initialization/assets in an offline mocked fixture, not the parent's mockup.
  The strict comparison passed all four tree/View states for both entry points
  before review, retained under
  `/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/ws-files-verification-i1v5spyu/`.
- The final pinned-commit run at `1f7363b4f` again produced four differing pixels
  in Files-tab View (4/1296000); tab tree and both toolbar states remain exactly
  equal. Read `comparison.html`, `results.json`, and `compare-tab-view.png` under
  `/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/ws-files-verification-4nvxg2o1/`.
  The side-by-side View capture was read; strict comparison is **mismatch**.
  The earlier four-pixel signature had disappeared in the intervening passing
  run, so its implementation-versus-browser-raster cause remains unproven.
  The strict assertion was not weakened. Repeated appearance non-convergence
  triggers worker stop (f); no further attempt was made.
- Final failing command:
  `python3 /var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/ws-files-verification-y_tgjir1/browser-wrapper.py`
  (120s cap, exit 1; browser closed and final ps for owned group 95988 empty).
- No Result is recorded and the ticket remains ready pending final acceptance.
  Full-package tests/build and broader viewport/theme coverage were omitted to
  preserve the authorized bounded offline scope. No shared daemon, endpoint
  29417, installs/downloads, real holders/providers, merge, push, or worktree
  release occurred. MAIN owns the next acceptance decision and integration.
