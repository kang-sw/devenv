---
title: Dashboard wakes reuse unchanged reads instead of reopening files
related:
  261115-refactor-ws-dashboard-stream-whiteboard-reread: "prior work; its FolderCache is the hit path made cheaper here"
  261115-feat-ws-dashboard-board-topic-transport: "sibling; render/tokenize cost, not read cost"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 57d0789d11fbfff3
sage-review-completeness-reviewed: 57d0789d11fbfff3
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
