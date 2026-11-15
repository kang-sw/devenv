---
title: "Keep dashboard streams through transient board or session read failures"
related:
  261114-chore-ws-dashboard-index-removal-diagnostics: "missing timestamp/reason telemetry limits historical attribution"
  261114-bug-ws-shared-daemon-restarts-without-a-swap: "separate daemon-replacement investigation; the observed process did not restart"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a287b6f219541be6
sage-review-completeness-reviewed: a287b6f219541be6
---

# Keep dashboard streams through transient board or session read failures

## Background

During read-only dogfooding on 2026-11-14, MAIN repeatedly saw the localized
`js.banner.gone` message while shared daemon PID 94122 remained unchanged.
Session removal/readdition pairs survive in the dashboard log, but their
missing timestamps and reasons do not identify a failing record or writer.
The later bounded registration/key snapshot was valid and matched.

MAIN's 2026-11-14 23:22:25 UTC investigation relay, board item (829),
identified both the index-removal path and the terminal client behavior.
The following source trace was independently checked at
`07c94a0954dac86607bf20d524124b2bd4bc5c54`; paths below are relative to
`agents-plugin-tool/`:

- `internal/wswhiteboard/folders.go:101-123` opens the authoritative board
  under `FolderStore.locked`. The exclusive folder lock has a two-second
  acquisition timeout (`:167-190`). `UpdateAndPublish` holds that lock through
  mutation, commit and its publication callback (`:138-165`). The previously
  traced board-output callback includes rendering/publication work; no lock
  boundary change has been implemented or selected here.
- `internal/mcp/session_layout.go:278-281` propagates an `openWhiteboard`
  error, and `internal/mcp/whiteboard_store.go:54-57` wraps `FolderStore.Open`
  failures. `internal/mcp/dashboard_sessions.go:879-895` `lookupRecord`
  collapses every `loadRecordWith(partAll)` error, absent candidate and
  identity mismatch into a false result after its two lookup attempts.
- `DashboardSessionSource.ReadSession` returns false on that result
  (`internal/mcp/dashboard_sessions.go:785-789`). `Index.apply` treats false
  as removal and deletes an existing indexed session
  (`internal/wsdashboard/index.go:381-395`). `ForgetSession` clears record,
  board, history and snapshot caches
  (`internal/mcp/dashboard_sessions.go:732-748`). `readLane` already retains
  an entry on an unreadable registration unless removal was requested
  (`internal/wsdashboard/index.go:351-366`), but this protection does not
  extend to an unreadable session record.
- On an already-open page stream, a not-ok re-render sends `gone`
  (`internal/wsdashboard/sse.go:468-474`). A first-render not-ok result is
  instead HTTP 404 (`:437-439`); MAIN also traced the other initial request
  check at `:244-246`. Initial failure does not itself send the open-stream
  `gone` event.
- `internal/wsdashboard/assets/dashboard.js:6374` registers `onGone` for
  `gone`. `onGone` closes the stream, marks it disconnected and displays
  `js.banner.gone`, without reconnecting (`:6437-6441`). Ordinary transport
  loss instead schedules `connect` after three seconds (`:6370-6372`).
  These client coordinates supersede the earlier relay's pre-drift lines.

Two separate gaps follow: fixing only the stream/page check can retry against
an index that already discarded the session, and the current client treats
`gone` as final without re-checking. A daemon restart or swap is not required
for this source-backed path; the reported incident retained its daemon PID.
This is not proof that every historical banner had this cause or that a
restart can never accompany another removal. Two lookup attempts may both fail
under continued contention, but no timed incident trace proves that they did.
Normal record writes are per-file atomic, and authoritative folder reads and
writes share their lock; no partial authoritative commit is needed to explain
the identified path.

## Decisions

- MAIN's (829) technical investigation relay on 2026-11-14 confirms the
  **T0 three-way read result**: found, truly absent/identity-mismatched, or
  transient error. Carry lock/I/O error classification through the internal
  source/index/page path instead of collapsing every error to false.
- On a transient error, retain the indexed session and schedule a re-read;
  page rendering keeps the last good content. Preserve genuine deletion and
  identity-mismatch handling rather than keeping removed sessions forever.
- The client re-checks `gone` with one reconnect after a few seconds and
  displays the gone banner only when that reopen returns 404. Do not turn
  transient record failures into permanent client termination or rely on a
  stream-only fix after index removal. The exact implementation must preserve
  the existing transport contract and is verified in isolated fixtures.
- **Sequencing superseded (MAIN, 2026-11-14 23:41:35Z).** MAIN reassigned
  this ticket from EXEC3 to EXEC4 with "Promote and implement", replacing the
  earlier 23:22:25 UTC placement after selection-bundle
  (`261114-feat-ws-selection-bundle-comment`). That placement only serialized
  EXEC3's own queue; there is no source dependency on selection-bundle, so
  implementation starts on current `develop` and whichever change lands
  second reconciles overlapping files.
- Moving expensive render/publication outside the folder lock is a design
  review topic, not a selected implementation. Preserve ordered publication
  and atomic authoritative state if the lock boundary changes.

## Compatibility analysis

Target **T0** if error classification remains internal and existing stream,
registration and session identity contracts remain intact: `gone` continues
meaning genuine absence/mismatch, while transient reads use the existing
retry/keep-page path. No new wire field or event is required by the approved
direction. A new event/field would require additive old-reader handling and
explicit review; reinterpreting a current protocol, store or event is outside
this target and stops for MAIN's settlement before implementation.

## Constraints

- Work only in an isolated worktree with isolated fixtures and state. No
  live lock hold on the shared daemon, process signal, restart or daemon swap;
  never touch the shared daemon or port 29417. Run focused tests with
  `go test -run` only, never the whole `./internal/wsdashboard` package, and
  end every test run with a scan for processes the run started.
- Preserve the approved index-retention, scheduled re-read, last-good page
  and one delayed client re-check behavior. Ground the precise retry delay and
  last-good-content lifetime in the implementation review and deterministic
  tests; do not invent persistent cache fields or change the wire contract.
- Moving rendering/publication out of the folder lock is separate design
  work requiring ordered-publication review. It is not a prerequisite or an
  implicitly authorized optimization in this repair.
- `261114-chore-ws-dashboard-index-removal-diagnostics` remains diagnostics
  only. Its missing historical timestamps/reasons are not repaired by source
  reasoning or by this ticket amendment.
- Investigate render frequency and contention separately from CPU attribution:
  the short OS sample showed rendering work, not its share of sustained CPU.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before future tooling edits.
  Settle the compatibility tier before any changed stream/validity contract.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261110-bug-ws-dashboard-daemon-read-amplification (2026-11-10, commit): "loadRecord gained a recordFileReader seam (loadRecordWith) instead of a second copy of the split/legacy layout logic" — bearing: constrains
- 261007-bug-session-record-cross-process-write-race (2026-11-08, commit): "Migration is lazy on the first write, never on read" — bearing: constrains
- 261111-refactor-ws-minimize-recurring-fs-touches (2026-11-11, commit): "an unreadable-but-present record never stops a live session." — bearing: supports
- 89eb4b89 (2026-11-10, commit): "one copy of the stat-before-read / equality / never-cache-failures rules instead of two." — bearing: constrains
- 261112-bug-ws-dashboard-open-tab-keeps-stale-assets-after-daemon-upgrade (2026-11-12, commit): "The event is sent on every stream, region-filtered ones included" — bearing: constrains
- 3ec591d8 (2026-11-14, commit): "old streams retain full region events and the first opted-in board send is full even if h matches." — bearing: constrains
- 7804f5f4 (2026-11-12, commit): "The Index reads it on every re-read into Session.Label, so a change raises Session.Rev" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/dashboard_sessions.go, session_layout.go, whiteboard_store.go; internal/wsdashboard/index.go, sse.go, assets/dashboard.js |
| scope.surface | cross-module | DashboardSessionSource.ReadSession supplies the wsdashboard source/index contract from mcp. |
| scope.new_public_symbol | unknown | No new named symbol is specified; the internal three-way result may require an exported cross-package type. |
| scope.new_type_contract | yes | Phase 1 replaces the boolean source/index/page read classification with found, absent/mismatched and transient outcomes. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/index_test.go, index_lanes_test.go, conversation_history_test.go; internal/mcp/dashboard_sessions_test.go; internal/wswhiteboard/folders_test.go |
| complexity.reuse_points | confirmed | FolderStore.Open/UpdateAndPublish, lookupRecord/loadRecordWith, Index.readLane/apply, ForgetSession and servePageStream were inspected. |
| complexity.side_effect_risk | high | A misclassification removes sessions and clears source/live/render caches through daemon.go#L535-L550. |
| risk.correctness | high | Retain transient failures without masking true deletion or identity mismatch; distinguish initial 404 from open-stream gone. |
| risk.fit | moderate | The three-way read must fit existing cache, lane serialization and topic-stream behavior without changing wire semantics. |
| risk.test | high | Controlled lock contention, malformed records, permission errors and delayed client recovery require deterministic isolated fixtures. |
| risk.security_or_contract | high | Retained pages must not survive genuine identity invalidation; preserve existing stream and session-identity contracts. |

## Phases

### Phase 1: Distinguish transient reads and preserve the valid reader

On current `develop`, use isolated state and controlled contention to establish the complete record-error to
index-removal to open-stream-gone path without touching the shared daemon.
Implement the approved internal found/absent-or-mismatched/transient result
through the source, index and page read paths, preserving error classes.
Transient reads retain the index entry, schedule another read and preserve the
last good page. Add the single delayed client reopen check before displaying
the gone banner on a confirmed 404. Preserve true absence/mismatch behavior.
Do not move rendering/publication outside the folder lock or introduce a new
stream event, persisted cache shape or protocol change; return any necessary
scope expansion to MAIN before implementing it.

Verification must distinguish genuine deletion and identity mismatch from
lock contention, malformed/unreadable records and permission/I/O errors.
Exercise both registration and record unreadability, and assert that transient
record errors do not remove the index entry or clear its valid caches. Cover
first-render 404 separately from not-ok re-render on an open stream, the
client's one delayed reopen, retention of last-good content during a transient
failure, and successful recovery when the read becomes available. Confirmed
absence must still reach the banner rather than retry forever. Prove that no
partial authoritative commit or daemon restart is needed for the reproduced
failure. Record exact source/fixture pins, commands, process quiescence and
historical confidence limits; no implementation Result is claimed here.

#### Implementation checkpoint - 2026-11-15 (review pending)

- Recovered the interrupted server implementation on
  `impl/exec4/busy-board-read-gone/wrist-snuff-visor`, keeping the client
  leaf at `5015e6747`. Finished the Go source/test caller migration to the
  internal `(value, found, error)` contract. No wire event or persisted field
  changed (T0).
- Record and key-scan failures preserve their I/O, permission, decode or
  lock-timeout causes. Readable identity mismatches remain terminal even
  behind a held board lock. Index failures retain existing entries and
  caches, including ended entries, and retry on the existing 30-second
  polling backstop. Open streams keep their shared last-good render until
  recovery, confirmed invalidity or the last subscriber disconnects; failed
  renders retry on the existing 15-second refresh tick. Initial transient
  requests return 503 instead of 404. The committed client leaf waits three
  seconds, probes the stream route, and banners only on confirmed 404;
  unsuccessful non-404 probes keep the same delayed retry cadence.
- Fixture pins: `internal/mcp/dashboard_transient_read_test.go` uses a
  temporary authoritative folder `.lock`, compares committed `meta.json`
  bytes before/after contention, verifies valid source caches survive, and
  exercises successful recovery and identity mismatch. It starts no daemon
  and performs no partial authoritative write. Classification cases include
  malformed key/part, directory-read I/O failure, actual mode-000 permission
  denial, absence and scan failure; all ran without skips on this host.
  `internal/wsdashboard/transient_read_test.go` covers registration decode
  and I/O failures, genuine removal/mismatch, ended-session retries, shared
  last-good renders, open-stream recovery/gone and initial 503 versus 404.
- Verification from `agents-plugin-tool/`, with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` for every Go invocation:
  - `go test ./internal/wsdashboard ./internal/mcp -run
    '^(TestTransientRead|TestDashboardTransient)' -count=1 -timeout=20s -v`:
    passed both packages; complete final output read, no skips.
  - `go test ./internal/wsdashboard -run
    '^(TestIndexRefreshTracksTheRegistry|TestRegistrationRoundTripIgnoresUnknownFields|TestSessionLeavingTheIndexDropsItsCachedEntries|TestSessionSourceForgetsARemovedSession|TestStreamsOfOnePageShareOneRender|TestStreamSendsOnlyTheLiveRegionForAStreamingTick|TestSessionPageFindingSpinnerUntilRegistered)$'
    -count=1 -timeout=20s`: passed.
  - `go test ./internal/mcp -run
    '^(TestDashboardSourceSeesBothLayouts|TestDashboardSourceRescanBlocksNoKnownLookup|TestDashboardReadPageServesTheBoard|TestDashboardReadPageLangTranslatesBoardAndPanelText|TestDashboardReadSessionCountsOpenAskItems|TestDashboardSessionFingerprintTracksTheInbox|TestAgentKeyStoreFindsTheLeadSessionBehindARow)$'
    -count=1 -timeout=20s`: passed.
  - `go test ./internal/wsagent -run '^$' -count=1 -timeout=20s`:
    compilation passed; no agent/daemon tests executed.
  - `node --test internal/wsdashboard/testdata/stream-gone.test.cjs`:
    all eight cases passed.
  - `git diff --check` and `gofmt -l` over changed/new Go files: clean.
  Every test command ended with a process scan; no started test, compiler
  or Node processes remained. Shared daemon/port 29417 were not touched;
  no process was signalled. Historical attribution remains unproven: these
  isolated fixtures establish the failure mechanism, not its incident rate.
- Pending: route-allocated independent lite review and final action gate.
  This print-mode invocation exposes asynchronous Pi spawn/send tools but no
  synchronous `pi.wait` or equivalent blocking agent tool. No child was
  spawned, because ending this turn with a child outstanding would repeat
  the reported interrupted-run failure. The lead must provide a blocking Pi
  review path or resume under a persistent harness; the phase has no Result
  and the ticket remains ready until independent review is resolved.
