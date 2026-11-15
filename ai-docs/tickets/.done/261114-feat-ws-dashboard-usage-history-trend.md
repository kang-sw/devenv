---
title: Design usage history storage and a baseline-colored popover trend chart
related:
  261114-feat-ws-dashboard-usage-footer-meter: Phase-1 latest-value footer; this is the separate phase-2 scope
  261113-feat-ws-dashboard-rail-usage-gutter: Existing latest-only source, privacy boundary and popover
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: f1f984e360088484
sage-review-design-reviewed: f1f984e360088484
completed: 2026-11-15
---

# Design usage history storage and a baseline-colored popover trend chart

## Background

MAIN relayed owner decision (738)=a on 2026-11-14: deliver the latest-value
footer first, then usage history storage and a popover trend chart. Current
`agents-plugin-tool/internal/wsdashboard/plan_usage.go:21-40,59-66,101-103`
holds and persists latest window values, not a series. Past observations
cannot be reconstructed from that state.

This is the phase-2 implementation contract. MAIN settled the separate-file
storage and T0 additive direction; the fact-population technical gaps are
resolved below without reopening the confirmed owner policy. Ready promotion
is preparation only, not permission to begin implementation.

## Approved visual reference

[usage-meter-732 mockup](../assets/261114-feat-ws-dashboard-usage-footer-meter/mockup.html),
section B, is the approved popover layout and cumulative-usage reference.
Its history is explicitly authored fixture data, not runtime evidence.

The owner approved (737) with this correction: the actual line should
transition gradually toward red above the ideal-burn baseline and toward
green below it; the area above the baseline should have a translucent red
cellophane-like fill. The original mockup's blue line/fill is superseded by
that correction, not authority to retain a fixed blue line in the result.

Verification must compare matching-dimension result captures side by side
against B and separately demonstrate the corrected line/fill behavior.

## Decisions

- (738)=a: history plus the popover trend belongs to phase 2, separate from
  the T0 latest-value footer. Rejected: implementing storage implicitly in
  the footer change or treating fixture history as collected observations.
- (737)=ok with the explicit line-color and above-baseline fill correction
  recorded above. The comparison reference is the ideal-burn baseline, not
  a separately invented fixed safety threshold.
- (739)/(740) remain applicable when showing latest values: after 30 minutes
  use clock/dotted/neutral stale treatment and freeze stale pace delta at
  the last observation. Stale data must not imply a fresh measurement.
- MAIN's 2026-11-15 02:36:00 technical settlement selects a separate bounded
  private history file. Preserve the latest cache/API byte semantics; expose
  history only through an additive, optional projection. Target **T0 additive**
  with old-reader/writer non-destruction and pruning tests. Reject replacing
  the latest model or embedding history in its fixed-struct round-trip, which
  would discard unknown fields. This tier depends on the tested additive
  mixed-version contract, not the footer's earlier T0 classification.
- Keep account identifiers out of history. Do not join observations across
  reset or source boundaries, or join into an unknown-reset series. Specify
  successful unchanged readings and GPT decreases explicitly in the settled
  sample-acceptance contract; no cached reread becomes a new observation.
- (890)=a, relayed by MAIN at 02:41:53: retain page-driven acquisition and
  the rail ticket's no-viewer polling boundary. Owner note, verbatim:
  "그러면 닫혀있는 구간은 점선 그래프로 연결, 기울기를 내삽하는 방식으로 그리죠."
  English translation: connect closed intervals with a dashed graph and
  interpolate the slope. MAIN's announced reading selects linear visual-only
  interpolation between bracketing real observations. Store no synthesized
  observations; hover identifies the segment as `no record (estimated)`.
  Never interpolate across reset or source boundaries. Apply (737)'s
  baseline-relative colors to dashed segments too. This owner decision
  supersedes MAIN's earlier instruction to break every missing interval.
  Rejected: daemon-periodic sampling, which would reverse the no-viewer rule.
- (891)=b: retain 14 days with a hard cap of 5000 observations per
  harness/window-kind and test pruning. Reject current-window-only coverage
  and the 30-day alternative for this phase. Both age and count bounds apply.
- Split strokes at even-burn baseline crossings. Preserve (737)'s gradual
  red-above/green-below tint and translucent red fill only in the excess region
  where actual usage is above the baseline; MAIN corrected its contrary
  wording at 02:37:47. The old blue fill-to-zero is superseded. Reset/source
  boundaries and unknown-reset series remain disconnected; ordinary missing
  intervals follow (890), not the superseded break-only rule.
- MAIN delegates exact styling and gap detection to technical settlement.
  Use B's muted-foreground token at zero baseline difference, mixing gradually
  toward its red/green tokens in proportion to the absolute percentage-point
  difference divided by 100. Retain B's 2.3px stroke and 0.09 area opacity,
  using red for the excess fill. Estimated segments use a `4 3` dash pattern
  and the explicit estimated hover label; the muted dashed ideal line stays
  visually distinct. Treat intervals over ten minutes (twice the existing
  ordinary polling interval) as gaps. This threshold detects observation
  gaps, not authoritative page-open/closed telemetry. Do not extend a segment
  beyond its last real observation or infer future usage.

### Technical contract resolved from fact-population gaps

The lead auto-confirms these four technical resolutions under the
fact-population exception; they introduce no new owner policy or acquisition
permission.

- **Separate schema and pruning:** use owner-only, atomic
  `plan-usage-history.json` beside the latest cache, with
  `{schema_version: 1, samples: [...]}`. Each sample contains only `harness`,
  `kind`, `source`, `resetsAt`, `asOf` (epoch milliseconds) and `percent`.
  Reuse existing validated window kinds, finite 0-100 percentages and positive
  observation times. Retain a sample exactly at `now - 14 days`; remove older
  samples. Then keep the newest 5000 per harness/window-kind across its reset
  and source partitions, not 5000 per partition. Sort by `asOf`, then a stable
  lexical tuple of harness/kind/source/reset for deterministic ties. Reject
  embedding history in `plan-usage.json`: its fixed-struct serializer would
  erase it. Do not add account, credential, plan or raw-error fields.
- **Optional wire projection:** add `history=1` to existing viewer-authorized
  `GET /rail/usage`, composing with the existing refresh parameter. Without
  that flag, preserve the existing response and latest-cache serialization
  byte for byte. With it, add only a `history` member containing
  `{schema_version: 1, status: "ok"|"empty"|"unavailable", samples: [...]}`;
  expose only the allowlisted sample fields. Reuse the current request's
  acquisition/coalescing behavior, not a second probe. An old server may omit
  the member; the new UI must fall back truthfully to current-point/no-history
  presentation. Unknown resets can appear only as isolated observations,
  without an invented baseline or connected series. Reject an always-present
  member or a separate polling request: neither is needed to preserve default
  response bytes and cadence.
- **Sample acceptance:** append only successful, identifiable acquisitions,
  never failed attempts or cache rereads. Claude contributes only updates
  accepted by its existing guard; unchanged heartbeat snapshots do not create
  points or refresh age. A fresh successful GPT response with a new `asOf`
  contributes a point even at the same percentage. Upsert equal
  harness/kind/source/reset/asOf keys with the last accepted real value rather
  than duplicating timestamps; this does not change latest-value acceptance.
  Retain a real GPT decrease without clamping it or joining the decreasing
  pair as cumulative growth. Keep those observations disconnected; do not
  infer an account identity or silently discard the measurement. Reset/source
  changes and zero-reset observations likewise never form a connected series.
- **Mixed-version preservation:** pin the latest-only reader/writer contract
  at `55b50bd534ba5246d64f9597d6296e2308612705` and test it against a directory
  containing the new history file: it must leave that file byte-identical.
  New writes must leave the latest-only serialized shape unchanged. A missing
  history file is empty; a corrupt or unsupported-version file remains intact,
  is not rewritten or pruned, and projects `unavailable` while latest usage
  still works. Recovery requires an explicit repair/removal, not automatic
  deletion. Preserve unknown fields in supported-version envelopes and retained
  sample records on read-modify-write, but never leak them through the viewer
  projection. Test that round-trip and newer-version write refusal. Reject
  destructive migration, silent corrupt-file reset and dropping unknown fields.

## Constraints

- Owner choices and the technical contracts above are settled. No
  implementation starts until MAIN assigns it, even after this ticket becomes
  ready.
- Preserve the existing usage-source and privacy boundary. Auth documents,
  tokens, account identifiers and subprocess output are not chart history.
- Reference charts contain fictional samples. A real trend must be based on
  collected observations; the approved reference's no-history state uses the
  current observation and baseline rather than fabricating a trend.
- No product edits, storage migration, live account query, browser/daemon
  activation or runtime test is authorized by this documentation capture.
- Before product edits, read `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Source grounding

Read-only inspection confirmed the following at the source-equivalent pins
`1234358fe7b9096ea9dec518193d78dbf510dd7c` and
`55b50bd534ba5246d64f9597d6296e2308612705`; Git comparison of the relevant
usage source, tickets and mockup assets was empty. The later footer hygiene
merge `f1798fcce` changes documentation only.

- `agents-plugin-tool/internal/wsdashboard/plan_usage.go:17-66` persists one
  latest Claude/GPT entry in `plan-usage.json`, not a time series. Its fixed
  serialization cannot preserve an unknown embedded history field.
- `plan_usage.go:72-115` rejects older Claude resets/observations and
  non-increasing values in the same reset. `plan_usage_probe.go:40-63`
  replaces GPT's latest successful entry; failures preserve old measurements
  with unavailable status. History must not silently change these latest-value
  acceptance rules.
- `assets/usage.js:76-102` renders text details in the popover; `:103-131`
  refreshes on mount, visibility return and popover opening, with ordinary
  five-minute polling only while a document is visible. Daemon query
  coalescing and failures mean this is not a guaranteed sampling clock.
- Claude hosted snapshots can arrive without an open page; GPT acquisition
  remains page-driven. The completed rail ticket explicitly rejected
  no-viewer polling. Repeated cached reads do not reconstruct absent history.
- The completed footer implementation is merge
  `21a9d0ee6b7ae02429b348a7837a4a3555c7f4f2`; its recorded tests and 16 visual
  comparisons are implementation evidence, not checks rerun in this lane.
  Section B's fictional samples remain layout references only.

Paths without a package prefix above are under
`agents-plugin-tool/internal/wsdashboard/`.

## Prior Decisions

- 261114-feat-ws-dashboard-usage-footer-meter (2026-11-15, Resolution): "Phase-2 history storage and the baseline-colored graph remain separate" — bearing: constrains
- 21a9d0ee (2026-11-15, commit): "16 light/dark x 1280/360 x normal/over/stale/weekly screenshots compared to the approved mockup" — bearing: supports
- e6fe5857 (2026-11-15, commit): "floor fixed the \"59.000000001%\" noise but shows 28.999999999999996 as 28 and 59.6 as 59, disagreeing with the server-rendered chips and with pace(), which already rounds." — bearing: constrains
- 261113-feat-ws-dashboard-rail-usage-gutter (2026-11-14, commit): "the gutter renders in the shell rail only, is not clipped, the popover opens and closes (Escape and outside click), the empty state works" — bearing: constrains
- d3500a7c (2026-11-14, commit): "detach usage queries from viewer cancellation with a bounded 10-second context, record lastQuery after completion" — bearing: constrains
- 292e2089 (2026-11-14, commit): "Keep changes in the existing usage assets rather than shared dashboard.js/css so parallel dashboard branches remain cheap to merge." — bearing: constrains
- f9b3c792 (2026-11-13, commit): "Window identity is its reset, with monotonic Claude usage within one window. Heartbeat-only repeats do not rejuvenate asOf." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/plan_usage.go, plan_usage_probe.go, assets/usage.js; separate history persistence and optional projection |
| scope.surface | public-interface | additive viewer JSON projection beside existing GET /rail/usage; agents-plugin-tool/internal/wsdashboard/plan_usage.go#L199-L225 |
| scope.new_public_symbol | no | no exported Go symbol is required; the new public surface is an optional history=1 query argument and history response member on existing GET /rail/usage |
| scope.new_type_contract | yes | schema_version:1 separate history samples and optional history projection are specified in Decisions |
| scope.test_surface | new-files | extend existing plan_usage_test.go, plan_usage_probe_test.go, usage.test.js and testdata/usage-footer.browser.cjs fixtures; add isolated history persistence/projection and mixed-version preservation coverage, none claimed implemented |
| complexity.reuse_points | confirmed | read latest store and Claude guard in agents-plugin-tool/internal/wsdashboard/plan_usage.go#L17-L115, GPT acquisition in plan_usage_probe.go#L19-L63, popover lifecycle and polling in assets/usage.js#L76-L131; read approved mockup B |
| complexity.side_effect_risk | high | adds persistent writes and pruning beside acquisition paths that already read credentials, spawn a subprocess and issue bounded HTTP requests; plan_usage_probe.go |
| risk.correctness | high | separate sample acceptance, reset/source partitioning, pruning and baseline crossings must preserve the asymmetric latest-value rules in plan_usage.go#L72-L115 and plan_usage_probe.go#L40-L63 |
| risk.fit | moderate | reuse the existing popover lifecycle and approved B geometry without changing page-driven query cadence; assets/usage.js#L76-L131 and approved mockup |
| risk.test | high | existing fixtures cover latest values, not historical ingestion, mixed-version non-destruction, deterministic pruning or corrected estimated-segment rendering |
| risk.security_or_contract | high | history must remain credential/account-free and preserve latest cache/API semantics across old/new readers and writers; plan_usage.go#L19-L33 and #L49-L66 |

## Phases

### Phase 1: Implement bounded observation history and the corrected popover trend

Extend existing successful usage acquisition with a separate bounded private
history store and additive optional projection, preserving the latest-value
cache/API and existing acquisition/privacy boundaries. Render B's cumulative
usage charts from actual retained observations. Implement the confirmed (737),
(890) and (891) contracts: baseline-relative color and excess-only red fill,
page-driven sampling, truthful dashed linear interpolation for gaps, and both
14-day and 5000-per-harness/window-kind bounds. Keep reset/source/unknown-reset
series separate and preserve stale latest-value styling and frozen delta.
No background sampling, account identifiers, synthesized history observations
or future extrapolation are authorized.

Verification must cover real observations versus absent history, reset/source
boundaries and unknown resets; successful unchanged readings versus cached
rereads, GPT decreases and acquisition failures; stale latest values and frozen
delta; both sides of a baseline crossing; dashed estimated segments with the
explicit hover label and no stored interpolation; and no interpolation across
identity boundaries. Demonstrate gradual red/green strokes and red translucent
fill only above the baseline, with matching-dimension captures compared side
by side against B plus (737)'s correction. Test empty/corrupt/unknown-version
history, latest-value behavior preservation, old-reader/writer non-destruction,
and deterministic age/count pruning including exact boundaries. Use fixtures
only; live credentials, providers, daemon replacement and shared port 29417
are not authorized by this documentation task. Final ready status is preparation
only; MAIN owns implementation assignment. No implementation Result is claimed.

### Result (5ba3975e3) - 2026-11-15

Implemented the assigned phase on `impl/develop/abide-zesty-bring`:

- `a3b112047` adds the separate owner-only atomic schema-1 history file,
  supported-version extension preservation, unavailable/write-refusal behavior
  for corrupt or unsupported files, deterministic inclusive 14-day retention
  and a newest-5000 cap per harness/kind across partitions. Only accepted Claude
  windows and successful GPT acquisitions enter history; fresh unchanged GPT
  values and real decreases survive, while cache rereads and failures add none.
- The opt-in `history=1` projection composes with the existing request and
  refresh/coalescing rules. Latest cache types/serialization and default API
  members remain unchanged. Frozen latest-only wire types/read-write logic
  from the settled compatibility pin verify adjacent-history non-destruction.
- `6147d0f19` adds pure observation-only geometry: exact baseline crossings,
  reset/source/unknown-reset barriers (including A-to-B-to-A transitions), no
  decreasing edge, no fabricated origin or future extrapolation, and dashed
  linear estimates only for intervals strictly over ten minutes.
- `5ba3975e3` renders 340px compact window cards, per-segment proportional
  red/green gradients, excess-only red fill, estimated hover labels, truthful
  old-server/empty/unavailable fallbacks, and stale neutral current values
  with frozen pace. The existing single visible-page polling request carries
  history; no sampling service or second fetch was added.

Verification:

- `GOPROXY=off GOTOOLCHAIN=local go test ./internal/wsdashboard -run
  'TestPlanUsage|TestAssetIDFollowsAssetContent' -count=1` passed after the
  final product commit. The earlier focused `TestPlanUsageHistory` run also
  passed. These include storage/privacy, exact pruning, acquisition acceptance,
  default/optional API preservation and pinned mixed-version coverage.
- The DOM leaf ran `node --test
  agents-plugin-tool/internal/wsdashboard/usage.test.js
  agents-plugin-tool/internal/wsdashboard/usage-trend.test.js`: 20 tests passed.
- `git diff --check` passed. One independent lite logic/state/test-integrity
  review of `bffec047c..5ba3975e3` was clean; no second review was run.
- Compared source-only production DOM capture
  `/tmp/ws-usage-history-popover-capture.html` vs
  `ai-docs/tickets/assets/261114-feat-ws-dashboard-usage-footer-meter/mockup.html`:
  mismatch. The 340px card and 288x113 SVG dimensions match in source; the
  reference legend/current-time guide are absent, and the approved corrected
  tint/excess fill intentionally replaces its blue styling. This was an HTML
  capture and source comparison, not a rendered visual pass or screenshot.

Decisions and unresolved:

- MAIN's implementation assignment supersedes this ticket's preparation-only
  hold. The latest owner UI-fast instruction supersedes its screenshot/matrix
  procedure: none were taken. Visual acceptance belongs to owner on-screen
  feedback after MAIN integrates through `ws/git.merge` and swaps the runtime.
- Minor unresolved visual-reference differences are the absent legend and
  current-time guide noted above. No logic/state review finding remains open.
- Browser checks, full suites, real providers/credentials, external downloads,
  shared daemon/port 29417, pushes, integration and runtime swapping were
  omitted. MAIN retains integration ownership; no final branch sync was
  needed for the isolated usage paths.


## Resolution (2026-11-15)

Implemented bounded private observation history, the additive opt-in projection, and observation-only popover trends. Scoped storage/compatibility/logic tests passed and one independent lite logic review was clean. MAIN retains develop integration and runtime swapping; owner on-screen feedback is visual acceptance under the UI-fast directive.
