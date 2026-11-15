---
title: Dashboard wakes reuse unchanged reads instead of reopening files
related:
  261115-refactor-ws-dashboard-stream-whiteboard-reread: "prior work; its FolderCache is the hit path made cheaper here"
  261115-feat-ws-dashboard-board-topic-transport: "sibling; render/tokenize cost, not read cost"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 57d0789d11fbfff3
sage-review-completeness-reviewed: 57d0789d11fbfff3
completed: 2026-11-15
---

# Dashboard wakes reuse unchanged reads instead of reopening files

## Background

After the stream reread fix (C, merged `aa07206c0`) and the metadata-write
elision (merged `74f573faf`; not an ancestor of this checkout, whose
`agents-plugin-tool/internal/wswhiteboard/folders.go#L682-L686` still
schedules every topic's metadata) (pending 261115-refactor-ws-dashboard-stream-whiteboard-reread), the live daemon
still burns CPU on every stream wake. Read-only profile of daemon pid 30242 (develop `74f573faf`,
2026-11-15 17:28 KST, 20s, raw `/tmp/exec5/perf2/cpu20.pb.gz`,
machine-local): 17.35s of samples (86% of one core); `ps` showed ~156%
over the first minutes after the swap. 66% sits under
`Daemon.stream -> pageCache.get`:

- `wslive.ReadObserved` through `FileCache[[]wslive.ObservedRecord].Load`:
  27.4% cumulative, `os.Open`-heavy (saved profile above, `go tool pprof -top -cum`); `State.SeedObserved` 11% and
  `OverlayObserved` 9%; reached from `SnapshotCache.Snapshot` (42%) for
  session and rail events.
- `wswhiteboard.FolderCache.Open` hit path: 13.5%, of which
  `State.clone` is 13.2%; the encompassing `FolderStore.withGuard`
  is 15%, including the clone as well as flock/stat work, not an additive
  cost (saved profile above; `agents-plugin-tool/internal/wswhiteboard/folders.go#L187-L215`).
- `pageReader.indexStamp` 12.8% and `sessionStamp` 10%.
- `syscall.rawsyscalln` is 39% flat, mostly `os.Open` (25%).

Board-transport (`261115-feat-ws-dashboard-board-topic-transport`)
addresses render and tokenize cost; this ticket addresses the read cost
that every wake pays before any render. Promoted by MAIN on 2026-11-15
(dogfood rule (1058)): scope is exactly the three items below.

## Decisions

- (W1) Scope: (a) observed records, (b) the FolderCache hit, (c) the
  session/index stamps. Source: MAIN, 2026-11-15. Rejected: folding these
  into board-transport (different cost centre and files).
- (W2) Observed records: on a wake, each observed-record file under
  `model-evidence/` (the directory enumerated by
  `agents-plugin-tool/internal/wslive/observed.go#L300-L327`) whose stat
  identity (same file, size, mode, mtime - the identity the FolderCache
  already uses) is unchanged is neither opened nor decoded; the cached
  decoded records, and the work derived from them per wake
  (`SeedObserved` / `OverlayObserved` inputs), are reused. A changed or
  vanished file is re-read exactly as today. Rejected: time-based TTL
  caching (can serve stale state; the chosen identity detects atomic
  replacements, not in-place edits preserving identity, size, mode and
  mtime: `agents-plugin-tool/internal/wswhiteboard/folders_cache.go#L30-L46`).
  Today `SnapshotCache` instead applies `FileCache` to the directory itself,
  not each record; that cache checks only size/mtime and declines to retain
  recent reads for two seconds
  (`agents-plugin-tool/internal/wsdashboard/snapshot_cache.go#L125-L131`;
  `agents-plugin-tool/internal/wsdashboard/file_cache.go#L40-L86`).
- (W3) FolderCache hit returns a shared read-only `State` instead of
  `clone()` per open, enforced by copy-on-write plus a `-race` test, not by
  convention. Go cannot freeze exported fields (`State` exposes writable
  fields/slices/maps, `agents-plugin-tool/internal/wswhiteboard/store.go#L249-L295`;
  `clone` is package-private, `#L359-L375`), so:
  - the cache marks the `State` it hands out as shared (an unexported
    flag);
  - every in-package mutation entry point that can receive a cached value
    (`BeginRender`, the package-level `Apply`, `#L563-L567`, `#L603-L625`,
    and any other mutator the worker finds) first replaces a shared value
    with its own `clone()` and mutates that copy (copy-on-write), so a
    caller that mutates never touches the cached value;
  - callers outside the package that today write fields of an opened
    `State` directly are moved to a mutating entry point or an explicit
    copy; the worker lists each such site in the Result;
  - a `-race` test runs concurrent dashboard-path readers of one cached
    `State` while each mutating caller path (whiteboard update/commit,
    render) runs, and asserts no race and that the cached value's digest is
    unchanged afterwards. The existing cache test that mutates
    `got.Topics[0].Body` (`folders_cache_test.go#L40-L54`) is rewritten to
    go through a mutating entry point and assert the cache stays intact.
  Source: MAIN, 2026-11-15 (copy-on-write or a frozen type checked by a
  `-race` mutation test). Rejected: panicking frozen methods alone
  (exported fields stay writable, so it would be convention after all); a
  separate read-only wrapper type (wider API churn for the same
  guarantee).
- (W4) Stamps: `indexStamp` and `sessionStamp` stop opening files on each
  wake; they derive from stat identity or from the cached reads above,
  keeping today's change-detection semantics (a real change still moves
  the stamp; no change keeps it). Current semantics include clock-based
  session/adapter expiry, watched-process liveness, contact liveness and
  the session's mail print, not just file metadata
  (`agents-plugin-tool/internal/wsdashboard/pages.go#L1029-L1091`).
  Watched directories are listed to detect in-place child edits even when
  the directory stat is unchanged
  (`agents-plugin-tool/internal/wsdashboard/stream_cache.go#L256-L310`).
- (W5) Compatibility T0: no on-disk store shape, MCP tool, `/mcp` hello,
  stream or page change; caches are in-process only. Source: MAIN.
  Under that scope, old/new writers keep `AppendObserved`'s immutable JSON
  records (`agents-plugin-tool/internal/wslive/observed.go#L246-L295`) and
  `FolderStore`'s intent/publication bytes unchanged
  (`agents-plugin-tool/internal/wswhiteboard/folders.go#L711-L807`);
  old readers still use `ReadObserved` and `FolderStore.load`, and a new
  process starts with empty caches and reconstructs through those same
  readers (`observed.go#L300-L341`, `folders.go#L306-L324` in those respective
  directories). This is the scoped T0 requirement, not proof of an
  implementation that has not run yet.

## Constraints

- `AGENTS.md` Implementation Conventions: `shipped-surface-boundary.md`
  and `release-compatibility.md` apply to `agents-plugin-tool/internal/`;
  `ws-mcp.md` to `agents-plugin-tool/internal/mcp/`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Never touch the shared dashboard daemon in verification; measure on
  disposable fixtures in-process.
- Do not run the full `./internal/wsdashboard` test package; use `-run`.
  Tests run with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.

## Prior Decisions

- 261115-refactor-ws-dashboard-stream-whiteboard-reread / 5cfa11b6 (2026-11-15, commit): "Use a bounded, per-board in-process cache rather than suppressing legitimate live-region updates or depending on process-local notifications." — bearing: supports
- 261110-bug-ws-dashboard-daemon-read-amplification (2026-11-10, Result): "A symlinked entry is followed with `os.Stat`." — bearing: constrains
- 261110-bug-ws-dashboard-daemon-lock-scope / 291bb82c (2026-11-10, commit): "broker.wake is no longer ordered behind the re-read. The page stream reads the live files itself, and the index change wakes it again." — bearing: constrains
- 261109-bug-ws-dashboard-stream-wake-rerender-cpu / dc8d5566 (2026-11-09, commit): "PageContent.Watch and ContactSource are small additions to the page source contract: the mcp source is the only one that reads files no nudge reports, so it names them." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/{snapshot_cache,file_cache,pages,stream_cache}.go, internal/wslive/observed.go, internal/wswhiteboard/{folders_cache,store}.go, and MCP dashboard consumers/tests |
| scope.surface | cross-module | exported wswhiteboard.FolderCache.Open returns State to mcp; wsdashboard.SnapshotCache consumes wslive observed evidence |
| scope.new_public_symbol | unknown | W3's writable-copy access is not named; existing State.clone is package-private, so an external caller cannot invoke it directly |
| scope.new_type_contract | yes | W3 changes the existing exported FolderCache.Open/State result from independent mutable copies to enforced shared read-only state |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/folders_cache_test.go, internal/wslive/observed_test.go, internal/wsdashboard/{file_cache,observed_snapshot,stream_cache,stream_cache_idle}_test.go, internal/mcp/dashboard_folder_cache_test.go |
| complexity.reuse_points | confirmed | FolderCache identity validation/flock; FileCache stat-before-load and ForgetUnder; SnapshotCache read/SetLive/Forget; ReadObserved and OverlayObserved; StampWatched |
| complexity.side_effect_risk | high | shared State pointers and derived observed summaries cross concurrent readers; cached values must not be mutated or survive invalidation incorrectly |
| risk.correctness | high | directory-only evidence stamps miss child edits; frozen method guards alone cannot protect exported fields; clock/liveness/mail changes must still invalidate |
| risk.fit | high | W2 crosses wslive and wsdashboard; W3 changes a mutable cross-package State contract while rejecting a wrapper; W4 must retain watched-child listing semantics |
| risk.test | high | current FolderCache test requires writable independent results; new mutation/race and exact open-count coverage must replace/extend that baseline using isolated fixtures |
| risk.security_or_contract | moderate | T0 excludes wire/disk changes, but frozen State must remain an in-process marker and public mutation paths such as Apply must not corrupt shared cache state |

## Phases

### Phase 1: Reuse unchanged reads on wakes

First record a before profile on a fixture shaped like the live case
(~112-topic folder board, observed-records files for several sessions, a
rail with those sessions, and repeated live-only wakes): CPU per wake,
file opens per wake, and an in-process pprof top list. Then implement
W2-W4 and record the after numbers on the same fixture in the Result
(table plus pprof top). Verify:

- regression tests that an unchanged wake performs zero opens of
  observed-records files and of the files behind the stamps (count via an
  injected opener or a test hook), and that a changed file is re-read and
  moves the stamp;
- the W3 `-race` test (concurrent readers, frozen mutation guard);
- existing focused tests: `wswhiteboard -run 'Folder|Open|Store'` with
  `-race`, `wslive` focused observed tests, `wsdashboard -run
  'Refresh|Stream|Snapshot|Rail|Stamp'`, `mcp -run 'Whiteboard|Dashboard'`
  (known red `TestWhiteboardEffectsKnownCreationBatchAndLegacySeparation`
  excluded).

### Result (82245d9fa) - 2026-11-15

Implemented W2-W4 in `82245d9fa`; the isolated test-cleanup repair is
`e9036098e`. No shared daemon operation, installation, plugin-runtime build,
version bump, merge, or push was performed.

#### Read and mutation contracts

- `wslive.ObservedCache` lists each evidence directory, checks each record's
  same-file identity/size/mode/mtime, and reuses unchanged decoded records and
  their derived model-history projection. Changed records alone are opened;
  deletion, recreation, malformed records, atomic same-stamp replacement,
  concurrent readers, and `Forget` have regressions. Snapshot overlays retain
  newer live evidence and never mutate a previously published projection.
- Folder-cache hits no longer deep-clone. They return a small, caller-owned
  `State` header sharing frozen backing data, with the unexported shared flag.
  A literal shared pointer would race at `Apply`'s final `*state = *work`;
  separate headers allow its existing private work copy to detach safely.
  `BeginRender` clones shared backing before changing it. Other in-package
  mutators operate on Apply's private work or a fresh folder transaction.
- External field-writer audit: `mcp/whiteboard_tools.go`'s `renderWhiteboard`
  (`Embeds`, `LastEmbedSeq`) and `evictWhiteboardEmbeds` already receive the
  explicit private clone from `wswhiteboard/folders.go:151-159`. They do not
  receive cached read generations, so no caller migration was needed.
  `DashboardSessionSource`'s cached consumers only query/build/render views.
  Both package-level and actual MCP update/implicit-render race tests retain
  a cached generation during mutations and assert its serialized digest is
  unchanged afterwards.
- Session/index stamps read only cached snapshot freshness, not observed-model
  overlays. Snapshot, reply-registry and contact-scope reads use
  `FileCache.LoadIdentity`, including recently written files; other
  `FileCache.Load` consumers keep their old recent-write behavior. Clock-based
  adapter/session/contact expiry, watched PID checks, mail prints and watched
  child-directory listings remain live inputs. Only two cache-load lines below
  the render code changed in `mcp/dashboard_page.go`; no render/transport
  restructuring overlaps the sibling ticket.

#### Same-fixture before/after evidence

Machine-local raw profiles, binaries, output, cumulative tops and methodology:
`/tmp/exec5/wakereads/{before,after}.{cpu,test,txt,top.txt}` and `README.md`.
The retained `BenchmarkWakeReads112` uses a disposable 112-topic folder board
(one history entry each), four synthetic index/rail sessions with 32 observed
records each, hosted snapshots, and 2,000 repeated read-side wakes. A wake
opens the unchanged board, reads the four snapshots, and computes session and
index stamps; rendering/transport is deliberately excluded.

The baseline uses the same fixture/wake body on `c2c18c07b`, with its old
observed-directory `FileCache` clock frozen at fixture creation. This holds the
recent-write window active throughout the run instead of mixing fresh and
aged cache behavior after two seconds. It measures the active/recent-write
case, not every aged/static workload.

| Per wake | Before | After |
| --- | ---: | ---: |
| Wall time | 22.005 ms | 8.783 ms |
| Approximate sampled CPU | 14.37 ms | 2.95 ms |
| Observed-record opens | 384 | 0 |
| Snapshot-file opens (hosted fixture) | 0 | 0 |
| Allocated bytes | 2,158,238 | 285,815 |
| Allocations | 20,779 | 1,829 |

Baseline record opens are inferred from the counted directory-cache misses
multiplied by 32 valid fixture files; after opens are counted at the record
opener. Separate injected-opener regressions check unchanged record/snapshot
opens and exact changed-file rereads. Directory listings are excluded from
record opens: evidence listings fall from 12 to four per wake; the board still
opens its flock once and validates its authoritative files.

CPU/wake divides the wake closure's cumulative CPU samples (28.74 s before,
5.89 s after) by the 2,000 timed wakes, including a small warmup contribution.
The profiles also include fixture construction, so their total sampled CPU is
not used as CPU/wake. Wall time falls about 60%; sampled wake CPU about 79%.
Cumulative pprof tops (percentages overlap, not additive):

| Site | Before | After |
| --- | ---: | ---: |
| `SnapshotCache.Snapshot` | 80.03% | 45.11% |
| `ReadObserved` / `ObservedCache.Load` | 76.09% | 45.11% |
| `FolderCache.Open` | 11.10% | 39.08% |
| `sessionStamp` | 26.65% | below retained top threshold |
| `indexStamp` | 25.85% | below retained top threshold |
| `os.Open` | 65.63% | 1.58% |
| `os.Stat` | see raw top | 75.14% |

The remaining profile is chiefly identity stats, which the chosen contract
requires. The host terminated the first after-profile command at its background
session limit; the resumed command produced the complete after profile.

Reproduction (inside `agents-plugin-tool`, with the environment below):

```sh
go test ./internal/wsdashboard -run '^$' -bench '^BenchmarkWakeReads112$' \
  -benchtime=2000x -count=1 -benchmem \
  -cpuprofile=/tmp/exec5/wakereads/after.cpu -o /tmp/exec5/wakereads/after.test
go tool pprof -top -cum /tmp/exec5/wakereads/after.test /tmp/exec5/wakereads/after.cpu
```

#### Verification and review

Every Go command used `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.
All of the following passed; full output is under `/tmp/exec5/wakereads/`:

- `go test -race ./internal/wswhiteboard -run 'Folder|Open|Store' -count=1`
  (`whiteboard-race.txt`), including unchanged-digest readers concurrent with
  Apply, BeginRender and folder update/commit.
- `go test -race ./internal/wslive -run Observed -count=1`
  (`observed-race.txt`).
- `go test ./internal/wsdashboard -run 'Refresh|Stream|Snapshot|Rail|Stamp' -count=1`
  (`dashboard-focused.txt`); the whole package was never run.
- `go test ./internal/mcp -run 'Whiteboard|Dashboard' -skip '^TestWhiteboardEffectsKnownCreationBatchAndLegacySeparation$' -count=1`
  (`mcp-focused-after.txt`). Its first run exposed an existing timer/TempDir
  teardown race in `TestDashboardRepoReadOncePerHeartbeat`; `e9036098e` stops
  and drains that test's own server before cleanup, and the rerun passes.
- `go test -race ./internal/mcp -run '^TestDashboard(StampReads|FolderShared|RepoReadOncePerHeartbeat)' -count=1`
  (`mcp-new-race.txt`), covering real MCP update/commit and implicit rendering
  against concurrent dashboard readers.
- `go test -race ./internal/wsdashboard -run 'FileCache|WakeStamp|ObservedSnapshot' -count=1`
  (`cache-stamp-race-final.txt`), including zero unchanged opens, same-size/time
  atomic replacement, expiry and cache forgetting.
- `go test ./internal/wsdashboard -run '^TestWakeReadsBrowser$' -count=1 -v`
  with the assigned `WS_PLAYWRIGHT_MODULE` and `PLAYWRIGHT_BROWSERS_PATH`
  (`wake-chromium-after.txt`): disposable Chromium at 1440px and 420px, 112
  topics, three live-only wakes, changed child evidence visible in the tree,
  unchanged board DOM identity, and no page errors.
- Lite correctness/test-integrity review was performed by the worker itself
  because the task prohibits subagents; no introduced Critical/Important
  findings remain. Details: `/tmp/exec5/wakereads/self-review.md`.

Pre-existing browser findings reported to the lead, outside this read-path
scope: (Important) `dashboard-conversation.js:71`'s entries merge ignores
sibling `.modelhistory`, leaving its history stale after SSE; (Minor)
`selection-note.browser.cjs:27` waits for visible `#ctext`, now a hidden
`.wsc-backing` textarea. The existing selection-note browser check failed on
that stale selector. The first read/wake smoke exposed the history merge
omission; the retained regression checks changed child models through the
correctly refreshed tree region. Neither JavaScript/render issue was changed.

#### T0 confirmation

No disk/wire shape changed. Old/new writers retain `AppendObserved`'s immutable
JSON publication (`wslive/observed.go:254-298`) and the same folder intent and
publication bytes (`wswhiteboard/folders.go:711-807`, unchanged). Old readers
continue reading those bytes through `ReadObserved` and `FolderStore.load`;
new empty caches reconstruct via the same bounded record decoder
(`observed.go:304-341`, `observed_cache.go:50-126`) and folder reader
(`folders_cache.go:110-128`). The State marker is unexported and never JSON.
No data is overwritten or lost when old/new versions alternate, and reopening
requires no migration or recovery beyond loading those unchanged stores.
