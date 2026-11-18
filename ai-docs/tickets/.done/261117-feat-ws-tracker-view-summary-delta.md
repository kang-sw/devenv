---
title: "Tracker view: summary first, deltas since a revision, incremental server view"
completed: 2026-11-18
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0bf72dad2df626bb
sage-review-completeness-reviewed: 0bf72dad2df626bb
---

# Tracker view: summary first, deltas since a revision, incremental server view

## Background

The dashboard tracker view (`GET /tracker/stores/<store>/view`) sends the
whole store as one JSON document. Two fixes reduced its repeat cost:

- `41a1fa839` (tracker-view-refetch): the view is cached by data revision,
  answered with a weak ETag and 304 when unchanged (bounded by a three-minute
  TTL; `agents-plugin-tool/internal/wsdashboard/tracker_answers.go#L13-L35`).
  The owner measured a hit at 7 ms on 2026-11-17
  (`ai-docs/.plans/2026-11/17-1837-tracker-miss-path.md#L10-L13`).
- `beee01e06` (tracker-miss-path, merged on 2026-11-17): a miss whose
  ledger and git state have unchanged, trustworthy stamps skips ledger
  refresh and git; files younger than two seconds cannot be trusted and
  force reads (`agents-plugin-tool/internal/wsledger/stamp.go#L16-L38`,
  `agents-plugin-tool/internal/wsledger/store.go#L778-L786`,
  `agents-plugin-tool/internal/wstrackerhost/host.go#L952-L970`).

The miss-path fix also reuses unchanged item/comment markdown renderings
when something changes (`agents-plugin-tool/cmd/ws-mcp/tracker_host.go#L326-L373`).
The remaining full-view derivation, encoding, transfer and browser work still
scale with the view, even for a one-entry change; file reads are stamp-gated,
not necessarily O(N) per change:

| step | cost |
|---|---|
| server derives and encodes the whole view | CPU; unchanged markdown is reused (`agents-plugin-tool/internal/wstracker/tracker.go#L68-L73`, `agents-plugin-tool/cmd/ws-mcp/tracker_host.go#L193-L239`) |
| the whole view travels as JSON on a changed 200 response | 291 kB in the owner's earlier trace, not a fresh size measurement (`ai-docs/.plans/2026-11/17-1707-tracker-view-refetch.md#L5-L9`) |
| the browser parses the full answer and redraws the open drawer/window | render time; a closed drawer skips its DOM rebuild, but handles and feed computation still run (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-tracker.js#L28-L39`, `agents-plugin-tool/internal/wsdashboard/assets/tracker-render.js#L125-L199`) |

Existing remote benchmark evidence, not remeasured here: `3ebe32538` records
`BenchmarkDashboardTrackerViewMiss` on w5090 without antivirus, 120 items each
with a comment, 979 kB view: dashboard view 57.6 -> 9.5 ms; remaining encoding
stages were answer re-encode 7.3 ms, tag re-marshal 4.9 ms, view JSON 3.2 ms.
These are separate benchmark stages, not a new end-to-end timing or an owner-PC
projection (`agents-plugin-tool/cmd/ws-mcp/tracker_view_bench_test.go#L32-L40`,
`agents-plugin-tool/cmd/ws-mcp/tracker_view_bench_test.go#L94-L125`).

The owner expects stores to reach tens of thousands of entries. At that size
each change would move several MB to over 10 MB, and with several sessions
writing, changes arrive every few seconds. The owner's PC runs AhnLab V3, and
target users run heavier security software, so file touches and large
responses are the cost to cut (clone note
`release-gate-daemon-file-io-20261117`).

## Decisions

- **Direction (owner (1307)=a, 2026-11-17 18:23).** The view is redesigned
  along three lines:
  1. **Summary first, detail on demand.** The first response carries only
     what the list needs (title, status, time). Bodies and details are
     fetched when the user expands an entry or it scrolls into view.
  2. **Deltas since a revision.** The browser sends the revision it holds;
     the server returns only the entries changed since then. The server keeps
     the per-revision change list in memory and does not reread files.
  3. **Incremental server view.** The server applies changed entries to its
     in-memory view instead of rebuilding from scratch, and runs no git while
     the store stamp is unchanged.
- **Sequencing (owner (1307)=a).** This work starts after the current
  daemon file-load work (2026-11-17 perf queue) settles.
- **Incremental fold (owner (1323)=d, 2026-11-17).** Direction 3 is realized
  as checkpointed incremental folding plus output diffing. The server keeps
  in-memory fold checkpoints (the saved fold state every K events, denser near
  the tail). Every change re-folds from the latest checkpoint before its
  earliest affected position: a tail append, a mid-order remote arrival (the
  normal case in team use: a teammate's concurrent events sort by ts among
  local events written since the last sync, `wstracker/replay.go:161-163`), a
  filled gap, or a changed protocol verdict. Cold start, when no checkpoint
  exists, folds from zero; otherwise only the worst case below re-folds most
  of the history. The delta comes from per-item summary
  hash changes between the previous and the new fold. Owner model: cache the
  state, replay from a point in order, and deterministically reach the current
  state. The fold order is kept by splicing changed events into it, not by
  re-sorting the whole history.
  - Worst case (owner (1341)=a): the cost is proportional to the events after
    the earliest affected position. An event older than most of the history,
    such as a long-offline clone's sync, sorts near the front and re-folds
    almost everything after it; this bounded worst case is accepted, because
    in a deterministic total-order fold an event inserted at the front affects
    everything after it, and checkpoints only spare the prefix. Common team
    syncs sit near the tail. Rejected: folding in per-clone arrival order so
    late events always append, because clones would fold the same events in
    different orders and lose determinism; dependency-aware re-derivation of
    only the touched items, because fold-order rules (the label clash) and
    causal-past verdict checks make it far heavier.
  - Why a checkpoint and not the current state alone: the fold overwrites
    state and is not invertible, so a restart needs the state saved before the
    earliest affected position.
  - Prefix stability (INFERRED from source; Phase 1 proves it with a property
    test): linearize is Kahn's algorithm over a ts/member/clone/seq priority
    heap (`wstracker/replay.go:164-213`). A new event has no successors among
    existing events, so the order is the old order with it inserted. A filled
    gap moves the waiting events in at or after the filled event. A revocation
    or ignore verdict keeps the event in the causal order (`inCausal` excludes
    only broken and Waiting events, `replay.go:165`) and changes only its
    status.
  - Rejected: (a) an append-only fast path with full replay on any
    out-of-order arrival, because in team use almost every remote sync arrives
    mid-order; (b) output diffing only, because replay stays O(total history)
    per change; (c) fully dependency-aware incremental derivation, which is
    more machinery than the prefix property needs; an earlier form of (d) that
    kept full replay for verdict changes, because verdict changes are
    positional too.
- **Routes and compatibility ((1324), announced).** `GET
  /tracker/stores/<store>/view` and its 304/ETag stay unchanged; the new
  routes sit beside it; a new page falls back to the full view when a new
  route is missing (old daemon).
- **Summary fields ((1325), announced).** The summary carries every per-item
  field the list rows, handles, badges, notices, mentions and palette read
  (title/status/time alone breaks the rows in `wsdashboard/assets/tracker.js`
  and the handles that read the view at mount, 8e7a67b9d). It excludes
  body/bodyRev, comment and thread-event payloads, rendered HTML and
  attachment detail.
- **Cursor ((1327), technical).** An opaque daemon-minted cursor `<daemon
  epoch>.<store view generation>`; a cursor equal to the current generation
  answers 304. Rejected: a ledger-stamp-derived revision, because ledger
  stamps are size/mtime fingerprints and are not ordered (`wsledger/stamp.go`).
- **Change ring ((1328), technical).** An in-memory ring of the last 64 view
  generations' changed and removed item ids per store; an older or
  foreign-epoch cursor gets an inline full summary marked `reset`. Rejected: a
  reset response followed by a refetch, which costs a second round trip.
- **Seen and time-relative state ((1329), technical).** Seen marks and
  time-relative flags are per-item summary fields, so a seen write or a stale
  flip puts that item in the next delta through the same output diff.
  Rejected: a separate seen/global delta channel, a second change path.
- **Detail ((1330), technical).** Detail is a per-item bundle (body, bodyRev,
  comments, thread events, rendered HTML, attachments) fetched in batches of
  expanded or visible ids. Rejected: paginated per-resource endpoints, which
  need more round trips.
- **Detail version (design review, autonomous).** Each summary row carries a
  detail version: a hash over everything its detail bundle renders (thread
  events with their verdict reasons, comments, body, attachments). The output
  diff includes it, so a detail-only change, such as a verdict reason change
  that `wsledger/store.go:1034-1046` tracks and `assets/tracker.js:630-635`
  shows, still puts the item in the next delta. Each bundle carries the
  detail version it was built from, published in the same generation as the
  summary. The page uses a bundle only when its version equals the summary
  row's; a mismatched bundle is held until the matching delta arrives or is
  refetched. Rejected: dropping only bundles older than the row, because a
  newer bundle can arrive before its summary delta.
- **Page model ((1331), technical).** The page applies deltas to a keyed local
  model and redraws only changed rows. Rejected: a full redraw per answer;
  redraw is one of the three costs this ticket targets.
- **Feed ((1333), technical).** The feed reads a bounded latest-N activity
  projection carried with the summary and delta. Rejected: fetching detail
  bundles for feed snippets; the feed shows snippets only
  (`wsdashboard/assets/tracker-panel.js`).
- **Phasing ((1332), technical).** Phase 1 is the server side, Phase 2 the
  page. Rejected: one phase, because correctness and test risk are high and
  each half is independently reviewable.

## Constraints

- Expected compatibility tier T0, conditional on retaining the full-view
  endpoint and shape: the daemon writes the answer through `TrackerHost.View`
  and the cache/encoder (`agents-plugin-tool/internal/wsdashboard/tracker_routes.go#L719-L738`,
  `agents-plugin-tool/internal/wsdashboard/tracker_answers.go#L149-L168`);
  the existing page reads it with GET, retaining its answer on 304
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-tracker.js#L74-L101`).
  View reads do not write store data; page mutations use separate dispatch
  and seen POSTs (`agents-plugin-tool/internal/wsdashboard/tracker_routes.go#L21-L26`,
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard-tracker.js#L193-L203`).
  An old daemon cannot serve a new endpoint: the new page falls back to the
  existing full view (Decision "Routes and compatibility"), and Phase 2
  verifies that fallback.
- Keep the 304/ETag behaviour from `41a1fa839`.
- Read the manuals in `AGENTS.md` `### Implementation Conventions` for the
  touched paths (`agents-plugin-tool/internal/` -> release-compatibility).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- No local `go test`, Go test executables, ws exes or launcher on the owner's
  PC; Go runs only on the w5090 remote (clone note
  `no-local-go-test-v3-20261116`).
- Verification ((1326), announced) measures synthetic stores of 10k and 30k
  items carrying about one million events on w5090, before and after:
  response bytes and server time per one-item change on the checkpoint path
  (a tail append, a mid-order insertion, and a far-back insertion near the
  start of history), the cold-start full fold, and
  browser render time. The `stateBefore` verdict cost
  (`wstracker/replay.go:319`) of the re-checked tail is part of the bench.
- Out of scope: a persisted snapshot that removes the cold-start full fold;
  it is a follow-up ticket.

## Prior Decisions

- 41a1fa839 (2026-11-17, commit): "answer tracker views from a revision-keyed cache with 304s" — bearing: constrains
- 8e7a67b9d (2026-11-17, commit): "a lazy view fetch on opening the tracker UI - the handles' badges, notices, mentions and palette read the view at mount, so it needs a new summary endpoint" — bearing: constrains
- 3ebe32538 (2026-11-17, commit): "The memo sits in wsledger.refresh, not only in Reload. Every refresh caller benefits" — bearing: constrains
- 261116-research-ws-polling-minimization-event-ledger (2026-11-16, Decisions): "Reuse the existing broker, targeted index lanes and mailbox wake hub rather than adding a parallel event framework." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/tracker_routes.go, agents-plugin-tool/cmd/ws-mcp/tracker_host.go, agents-plugin-tool/internal/wstrackerhost/host.go, agents-plugin-tool/internal/wsdashboard/assets/dashboard-tracker.js |
| scope.surface | cross-module | TrackerHost.View bridges JSON-level dashboard and tracker host; new HTTP summary/delta/detail responses cross server/browser boundary (agents-plugin-tool/internal/wsdashboard/tracker_routes.go#L186-L197). |
| scope.new_public_symbol | unknown | Endpoint/interface shape is explicitly undecided; no new Go symbol is named. |
| scope.new_type_contract | yes | Summary, since-revision delta and detail responses are new server/browser contracts; exact fields are undecided. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/tracker_answers_test.go, agents-plugin-tool/internal/wsdashboard/tracker_routes_test.go, agents-plugin-tool/internal/wstrackerhost/view_memo_test.go, agents-plugin-tool/cmd/ws-mcp/tracker_view_bench_test.go |
| complexity.reuse_points | confirmed | trackerAnswers weak ETags/seen union/TTL; viewRenders per-text memo; ledger stamp gating; tkLoadView revalidation read in current source. |
| complexity.side_effect_risk | moderate | Incremental cache replacement must remain coherent across background reloads, dispatches, seen writes and cross-process changes (agents-plugin-tool/internal/wstrackerhost/pageview.go#L34-L55). |
| risk.correctness | high | Derived assignments, hierarchy, rollups, notices and time-relative state can change beyond the directly edited item (agents-plugin-tool/internal/wstracker/view.go#L16-L37, agents-plugin-tool/internal/wstracker/view.go#L108-L159). |
| risk.fit | moderate | Summary fields and detail-fetch granularity remain undecided; handles/feed currently require more than title/status/time (agents-plugin-tool/internal/wsdashboard/assets/tracker-panel.js#L17-L95). |
| risk.test | high | Large-store before/after and browser render measurement are required; existing 120-item benchmark is not tens-of-thousands validation (agents-plugin-tool/cmd/ws-mcp/tracker_view_bench_test.go#L32-L40). |
| risk.security_or_contract | moderate | Additive routes must preserve old full-view/304 clients, credential redaction and attachment rendering; new-page/old-daemon fallback is unsettled (agents-plugin-tool/cmd/ws-mcp/tracker_host.go#L193-L216). |

## Phases

### Phase 1: Server summary, deltas, detail and incremental fold

Add the summary, since-cursor delta and detail-bundle routes beside the
existing view; mint the cursor and keep the per-store generation ring; replace
the per-change full replay with the checkpointed incremental fold and the
per-item summary diff, including seen marks, time-relative flags and the feed
projection.

Verification: a property test that, for random event sets and random
insertions (tail append, mid-order arrival, filled gap, verdict change), the
checkpointed fold equals a fold from zero; route tests for 304 on an equal
cursor, `reset` on an old or foreign-epoch cursor, a detail-only change
(a verdict reason) appearing in the delta with a new detail version, and
unchanged full-view 304/ETag behaviour; the Go bench rows of the verification constraint.

### Result (a89ffd864) - 2026-11-18

- **VERIFIED (source):** Phase 1 server implementation is committed on
  `impl/develop/ember-bunny-cacti`. Additive `GET summary`, `GET
  delta?since=<cursor>` and `GET detail?id=<item>&id=<item>` routes use the
  optional `TrackerProjectionHost`; old hosts answer 404 on these routes.
  The existing full-view handler and weak ETag path remain separate
  (`agents-plugin-tool/internal/wsdashboard/tracker_delta.go:236`,
  `agents-plugin-tool/internal/wsdashboard/tracker_routes.go`). Detail batches
  are capped at 100 ids. Cursor epochs are store-scoped as well as
  process-local so another store's equal-numbered generation cannot match.
- **VERIFIED (source):** The 64-transition ring diffs item summaries,
  detail versions and per-item seen marks; global fields are independently
  replaceable so an event count change does not resend itemOrder. Detail
  bundles capture the same immutable view as their row version; markdown
  and detail JSON are built only for requested ids. The feed is bounded to
  the latest 200 entries, while seen-key association covers all notifications
  (`agents-plugin-tool/internal/wsdashboard/tracker_delta.go:110`,
  `agents-plugin-tool/cmd/ws-mcp/tracker_projection.go:24`).
- **VERIFIED (source):** Tracker refresh splices the changed dependency
  closure into the retained causal order and resumes verdict checks/folding
  from a bounded checkpoint. Checkpoints copy fold-owned state and save
  prefix event annotations. Cold/far-back folds sample a coarse historical
  prefix and a dense tail rather than constructing a deep copy at every
  1024-event boundary
  (`agents-plugin-tool/internal/wstracker/incremental.go:101`, `:281`,
  `agents-plugin-tool/internal/wstracker/checkpoint_state.go:12`).
- **VERIFIED (review artifact and diff):** One independent lite review of
  `b6e0f63d7..30f8bda3d` found two Critical defects. Commit `a89ffd864`
  separates causal-check verdicts from final-fold concurrency flags and
  reconciles both dependency indexes when an existing event's Seen changes.
  Explicit losing-claim/late-descendant and pending-key-style Seen-replacement
  regressions were added alongside randomized clock/verdict/arrival tests
  (`agents-plugin-tool/internal/wstracker/incremental_regression_test.go:42`,
  `:72`). **INFERRED:** the fixes restore runtime equivalence; tests were not
  executed, and lite allocates no second review. No clean review verdict or
  runtime pass is claimed.
- **VERIFIED (source compatibility):** T0 implementation: the new routes and
  interface are additive, the legacy full-view response type is unchanged,
  and all cursors/checkpoints/rings are memory-only. No event/schema or
  on-disk writer change was made. **INFERRED:** cross-version runtime
  compatibility remains to be exercised with the unrun tests.
- **VERIFIED (command output):** `gofmt` completed and
  `git diff --check b6e0f63d7..a89ffd864` produced no diagnostics. These are
  formatting checks only, not compilation or behavioral verification.
- **VERIFIED (source):** Opt-in 10k/30k-item, million-event harnesses cover
  tail, near-tail remote and far-back insertion, before/after full-fold and
  checkpoint paths including stateBefore checks, separate cold folds, and
  authenticated full-view/summary/delta handler time and response-body bytes
  (`agents-plugin-tool/internal/wstracker/incremental_bench_test.go:30`,
  `:106`, `agents-plugin-tool/cmd/ws-mcp/tracker_projection_bench_test.go:47`).
  **UNRUN:** all Go tests, race checks and benchmarks. The ticket forbids
  local Go execution and this run forbids SSH; no allowed w5090 execution
  path was available. No timing, response-byte measurement or performance
  improvement is claimed. Checkpoint memory is bounded in count, not bytes;
  cold-copy cost and peak memory still require measurement.

Authorized-host verification commands, from `agents-plugin-tool/` (not run):

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wstracker -run '^(TestStateCheckpoint.*|TestIncremental.*)$' -count=1
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdashboard -run '^(TestTrackerSummaryDeltaDetailAndLegacyView|TestTrackerDelta.*|TestTrackerProjection.*|TestTrackerPageReadsAnswerFromMemory)$' -count=1
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./cmd/ws-mcp -run '^TestTrackerProjection.*$' -count=1
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wstracker -run '^$' -bench '^(BenchmarkTrackerIncrementalMillion|BenchmarkTrackerColdMillion)$' -benchtime=1x -count=1 -benchmem
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./cmd/ws-mcp -run '^$' -bench '^BenchmarkDashboardTrackerProjectionMillion$' -benchtime=1x -count=1 -benchmem
```

**Omitted:** Phase 2 browser changes and render-time/fallback/ordering
verification, intentionally deferred by the lead. The benchmark uses real
handler/auth paths over an in-memory tracker source, not real ledger/git I/O,
socket/network latency or browser rendering. The ticket remains in `ready/`.
The workflow_manual tool was not exposed in this worker's tool surface;
AGENTS.md, the ticket, matching manuals, route verdict and bundled ticket
conventions were read instead.

#### Edition (dff9e8dfb) - 2026-11-18

- **VERIFIED (lead-provided command results at this baseline):** build and
  scoped vet were clean; wstracker tests, scoped wstracker race tests and
  wsdashboard Tracker tests passed. cmd/ws-mcp Tracker tests failed in two
  new feed tests on missing status-event references.
- **VERIFIED (source diagnosis):** the shared fixture's status event was by
  `other`, omitted `assignee`, and therefore targeted `other`, while the
  tested assignment belongs to `me`. Both `state.go`'s status fold and
  `tracker-panel.js`'s statusEv use explicit assignee or event author. The
  Feed Decision does not authorize attributing another holder's status to
  this assignment. Corrected the fixture to carry `assignee:"me"`, retaining
  all expected event-reference assertions; added explicit-assignee,
  self-author, different-holder and different-item regression cases. No
  production behavior change was needed.
- **UNRUN by this worker:** all Go commands, as instructed. **INFERRED:**
  the fixture correction resolves the two reported failures; the lead's
  rerun remains outstanding.

#### Edition (6f8c528c0) - 2026-11-18

**Verification status update — VERIFIED (lead-provided results):** this
Edition supersedes the historical Go-test/benchmark **UNRUN** statements and
pending rerun in the frozen Result and earlier Edition. At `6f8c528c0`, the
lead reports PASS for the full `internal/wstracker` package,
`internal/wsdashboard -run Tracker`, and `cmd/ws-mcp -run Tracker`, including
resolution of the two feed-fixture failures. The earlier lead-reported
build/vet and scoped wstracker race passes were at `dff9e8dfb`; they are not
claimed as newly rerun at this revision. This worker ran no Go commands.

**Measurement provenance — VERIFIED (lead-provided):** benchmarks ran on the
Mac lead host, **Apple M3 Max, darwin/arm64, `-benchtime=1x` — NOT w5090**.
These are single-iteration observations, not statistical estimates or a
projection of owner-PC/antivirus performance. They do not satisfy the
Constraint's specifically named w5090-host measurement. The handler fixture
uses an in-memory source; times/bytes exclude real ledger/git I/O, socket
latency and browser rendering. MB/KB below retain the lead's reported units.

**VERIFIED (lead-provided): `BenchmarkTrackerIncrementalMillion`**, synthetic
million-event stores; seconds/op, before zero-fold + view versus after
checkpoint + view, with the visible-children index in `6f8c528c0`:

| Items | Arrival | Before (s/op) | After (s/op) |
|---|---|---:|---:|
| 10k | tail append | 24.08 | 1.30 |
| 10k | near-tail remote | 25.95 | 1.30 |
| 10k | far-back remote | 25.93 | 25.10 |
| 30k | tail append | 15.01 | 3.90 |
| 30k | near-tail remote | 14.24 | 3.56 |
| 30k | far-back remote | 15.42 | 13.75 |

**VERIFIED (lead-provided historical comparison):** before the children index,
`8ee9ed984` after/checkpoint + view tail rows were 4.41 s at 10k and 40.00 s
at 30k; the 30k before/zero-fold + view row was 48.0 s. The lead's profile
identified the pre-existing `subtasks` scan over every item for each item as
O(items²). Commit `6f8c528c0` replaces that repeated scan with one per-view
parent-to-visible-children index, retaining fold order and hidden filtering.
These historical rows and the table above are separate 1x runs.

**VERIFIED (lead-provided): `BenchmarkTrackerColdMillion`**, pre-index run,
seconds/op. Cold fold has no view stage, so these rows do not measure the
children-index change:

| Items | Before zero fold (s/op) | After checkpoint cold fold (s/op) |
|---|---:|---:|
| 10k | 23.59 | 24.20 |
| 30k | 11.33 | 12.79 |

**VERIFIED (lead-provided): `BenchmarkDashboardTrackerProjectionMillion`**,
authenticated handler time and response-body size:

| Items | Arrival | Route/path | Time (s/op) | Response body |
|---|---|---|---:|---:|
| 10k | tail append | before zero-fold view | 28.77 | 422.4 MB |
| 10k | tail append | checkpoint view | 4.73 | 422.4 MB |
| 10k | tail append | summary | 5.65 | 33.7 MB |
| 10k | tail append | delta | 5.55 | 759 KB |
| 10k | far-back remote | delta | 28.75 | 3.6 KB |
| 30k | tail append | before zero-fold view | 19.30 | 495.8 MB |
| 30k | tail append | checkpoint view | 8.33 | 495.8 MB |
| 30k | tail append | summary | 9.41 | 67.6 MB |
| 30k | tail append | delta | 9.15 | 2.36 MB |
| 30k | far-back remote | delta | 19.69 | 2.5 KB |

**Forward — remaining work, not implemented in this Edition:**

1. **VERIFIED (source and read-only size reconstruction; lead confirms the
   payload diagnosis):** `globals.my` is replaced wholesale when
   `my.assignments` / `my.today.work` reorder. Those two complete fallback-ID
   arrays account for approximately all tail-delta bytes: reconstructed
   755,578 bytes at 10k and 2,355,578 bytes at 30k. A tail status changes the
   first item's rank from blocked to doing; a far-back status loses to later
   writes, leaving these arrays unchanged. This is an **O(N) payload-scaling
   gap**, a Phase 2/follow-up candidate, not extra changed-item rows or detail
   payload leakage (`agents-plugin-tool/cmd/ws-mcp/tracker_projection.go:132`,
   `agents-plugin-tool/internal/wstracker/view.go:1214-1219`,
   `agents-plugin-tool/internal/wsdashboard/tracker_delta.go:202`).
2. **INFERRED (lead's remaining-cost assessment):** per-change server work
   still includes full view derivation and summary projection across the
   items, **O(items)** work that the checkpoint does not remove. The
   checkpoint saves prefix verdict checks/folding, not these full-view
   stages. Further server-time reduction remains a follow-up candidate.
3. **UNMEASURED:** browser render time remains a Phase 2 verification gate.
   No page/fallback/detail-ordering implementation or Phase 2 plan change is
   included here. w5090-specific measurements and peak-memory measurement
   also remain outstanding; the Mac results above are not substitutes.

### Phase 2: Page on summary and deltas

Switch the tracker page, handles and feed to the summary plus deltas with a
keyed local model and changed-row redraw; fetch detail bundles for expanded or
visible items and use a bundle only when its detail version matches its row;
fall back to the full view
when the new routes are missing.

Verification: the browser render-time rows of the verification constraint;
the fallback against a daemon without the new routes; and detail ordering
tests where a newer bundle arrives before its summary delta and an older
bundle arrives after it, asserting that neither mismatched bundle is used
and the row shows the matching detail once it arrives.

### Result (c0ceb4518) - 2026-11-18

- **VERIFIED:** merged `origin/develop` into the retained implementation branch
  with `--no-ff` (`6629e21b8`), without conflicts or generated-bundle edits.
  The requested Phase 1 lead gates passed after the merge:
  `cmd/ws-mcp -run Tracker -count=1` (`ok .../cmd/ws-mcp 13.871s`) and
  `internal/wstracker/...` (`ok .../internal/wstracker 23.667s`).
- **VERIFIED (implementation and tests):** the page, handles and localized
  bounded feed now start from `/summary`, then apply `/delta?since=` to a
  keyed in-memory model. Equal-cursor 304s redraw nothing; resets replace
  the model inline; removed/hidden stores retire their pending reads.
  Concurrent answers cannot regress the cursor or resurrect a hidden store.
  Missing summary, delta or detail routes select the retained legacy
  full-view/ETag path, including sibling-frame lending.
- **VERIFIED (Chromium and Node):** visible rows use clipped intersection
  observation and batched detail requests of at most 100 ids. Expanded items
  show loading/error/reload controls until the bundle's version equals the
  row's. Neither future-before-delta nor late-old-after-delta bodies,
  comments, events or HTML enter the rendered model. Matching held detail is
  installed with its delta. Failed reads do not spin; explicit retry works.
- **VERIFIED (Chromium):** canonical keyed-row reconciliation retains an
  unchanged row and its title node with **zero mutations**, keyboard focus,
  and an unsent composer across an unrelated delta. Correction `5673ea6ba`
  also preserves draft/caret/scroll across an invalidating detail-loading
  gap, and resumes a waiting editor from the matching post-reset body/base.
  **Boundary:** open panels still generate and parse their full HTML before
  reconciliation; this is changed-DOM redraw, not O(changed-items) render CPU.
- **Decision — preserve non-list functionality:** global Activity and sync
  history explicitly request the retained `/view` only when opened. These
  global events are not in per-item bundles and never satisfy an item's
  detail gate. Dispatch obtains actual event verdicts from item bundles,
  falling back to history for global/missing events instead of guessing
  `applied`. Failed Activity history offers an explicit same-cursor reload.
- **Decision — complete the page contract:** additive summary fields carry
  flagged-thread counts, historical label names and feed actors. Detail
  bundles include incoming comment references and direct reply parents,
  with those comments included in the version hash, so referenced snippets
  invalidate correctly. The summary still excludes bodies and thread-event
  payloads. Existing assignment `ended.type` supplies removed-member text.
  Per-item time projections drive due/recently-closed flags.
- **VERIFIED (independent lite review):** the one-pass review of
  `6629e21b8..c0ceb4518` found C1 (active search cached by the stable view
  object), I1 (history errors without retry), and I2 (color-only label edits
  erasing historical names). `5673ea6ba` fixes all three with regressions:
  search tracks the cursor and drops overtaken answers; history has bounded
  explicit retry; only actual string name edits replace historical names.
  Lite allocates no re-review; **no independent clean post-fix verdict is
  claimed**. The draft/loading and reset-editor fixes were self-review.
- **VERIFIED (compatibility/performance boundary):** T0 remains additive:
  legacy full-view routes and writes are unchanged; old daemons get the
  full-view fallback; cursor, models, bundles and new metadata are memory
  only. No persistence, filesystem operation or process spawn was added to
  a tracker read. Existing memory-answer host-call tests passed. **UNMEASURED:**
  syscall/file-activity counts and visible latency on Windows with heavy AV;
  source reasoning and host-call counters are not that measurement.

**Final verification at `5673ea6ba`:** every Go command used
`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` (the current task explicitly
superseded the historical no-local-Go constraint). Browser commands also used
`PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright` and the existing
`WS_PLAYWRIGHT_MODULE=$HOME/.npm/_npx/705bc6b22212b352/node_modules/playwright`.

```text
node --test tracker_projection_adapter.test.js tracker_projection_render.test.js
  tests 14; pass 14; fail 0; skipped 0
node testdata/tracker-projection.browser.cjs
  PASS tracker browser: summary/visible batches/keyed unchanged row + focus/draft
  PASS tracker browser: new page / old daemon full-view fallback
  PASS tracker browser: future-before-delta / old-after-delta / matching detail only / draft and caret across loading gap
go test ./cmd/ws-mcp -run Tracker -count=1
  ok github.com/kang-sw/devenv/cmd/ws-mcp 14.554s
go test ./internal/wsdashboard -run Tracker -count=1 (including Chromium)
  ok github.com/kang-sw/devenv/internal/wsdashboard 8.902s
go test ./internal/wstracker/...
  ok github.com/kang-sw/devenv/internal/wstracker (cached)
git diff --check
  no diagnostics
```

Earlier failed checks were diagnosed and corrected, not waived: extracted-JS
harnesses needed the real new helper dependencies; a new Go fixture used
`Ref` where `Ended` was required; live hover positioning initially caused an
unchanged-row mutation, fixed by comparing canonical row markup. The final
commands above passed after those corrections.

**Browser measurement — supporting Mac evidence only:** opt-in
`WS_TRACKER_BROWSER_BENCH=1` ran disposable Chromium on the Mac worker host
against baseline renderer `6629e21b8` and the Phase 2 checkpoint. Synthetic
10k/30k-item models each carry **1,000,000 events**. One sample per row, timing
synchronous changed-response **JSON parse + model apply + closed-handle render**;
fixture generation/stringification and network/server time are excluded.
The after path parses one changed summary row, not the full event history.
This is neither open-drawer/paint latency nor w5090/Windows-AV measurement.

| Items | Events | Before (ms) | After (ms) | Surface |
|---|---:|---:|---:|---|
| 10k | 1,000,000 | 677.8 | 0.2 | closed handles |
| 30k | 1,000,000 | 656.2 | 0.1 | closed handles |

**Forward / remaining verification:**

1. `globals.my` wholesale replacement on assignment/today reorder remains the
   Phase 1 **O(N) delta-payload gap**. Phase 2 consumes it unchanged; fixing it
   is not required for the page contract. Full-view/projection derivation is
   still O(items), as recorded in the Phase 1 Edition.
2. Large open-drawer/window render CPU (including full HTML generation and
   parsing), peak memory/checkpoint bytes, the specifically required w5090
   million-event measurements, and Windows heavy-AV file-operation/latency
   measurements remain **UNRUN**. SSH was forbidden; the local supporting
   measurements do not satisfy those host-specific gates. Lead/owner retains
   performance sign-off before release.
3. Global Activity/sync history still transfers a legacy full view on demand;
   a bounded global-history projection is future work, not silently removed
   functionality.

**Omitted:** no daemon replacement/build, Electron launch, SSH, broad
`internal/wsdashboard`/`internal/mcp` package run, or merge into develop.
`workflow_manual`, route resolution, acquire/close and playbook-render tools
were not exposed; bundled reviewer/ticket conventions and the declared
manuals were read instead. Phase 2 implementation is complete; ticket closure
uses the documented Git-move fallback. The retained branch is pushed for the
lead's review/integration, not merged. Ticket guardrails pass; the advisory
Sage-freshness warning is accepted for this Result/completion-date-only
update, with no phase-plan or settled-decision edits.

#### Edition (07ebb9c91) - 2026-11-18

**Independent-review corrections:** all four findings from the review of
`79383b9ee..1fcc5c50b` are fixed on the retained implementation branch.

1. **Critical — published-view ownership:** incremental refresh can replace
   existing node fields while the previous view is encoded. `view.go` now
   copies `Assignment.StatusOrigin` and every other borrowed fold/node
   pointer field, including dates, parent and membership references, plus
   event payload bytes. Returned snapshots own their data; calls on Tracker
   itself still require caller serialization. New tests assert snapshot
   stability after status replacement/direct fold mutation and concurrently
   encode a published view during repeated refreshes. The full wstracker
   package passes under `-race`.
2. **Important — store mutex across socket writes:** summary, delta, detail
   and 304 status/header/body construction finishes under the store lock;
   every HTTP write runs after unlock. A deterministic blocked-writer test
   holds the first response at a socket boundary while a second request
   publishes a new generation for that same store. It also verifies the
   first payload remains detached from the new maps. Normal and race runs
   pass.
3. **Important — recovery rereads on projection misses:** registration reads
   now reuse the existing stat/file-identity-validated small-file cache;
   registry writers retain fresh reads under the cross-process root lock.
   Seen state is recovered once per registration directory/nonce, returned
   as copied slices, and updated in memory only after successful persistence.
   Registration replacement cannot inherit the old clone's marks. Tests
   cover external relabel/hide/removal/re-registration, caller mutation,
   restart recovery, failed recovery retry and failed-write isolation.
   **Correction to the earlier Result's I/O assessment:** projection misses
   did reread both `stores.json` and `ui-seen.json`; that read chain was not
   I/O-free. This Edition fixes those repeated recovery reads, not all ledger
   or host-sync I/O. Registry validation still performs a stat, and the
   existing cache conservatively rereads recently modified/coarse-timestamp
   registry files; seen reads need no file touch after recovery, including
   immediately after this daemon writes new marks.
4. **Important — downgrade loading gap:** selecting legacy mode retains the
   summary model and its detail gate until a valid full view is installed.
   A slow or failed `/view` cannot expose an unloaded item's absent
   attachments/events to the window renderer. Explicit retry remains safe;
   all editor continuations, including those added while waiting, settle
   only after installation. Node and Chromium regressions cover both a
   missing delta route and a missing detail route, delayed/failed legacy
   reads, opening an unloaded item while waiting and eventual full windows
   without exceptions.

**VERIFIED — supporting file-operation measurement:** fixture counters run
five instances of the former Seen recovery-read path and five warm production
Seen calls. Before: **10 `ReadFile` attempts**. After: **0 `ReadFile` attempts
and 5 registry-validation `Stat` attempts** (two reads per call → zero reads
plus one stat). The post-write read also performs no recovery reread. These
are instrumented Go operation attempts for this component, not a whole-delta
kernel syscall trace, Windows-heavy-AV measurement or visible-latency claim.
The persistence format and legacy readers/writers are unchanged: **T0**.

**VERIFIED — final narrow gates:** every Go command used
`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; Chromium used the browser cache
and existing Playwright module recorded in the Phase 2 Result.

```text
go test -race ./internal/wstracker/... -count=1
  ok github.com/kang-sw/devenv/internal/wstracker 74.654s
go test ./cmd/ws-mcp -run Tracker -count=1
  ok github.com/kang-sw/devenv/cmd/ws-mcp 13.696s
go test ./internal/wsdashboard -run Tracker -count=1 (including Chromium)
  ok github.com/kang-sw/devenv/internal/wsdashboard 8.999s
go test -race ./internal/wsdashboard -run '^TestTrackerProjectionSlowSocketDoesNotBlockSameStore$' -count=1
  ok github.com/kang-sw/devenv/internal/wsdashboard 2.058s
go test -race ./internal/wstrackerhost -run 'TestTrackerReadStateCache|TestSeen' -count=1 -v
  PASS; ok github.com/kang-sw/devenv/internal/wstrackerhost 5.125s
node --test internal/wsdashboard/tracker*.test.js
  tests 22; pass 22; fail 0; skipped 0
node internal/wsdashboard/testdata/tracker-projection.browser.cjs
  PASS summary/visible batches/keyed unchanged row + focus/draft
  PASS new page / old daemon full-view fallback
  PASS missing delta / delayed legacy view / unloaded window gated / failed read retry / full window without exceptions
  PASS missing detail / delayed legacy view / unloaded window gated / failed read retry / full window without exceptions
  PASS future-before-delta / old-after-delta / matching detail only / draft and caret across loading gap
git diff --check
  no diagnostics
```

A fresh read-only source audit of the ownership and metadata-cache changes
found no remaining correctness defect in that targeted scope; the auditor
ran no tests. **Unfixed findings: none.** The earlier Forward work and
host-specific performance/open-large-panel/peak-memory gates remain unrun;
no new benchmark or allocation-cost improvement is claimed for the owned
payload copies. No daemon replacement, Electron, SSH or develop merge ran.

## Sage Review Round 1 (2026-11-17)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Earliest-position changes require full replay | important | missing |
| 2 | Summary-only diffing can leave detail permanently stale | important | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
