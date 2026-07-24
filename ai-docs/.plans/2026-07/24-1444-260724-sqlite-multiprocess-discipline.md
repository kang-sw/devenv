# Plan: 260724-bug-windows-mcp-mid-session-disconnect — Phase 4: SQLite multi-process discipline

## Relevant Ticket Contract

- Extend bounded busy-retry to the currently-unretried point-read paths, cited
  by the ticket at `store.go:632,789,860,884` (ticket Phase 4, `:273-274`).
- Re-assert `journal_mode=WAL` on existing-DB opens; the ticket states WAL is
  currently set only on new-DB creation, cited at `store.go:254` (ticket
  Verified Findings `:55`, Phase 4 `:274`).
- Evaluate wiring the already-present but unused
  `internal/wsstate/orchestrator_lock.go` (`AcquireOrchestratorLock`, zero
  non-test callers) **or** `busy_timeout`/`wal_autocheckpoint` tuning to
  coordinate concurrent server processes over the shared `state.sqlite`
  (ticket Phase 4 `:274-276`).
- This phase is explicitly **hardening/robustness, not a confirmed disconnect
  cause** — the ticket's Decisions section (`:71-85`) rejects hypothesis B
  (SQLITE_BUSY → fatal exit) as the disconnect trigger: busy/locked is
  non-fatal in serve mode and returns a JSON-RPC error, not a process crash.
  Contention is an amplifier of symptoms (latency/error rate), not the
  disconnect mechanism itself (ticket `:56-60,77-79`). This survey's
  recommendations are calibrated to that posture: favor low-risk, cheap,
  reversible changes; do not accept invasive or cross-module coordination
  mechanisms on the strength of "it's already there and unused."
- Constraints (ticket `:87-95`): this dev box is Linux/WSL2 with
  `powershell.exe` interop for Windows-side process experiments — not needed
  for this phase, since SQLite/Go-level behavior (retry wrapping, PRAGMA
  re-assert) is fully cross-platform and testable on Linux. No launcher
  (`agents-plugin/bin/ws-mcp-launcher.py`) changes are in scope for this
  phase.
- Spec Impact (ticket `:97-108`): target `mcp-tools.md`'s
  `{#260525-runtime-metadata-migration-gate}` anchor, which already documents
  "SQLite state-store configure, migration, and short write paths use bounded
  retry" — this phase's item 1 changes that claim's scope (adds point reads)
  and must update the anchor text to stay accurate.

This document is a **survey and implementation plan only**. No source files
were modified while producing it.

## Out of Scope

- Phases 1-3 (panic recovery, launcher abnormal-exit breadcrumb, Windows
  process-lifecycle hardening) — all three are `### Result`-closed per the
  ticket (`:133-269`); this phase does not revisit them.
- Any change to `internal/wsstate/orchestrator_lock.go`'s own logic
  (`AcquireOrchestratorLock`, `createOrchestratorLock`, staleness recovery) —
  this survey only evaluates whether to *wire* the existing function into a
  new call site, not whether to modify its internals. See Design Evaluation
  below for why this survey recommends against wiring it at all.
- Any change to the async mercenary worker or `exec.*` subprocess lifecycle
  (`internal/wsagent`, `internal/execjob`) — those already do their own
  per-operation `wsstore.NewManager(...).Open(root)` (confirmed at
  `agent.go:378`, `execjob.go:558,620,676,740`); this phase changes what
  happens *inside* `Store.Open`/point-read methods, not who calls them or how
  often.
- Changing `SetMaxOpenConns(1)` (`store.go:190,225`) or the process-local
  `writeLocks`/`writeMu` mutex model (`store.go:49-53,168-177`) — out of
  scope; this phase adds retry and PRAGMA re-assertion around the existing
  connection/locking model, not a redesign of it.
- Raising `busy_timeout` above 5000ms or adding `wal_autocheckpoint` tuning —
  evaluated below and **not recommended** absent field/load evidence of
  insufficiency; see Design Evaluation and Escalations.
- Any spec/doc anchor rename. The existing
  `{#260525-runtime-metadata-migration-gate}` anchor slug is reused verbatim;
  only its body text is extended.

## Codebase Findings

### A. Point-read retry — verified against current source

All four ticket-cited line numbers in `agents-plugin-tool/internal/wsstore/store.go`
are **confirmed accurate against current source** (unlike Phases 2/3, where
line numbers had already shifted by the time of survey — here they have not
moved since the ticket was written):

| Line | Method | Query shape | Error surfaces at |
|---|---|---|---|
| `store.go:632` | `AgentDefinition` | `s.db.QueryRowContext(...)` — single-row, `agent_defs` LEFT JOIN `agent_instances` | `row.Scan(...)` at `:636` |
| `store.go:789` | `ExecJob` | `s.db.QueryRowContext(...)` — single-row `exec_jobs` lookup by key | `row.Scan(...)` at `:793` |
| `store.go:860` | `Artifact` | `s.db.QueryRowContext(...).Scan(...)` — single-row `artifacts` lookup by id, combined on one line | directly at `:860`, checked via `errors.Is(err, sql.ErrNoRows)` at `:861` |
| `store.go:884` | `PruneExpired` | `s.db.QueryContext(...)` — multi-row `artifacts` expiry scan | `if err != nil` at `:893` |

**Beyond the ticket's list**, a full grep of `s.db.Query` call sites
(`grep -n "s\.db\.Query\|s\.db\.Exec" store.go`) turns up three more unretried
read sites the ticket does not mention:

- `store.go:954` (`PruneAgentInstances`) — multi-row `agent_instances` scan,
  same shape as `:884`.
- `store.go:1064` (`Count`) — single-row `SELECT COUNT(*) FROM <table>`,
  `QueryRowContext(...).Scan(&count)`.
- `store.go:1109` (`retryTombstones`) — multi-row `artifact_tombstones` scan.

All writes and the migration/configure path already route through
`withSQLiteRetry`/`withSQLiteResultRetry` via `s.execWrite` (`store.go:1179-1185`)
or direct `withSQLiteRetry` wrapping (`configure` at `:257`, `Migrate` at
`:270`, `UpsertAgentDefinition` at `:573`, `DeleteAgentDefinition` at `:657`).
No read path (`QueryRowContext`/`QueryContext`) anywhere in `store.go` is
currently wrapped. **Finding: the ticket's characterization is accurate but
incomplete — 7 unretried read call sites exist, not 4.** A thorough Phase 4
should cover all 7, not just the ticket's cited subset, since `Count` and the
two prune scans are exercised on the same contended `state.sqlite` file as
the cited four.

### `retry.go` — exact existing retry helper API

`internal/wsstore/retry.go:1-73`:

- `withSQLiteRetry(ctx context.Context, fn func() error) error` (`:22-49`) —
  takes a **closure with no return value other than error**, retries up to
  `sqliteRetryAttempts = 8` (`:15`) times with exponential backoff from
  `sqliteRetryBaseDelay = 10ms` to `sqliteRetryMaxDelay = 120ms` (`:16-17`),
  gated on `isSQLiteBusyOrLocked(err)` (`:61-73`, matches `SQLITE_BUSY` /
  `SQLITE_LOCKED` via `modernc.org/sqlite`'s typed `*sqlite.Error` code check
  or a string fallback for `"SQLITE_BUSY"`/`"SQLITE_LOCKED"`/`"DATABASE IS
  LOCKED"`). Respects `ctx.Done()` between attempts (`:37-38`).
- `withSQLiteResultRetry(ctx context.Context, fn func() (sql.Result, error))
  (sql.Result, error)` (`:51-59`) — thin wrapper for the `Exec`-shaped
  signature, used by `s.execWrite` (`store.go:1179-1185`).
- There is **no existing "read" variant that returns a typed value** (e.g. no
  `withSQLiteRetry[T]` generic or a `(*sql.Row, error)`-shaped helper) — every
  current caller of `withSQLiteRetry` discards the closure's return value
  entirely (it returns only `error`) and captures results into outer-scope
  variables via closures instead (see `Migrate`, `UpsertAgentDefinition`: the
  closure builds a `*sql.Tx`, executes multiple statements, and returns only
  the final error, with row data never extracted this way today because those
  are write paths).
- **Point reads need a *new* thin wrapper**, not a change to
  `withSQLiteRetry`'s signature, because point reads must return scanned
  Go values (not `sql.Result`), and `sql.Row.Scan` itself is where the
  deferred query error surfaces (see table above) — `QueryRowContext` never
  returns an error directly. The idiomatic minimal addition: a
  `withSQLiteRetry`-shaped helper that retries the **combined
  query-and-scan** operation as one closure, e.g.:
  ```go
  func (s *Store) AgentDefinition(ctx context.Context, agentKey string) (AgentDefinition, bool, error) {
      var def AgentDefinition
      var found bool
      err := withSQLiteRetry(ctx, func() error {
          row := s.db.QueryRowContext(ctx, `SELECT ...`, agentKey)
          // scan into locals; sql.ErrNoRows is NOT retriable — must be
          // classified as "not busy" by isSQLiteBusyOrLocked and returned
          // immediately, then translated to (false, nil) outside the retry.
          ...
      })
      ...
  }
  ```
  This requires each point-read method to move its scan-and-assign logic
  inside the retry closure (re-running the full query+scan on each retry
  attempt, not just the query) — cheap for single-row/short-scan reads, and
  it is exactly the same pattern `Migrate`/`UpsertAgentDefinition` already use
  for write+scan-inside-tx today, so it is idiomatic to the file, not a new
  concept.
- **Multi-row reads** (`PruneExpired`, `PruneAgentInstances`,
  `retryTombstones`, ticket `:884` + the two undiscovered sites) need the
  retry closure to fully drain `rows` into a local slice **inside** the
  closure (as these methods already do at `:898-911`, `:970-984`,
  `:1114-1125`) before returning, so a mid-scan busy/locked error triggers a
  full clean re-query rather than a partial-slice retry. This is a bigger
  diff per site than the single-row case but mechanically identical to the
  existing accumulate-then-process pattern already used in all three
  multi-row methods — no restructuring of the surrounding logic needed, only
  wrapping the existing `rows, err := s.db.QueryContext(...)` +
  drain-loop+`rows.Err()` block in `withSQLiteRetry`.
- **Read-vs-write distinction needed**: none functionally — `withSQLiteRetry`
  is read/write-agnostic (it only inspects the error). The **only** thing to
  decide per call site is whether to hold `s.writeMu` during the retry. Reads
  currently take **no** mutex at all (`AgentDefinition`, `ExecJob`, `Artifact`,
  `PruneExpired`, `PruneAgentInstances`, `Count`, `retryTombstones` all call
  `s.db.Query*` with zero locking), whereas every write path holds
  `s.writeMu.Lock()` first (`store.go:571,655` explicit;
  `execWrite` at `:1180`). Reads should **not** newly acquire `s.writeMu` —
  doing so would serialize reads behind writes in-process for no
  contention-safety benefit (SQLite/WAL already lets readers proceed
  concurrently with a single WAL writer; `SetMaxOpenConns(1)` already
  serializes this process's own DB access at the `database/sql` pool level
  regardless). Adding retry to reads is purely about **surviving an
  *other-process*'s writer holding the lock**, which is exactly what
  `TestIndependentHandleContentionRetriesShortWrite` (`store_test.go:754-814`)
  already exercises for a write — the same independent-second-handle pattern
  applies unchanged to a read.

### `busy_timeout=5000` — does it already cover most contention?

`store.go:250` sets `PRAGMA busy_timeout=5000` inside `configure()`
(`:246-265`), executed once per `Store` (i.e., once per `Manager.Open`/
`OpenWorktreeKey` call, which — per the per-operation open/close model
confirmed at `agent.go:378` and `execjob.go:558,620,676,740` — is once per MCP
tool invocation touching the store). `busy_timeout` makes SQLite's own C-level
busy handler retry internally (blocking up to 5000ms) **before** ever
returning `SQLITE_BUSY` to the Go driver. The app-level `withSQLiteRetry`'s 8
attempts only fire *after* that 5000ms internal wait is exhausted for a given
attempt. So the two layers compose additively, not redundantly: **most
transient contention is already absorbed silently inside a single 5000ms
`busy_timeout` window**, and the 8-attempt app retry is a second-order
safety net for the rare case where a writer holds the lock for longer than
5000ms continuously (unlikely given the codebase's short-transaction
discipline — `Migrate`, `UpsertAgentDefinition`, `execWrite` all wrap brief,
single-statement-or-short-tx operations, never a long-running or
subprocess-spanning transaction, matching the existing mental-model claim at
`mcp-runtime.md:74`). **Conclusion: yes, app-level read retry is
belt-and-suspenders relative to `busy_timeout=5000`, not the primary
contention-safety mechanism** — but it closes a real, if narrow, gap
(concurrent processes' overlapping 5000ms windows, or a slow disk/WSL9p
filesystem stretching a single write past 5000ms) and costs nothing extra
under the common case (retry only fires when `isSQLiteBusyOrLocked` is true).

### B. WAL re-assert — verified against current source

- `store.go:253-255`:
  ```go
  if setJournalMode {
      statements = append([]string{`PRAGMA journal_mode=WAL`}, statements...)
  }
  ```
  `setJournalMode` is the `newDB` bool computed at `Manager.Open` (`:185`,
  `!fileExists(path)`) and `OpenWorktreeKey` (`:220`), passed into
  `store.configure(ctx, newDB)` (`:194`, `:229`). **Confirmed exactly as the
  ticket states**: `journal_mode=WAL` is issued only the first time a given
  `state.sqlite` file is created, never on a later open of a pre-existing
  file. `busy_timeout=5000` and `foreign_keys=ON` (`:250-251`), by contrast,
  run unconditionally on every open (they are outside the `if` block).
- **Why this matters**: any `state.sqlite` created before the WAL PRAGMA was
  added to this codebase (or, hypothetically, any DB whose journal mode was
  externally reset, e.g. copied/restored from a rollback-journal-mode backup)
  permanently stays in SQLite's default rollback-journal mode on every
  subsequent open, because nothing ever re-issues the PRAGMA. Rollback-journal
  mode has materially worse concurrent reader/writer behavior than WAL
  (readers block writers and vice versa, vs. WAL's concurrent-reader/
  single-writer model) — directly relevant to "multi-process discipline."
- **Is `PRAGMA journal_mode=WAL` a cheap no-op when already WAL?** Yes, per
  SQLite's own documented behavior: if the database is already in WAL mode,
  the pragma is a fast no-op (single header/flag check, no file rewrite, no
  need for other connections to have zero active statements — that
  requirement only applies when *switching into* WAL from another mode).
  Given `SetMaxOpenConns(1)` (see below), there is only ever one live
  connection from this process at `configure()` time, so even a genuine
  mode-switch (non-WAL → WAL, the one-time legacy-DB case) safely satisfies
  SQLite's "no other connections with the DB open" precondition **from this
  process's perspective** — a concurrent WAL switch attempted by a second
  server process against the same file is the actual multi-process risk case
  and is exactly why this stays wrapped in `withSQLiteRetry` (a concurrent
  switch attempt can itself return `SQLITE_BUSY`).
- **Per-connection vs per-DB, and `SetMaxOpenConns(1)`'s effect**:
  `journal_mode=WAL` is a **database-file-level** property (persisted in the
  file header across connections and process restarts) — once successfully
  set, it stays WAL until something explicitly changes it, unlike
  `busy_timeout`/`foreign_keys`, which are **per-connection** settings that
  reset to SQLite defaults on every new physical connection and must be
  re-applied. `store.go:190,225` (`db.SetMaxOpenConns(1)`) means
  `database/sql` never opens more than one physical connection per `*Store`
  at a time, so the current one-shot `configure()` call (executed once,
  immediately after `sql.Open`, under `openMu` at `:192-197`/`:227-232`)
  correctly re-applies `busy_timeout`/`foreign_keys` for that connection's
  entire lifetime — **provided the pool never transparently recreates the
  underlying connection**. `database/sql` *can* silently open a replacement
  connection if the existing one is dropped (e.g. `driver.ErrBadConn`, or, in
  principle, idle-connection recycling — though `SetConnMaxIdleTime`/
  `SetConnMaxLifetime` are never called in this codebase, so no time-based
  recycling is configured); a transparently-recreated connection would **not**
  automatically get `busy_timeout`/`foreign_keys` re-applied, since
  `configure()` only runs once at `Open()` time, not on every new physical
  connection. This is a **pre-existing latent gap for `busy_timeout`, not
  something this phase's WAL work introduces** — noted for completeness but
  out of scope to fix here (would require `sql.Open` + a
  `ConnectHook`/`driver.Connector`, a larger change than "re-assert on
  existing-DB opens").
- **Does `modernc.org/sqlite` honor a DSN parameter, or need explicit
  `db.Exec("PRAGMA ...")`?** Both are supported, and this matters for the
  latent-gap above. Confirmed by reading the vendored driver source
  (`$(go env GOMODCACHE)/modernc.org/sqlite@v1.30.2/sqlite.go:818-912`,
  version pinned at `go.mod:8`, `modernc.org/sqlite v1.30.2`): `newConn`
  parses a DSN's query string and `applyQueryParams` (`:881-912`) runs a
  `PRAGMA ...` for **every value of a repeated `_pragma=` query parameter**
  (`:887-893`, e.g. `file:state.sqlite?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)`)
  — and this runs on **every new physical connection** the driver creates,
  not once per `*sql.DB`. This is a strictly more robust mechanism than the
  current one-shot `db.Exec("PRAGMA ...")` in `configure()` for
  connection-level settings (`busy_timeout`, `foreign_keys`), because it
  self-heals across any future connection churn without relying on
  `configure()` having been called again. **This phase's WAL item does not
  require switching to the DSN mechanism** (the current `Exec`-based approach
  is fine for `journal_mode`, since it is file-level and persists regardless
  of which connection sets it) — but it is worth recording as an available,
  lower-risk alternative for the `busy_timeout`/`foreign_keys` per-connection
  gap noted above, should a future phase need to harden against connection
  churn specifically.

**Design for the re-assert**: remove the `if setJournalMode` gate so
`PRAGMA journal_mode=WAL` runs unconditionally in `configure()`'s statement
list, on every `Open`/`OpenWorktreeKey` call, for both new and pre-existing
DBs — cheap (no-op) on the already-common case, and closes the one-time-only
gap on the pre-existing-DB case. The `newDB` parameter to `configure()` then
becomes unused and should be removed (or intentionally kept only if a
follow-on tuning decision, e.g. `wal_autocheckpoint`, wants a
new-vs-existing distinction; this survey found no such need — see below).

### C. Item 3 — evaluate wiring `orchestrator_lock.go`, or pure tuning

`internal/wsstate/orchestrator_lock.go:1-103` read in full:

- `AcquireOrchestratorLock(repoPath, version string) (OrchestratorLockResult, error)`
  (`:28-69`) creates/reads a **single** file, `orchestrator.lock`, at
  `layout.WorktreeLocksDir/orchestrator.lock` (`:11,33`) — a **different
  directory/file** than `state.sqlite` (`WorktreeDir/state.sqlite`,
  `store.go:181,219`). It is a **whole-process, hold-once, single-owner**
  mutual-exclusion lock: `createOrchestratorLock` uses `O_EXCL` create
  (`:77`) to atomically win ownership once, records the winning PID
  (`os.Getpid()`, `:37`), and staleness recovery only triggers when the
  **entire recorded PID is dead** (`processAlive(existing.PID)`, `:56`) — it
  has no concept of shared/read access, no lease renewal, no per-operation
  acquire/release, and no timeout-based fairness. Confirmed **zero
  non-test callers**: `grep -rn "AcquireOrchestratorLock"` across the whole
  `agents-plugin-tool` tree matches only its own definition
  (`orchestrator_lock.go:28`) and three call sites in
  `internal/wsstate/paths_test.go` (`:87,101,136,227,231`).
- **Semantic mismatch with the state.sqlite contention problem**: the
  contention this ticket phase is about is *many short-lived, per-tool-call
  `Store.Open`/`Close` cycles* from potentially multiple concurrent
  `ws-mcp serve` processes (confirmed frequency: every `wsagent`/`execjob`
  call opens and presumably closes its own `Store`, `agent.go:378`,
  `execjob.go:558,620,676,740`) — i.e., a **high-frequency, fine-grained,
  reader/writer** access pattern that SQLite's own WAL + `busy_timeout` +
  this codebase's existing retry already target. `AcquireOrchestratorLock`
  is shaped for the opposite problem: a **coarse, singleton-instance,
  whole-process-lifetime** lock (its own name — "orchestrator" — and its
  PID-recorded-once/staleness-on-death design match a "is there already a
  running orchestrator for this worktree" check, not a per-query
  coordination primitive). Wiring it into the `Store.Open`/point-read path
  would require either (a) acquiring/releasing it on every single store
  operation — defeating its own `O_EXCL`-once design and adding a second,
  redundant filesystem-lock layer on top of SQLite's own locking for every
  read and write, a real latency/complexity cost for no coordination benefit
  SQLite doesn't already provide — or (b) acquiring it once per `ws-mcp
  serve` process at startup and treating a second process as
  "must wait/fail" — which would silently break the ticket's own confirmed,
  currently-supported reality that **multiple concurrent server processes
  legitimately share one `state.sqlite`** (this is exactly the scenario the
  ticket's Verified Findings describes as already-happening and only
  degrading gracefully, not something to be prevented outright).
- **Recommendation: DEFER wiring `AcquireOrchestratorLock`.** One-line
  rationale: it is a coarse whole-process singleton lock guarding a
  different file, semantically mismatched to the fine-grained
  per-operation `state.sqlite` reader/writer contention this phase targets,
  and repurposing it would either add redundant per-query filesystem
  locking or wrongly forbid the multiple-concurrent-server-processes
  scenario the ticket already treats as supported. This is resolvable from
  source alone (the file/lock-shape mismatch is structural, not a judgment
  call needing stakeholder input) — no binding-decision escalation is
  raised for this sub-item.
- **Pure-tuning alternative, evaluated**:
  - `busy_timeout` (currently 5000ms, `store.go:250`): no source or test
    evidence in this codebase suggests 5000ms is insufficient — see the
    `busy_timeout` analysis above (already-short transactions, retry as
    belt-and-suspenders). Raising it would only mask, not fix, a
    longer-than-5s lock hold, and would make every busy contended
    call block proportionally longer before even entering the app retry
    loop. **Not recommended without field/load evidence**; see Escalations.
  - `wal_autocheckpoint` (SQLite default: 1000 pages, unset anywhere in this
    codebase — confirmed via `grep -rn "wal_autocheckpoint"` returning zero
    hits): this bounds how large the `-wal` file grows before an automatic
    checkpoint folds it back into the main DB file. Given every write path
    in this codebase already uses short, single-statement-or-brief-tx writes
    (no long-held write transactions found anywhere in `store.go`), organic
    WAL growth between checkpoints should stay small under normal operation;
    there is no source evidence of unbounded WAL growth being a problem here.
    **Not recommended to change without field evidence of WAL bloat**
    (e.g., a downstream report of a large `-wal` file) — tuning this
    blind is speculative hardening disproportionate to a phase explicitly
    scoped as "not a confirmed cause."
  - **Conclusion for item 3: neither wiring the lock nor tuning
    `busy_timeout`/`wal_autocheckpoint` is justified by current source
    evidence.** The two items already in scope (point-read retry, WAL
    re-assert) are sufficient for this phase; item 3 resolves to "evaluated,
    no action taken," which is itself the deliverable the ticket asks for
    ("evaluate wiring ... or tuning").

### D. Cross-platform / test shape

- `store_test.go` establishes the exact idiom this phase's tests should
  reuse:
  - **Temp DB / Manager.Open idiom**: `initRepo(t)` + `cache :=
    filepath.Join(t.TempDir(), "cache")` + `NewManager(Options{CacheHome:
    cache}).Open(root)` appears in nearly every test (e.g.
    `TestOpenCloseReopenCreatesWorktreeDatabase:23-49`,
    `TestIndependentHandleContentionRetriesShortWrite:754-762`).
    `TestOpenCloseReopenCreatesWorktreeDatabase` already closes and reopens
    the same path (`:38-48`) — a direct precedent for a re-open-and-assert
    WAL test.
  - **Concurrent-contention idiom** (`TestIndependentHandleContentionRetriesShortWrite`,
    `:754-814`): opens a **second, independent** `sql.Open("sqlite",
    store.Path())` handle (`:767`), sets `busy_timeout=1` on both the
    `Store`'s db and the second handle (`:763,772`) to shrink the test's
    wait time, issues `BEGIN IMMEDIATE` on the second handle to hold a real
    write lock (`:775`), then calls a `Store` write method in a goroutine and
    asserts (via the `sqliteRetryBusyHook` test seam, `:780-787`) that a
    busy/locked error was actually observed before the second handle commits
    and releases the lock (`:789-810`). **This is directly reusable for a
    point-read regression test**: swap the goroutine's write call
    (`store.UpsertAgentDefinition`) for a point-read call (e.g.
    `store.AgentDefinition(ctx, "contended")`) once that method is wrapped in
    retry, and assert it succeeds only after the busy hook fires and the
    holder commits.
  - **Retry-unit-test idiom** (`TestSQLiteRetryRetriesBusyAndLockedErrors`,
    `:872-903`): tests `withSQLiteRetry` directly with a synthetic error
    string containing `"synthetic SQLITE_BUSY"`/`"SQLITE_LOCKED"` and an
    attempt counter — no real DB needed. Useful as a template if a new
    typed-value retry helper needs its own unit test independent of a real
    SQLite file.
- **New tests this phase should add** (design, not yet written):
  1. **Point-read contention regression** — extend
     `TestIndependentHandleContentionRetriesShortWrite`'s pattern (or add a
     sibling test) asserting a point read (`AgentDefinition`, or one
     representative single-row and one representative multi-row method)
     succeeds and observes the busy hook under the same second-handle-holds-
     `BEGIN IMMEDIATE` setup, proving the retry path is actually exercised
     for reads, not just writes.
  2. **WAL re-assert on pre-existing non-WAL DB** — a new test that (a)
     creates a `state.sqlite` file at the expected path via a **raw**
     `sql.Open("sqlite", path)` + minimal schema/PRAGMA sequence that leaves
     it in SQLite's default (non-WAL) journal mode, closes that raw handle,
     then (b) calls `Manager.Open(root)`/`OpenWorktreeKey` against the same
     path and asserts `PRAGMA journal_mode` reports `wal` afterward (query it
     via `store.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode)`
     directly, mirroring how `store_test.go` already issues ad hoc PRAGMAs at
     `:763,772`). This is the regression test that would have caught the
     current `if setJournalMode` gate's gap.
  3. **WAL re-assert is idempotent on an already-WAL DB** — extend
     `TestOpenCloseReopenCreatesWorktreeDatabase` (or a new test) to assert
     `journal_mode` is `wal` both immediately after first creation and after
     the existing close+reopen (`:38-48`), proving the unconditional PRAGMA
     doesn't regress the already-passing common case.
- All of the above are pure Go/SQLite tests with no OS-specific behavior —
  fully exercisable in this Linux/WSL2 dev environment and on Windows CI
  identically, consistent with the ticket's Constraints framing that "Phase 1
  is fully cross-platform" (and, by the same reasoning, so is Phase 4 — it
  touches no launcher or OS-process code at all).

### E. Spec/doc impact

- `ai-docs/spec/mcp-tools.md:1632-1646`
  (`## Runtime Metadata Migration Gate {#260525-runtime-metadata-migration-gate}`)
  already contains the load-bearing sentence: "SQLite state-store configure,
  migration, and short write paths use bounded retry for `SQLITE_BUSY` and
  `SQLITE_LOCKED` conditions while retaining process-local write
  serialization" (`:1643-1645`). This sentence becomes **stale** once item 1
  ships (point reads gain retry too) — it must be extended to something like
  "...configure, migration, and read and write paths use bounded retry..."
  rather than left implying reads are unretried. Same anchor slug, body text
  edit only.
- `ai-docs/spec/plugin-runtime.md:198-212` — the two Phase 1-3 anchors
  (`{#260724-launcher-abnormal-exit-breadcrumb}`,
  `{#260724-windows-process-lifecycle-hardening}`) already mention
  `state.sqlite` locks in the orphan-prevention context (`:202`,
  "cannot leave an orphaned server holding a stale `state.sqlite` lock").
  Phase 4 does not need a new anchor here — the WAL/retry hardening is an
  internal robustness detail, not a new caller-visible contract, matching
  the ticket's own Spec Impact framing (`:106-108`, "no — a robustness fix
  reflected into the specs at implementation/closeout"). No new anchor
  required in this file.
- `ai-docs/mental-model/mcp-runtime.md:74` has the exact bullet this task
  description points at:
  > "SQLite configure, migration, and short write paths use bounded
  > `SQLITE_BUSY`/`SQLITE_LOCKED` retry while retaining process-local write
  > serialization. ... {#260525-runtime-metadata-migration-gate}"

  This needs the same "reads too" extension as the spec anchor above, kept
  consistent in wording since both cite the same anchor slug. No other
  mental-model bullet in this file references SQLite retry/WAL specifically.

## Implementation Plan

1. **Add a typed point-read retry pattern in `store.go`** (no change to
   `retry.go`'s existing helper signatures — `withSQLiteRetry`/
   `withSQLiteResultRetry` stay as-is and are reused directly):
   - Wrap each of the 7 identified read call sites
     (`AgentDefinition:632`, `ExecJob:789`, `Artifact:860`,
     `PruneExpired:884`, `PruneAgentInstances:954`, `Count:1064`,
     `retryTombstones:1109`) so the query **and** its scan/drain happen
     inside a `withSQLiteRetry` closure, following the existing
     accumulate-then-process shape those multi-row methods already use.
   - For single-row methods (`AgentDefinition`, `ExecJob`, `Artifact`,
     `Count`): move the `QueryRowContext(...).Scan(...)` call inside the
     closure; treat `sql.ErrNoRows` as an immediate non-retriable return
     (it is not `SQLITE_BUSY`/`SQLITE_LOCKED`, so `isSQLiteBusyOrLocked`
     already classifies it correctly as non-retriable — confirm this with
     a targeted test rather than assuming, since `errors.Is`/`errors.As`
     wrapping across the closure boundary must not lose the sentinel).
   - For multi-row methods (`PruneExpired`, `PruneAgentInstances`,
     `retryTombstones`): move the `QueryContext` + full drain loop +
     `rows.Err()` check inside the closure, writing results into a
     closure-captured outer slice, mirroring the existing pattern at
     `:898-911`/`:970-984`/`:1114-1125`.
   - Do **not** acquire `s.writeMu` in any read path — see the read-vs-write
     analysis above; only the existing write paths keep the mutex.
   - No change to `retry.go` itself; no change to `isSQLiteBusyOrLocked`.
2. **Re-assert WAL on every open** (`store.go:246-265`, `configure`):
   - Remove the `if setJournalMode { ... }` gate; always prepend `PRAGMA
     journal_mode=WAL` to the statement list.
   - Remove the now-unused `setJournalMode`/`newDB` parameter from
     `configure` and its two call sites (`Open:194`, `OpenWorktreeKey:229`),
     unless a later reviewer wants to keep the `newDB` computation for
     logging/metrics purposes (not required by this phase).
   - Leave `busy_timeout=5000` and `foreign_keys=ON` unconditional as they
     already are — no change needed there.
   - Do not switch to the DSN `_pragma=` mechanism for this phase (see
     Codebase Findings B) — it is a valid alternative for the separately
     noted `busy_timeout`/`foreign_keys` connection-churn gap, but is a
     larger, differently-scoped change than "re-assert WAL on existing-DB
     opens" and is not required to satisfy this phase's ticket text.
3. **Item 3: no code change.** Record the evaluation (wiring
   `AcquireOrchestratorLock`: deferred; `busy_timeout`/`wal_autocheckpoint`
   tuning: deferred) in the ticket's Phase 4 `### Result` when this phase
   closes, per the Design Evaluation above. No source edit for this item.
4. **Update spec/mental-model text** (see Spec/Doc Impact) alongside the
   code change, in the same change/commit that ships items 1-2, per this
   repo's "update drifted docs on contact" rule.

## Verification Plan

- `go build ./...` clean (no signature changes leak outside `wsstore`; all 7
  read methods keep their existing public signatures).
- `go test ./internal/wsstore/...` — full existing suite green, plus the
  three new/extended tests from Codebase Findings D:
  1. Point-read contention regression (busy hook fires on a read, then
     succeeds after the holder commits).
  2. Pre-existing non-WAL DB gets re-asserted to `wal` on `Manager.Open`.
  3. Already-WAL DB stays `wal` across close+reopen (idempotence /
     no-regression check on the common case).
- `go test ./...` (full repo) clean, to confirm no downstream package
  (`wsagent`, `execjob`, `mcp`) that calls the changed `Store` methods
  breaks on behavior (retry adds latency under contention only, not new
  error types or changed signatures).
- Manually confirm via `sqlite3 <path> 'PRAGMA journal_mode;'` (or the
  in-test PRAGMA query) against a DB created by the pre-patch binary, then
  reopened by the patched binary, actually flips to `wal` — a live
  before/after check complementing the automated test.
- No Windows-specific verification needed (see Codebase Findings D) — this
  phase is pure Go/SQLite logic exercised identically on Linux CI, matching
  Phase 1's precedent of being "fully cross-platform."

## Escalations

- No `[escalate-to-binding-decision]` is raised for **whether to wire
  `AcquireOrchestratorLock`** — the file/lock-shape mismatch (different
  file, coarse whole-process singleton lock vs. fine-grained per-operation
  reader/writer contention) is structural and resolvable from source alone.
  Recommended and default: **defer**, do not wire it.
- One item genuinely **cannot** be resolved from static source review alone:
  **whether `busy_timeout=5000` (or the unset `wal_autocheckpoint`) actually
  needs raising/tuning.** This requires field or load-test evidence of
  real-world lock-hold durations and WAL growth under the downstream's
  actual concurrency pattern (multiple background agents/subagents each
  holding a wsflow connection, per the ticket's Background), which this
  survey cannot observe from the repository alone. **Recommended default:
  do not tune either value in this phase; ship items 1-2 (retry + WAL
  re-assert) only, and revisit `busy_timeout`/`wal_autocheckpoint` only if a
  future downstream report or load test shows contention durations
  exceeding 5s or unbounded `-wal` file growth.** This mirrors Phase 2's
  precedent of dropping speculative hardening (the stderr-redirect capture)
  absent field evidence of a remaining gap.
