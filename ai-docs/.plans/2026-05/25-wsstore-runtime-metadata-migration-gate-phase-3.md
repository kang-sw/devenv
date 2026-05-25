# Survey: 25-wsstore-runtime-metadata-migration-gate-phase-3

## Reusable Components
- `agents-plugin-tool/internal/wsstore/store.go#L120-L158` — `Manager.Open`: opens the worktree `state.sqlite`, configures WAL/busy timeout, runs migrations, and returns the existing layout; relevant for exec metadata persistence under the existing gate.
- `agents-plugin-tool/internal/wsstore/retry.go#L14-L59` — `withSQLiteRetry` / `withSQLiteResultRetry`: bounded retry helper for `SQLITE_BUSY`/`SQLITE_LOCKED` writes; relevant to short exec lifecycle/byte-count updates.
- `agents-plugin-tool/internal/wsstore/store.go#L421-L483` — `Artifact` APIs: existing path/byte-count/pinned/expiry metadata rows for file-backed payloads; relevant to stdout/stderr/combined stream indexes and prune eligibility.
- `agents-plugin-tool/internal/wsstore/store.go#L485-L544` — `PruneExpired`: skips active/running/cancel-requested/leased states and tombstones cleanup failures; relevant to exec payload cleanup semantics.
- `agents-plugin-tool/internal/wsstore/metadata_inventory.go#L68-L87` — `ClassifyFileBackedPayload`: classifies missing file-backed payloads as recoverable consistency states; relevant to missing exec stream files.
- `agents-plugin-tool/internal/execjob/execjob.go#L38-L99` — `LaunchOptions`, `Record`, and response types: existing public/internal exec metadata and JSON response shape; relevant to preserving `exec.*` output fields.
- `agents-plugin-tool/internal/execjob/execjob.go#L429-L460` — `jobDir` / `streamPaths`: existing deterministic job-owned file layout and stream validation; relevant to keeping payload bytes file-backed.
- `agents-plugin-tool/internal/execjob/execjob.go#L250-L275` — raw reader functions: existing tail/read/grep path through `textreader`; relevant because raw scanning should not hold SQLite transactions.

## Existing Patterns
- SQLite-authoritative metadata with read-only legacy import: see `agents-plugin-tool/internal/wsagent/agent.go#L513-L545` — lookup checks SQLite first, imports legacy `agent.json` once when valid, and returns bounded recovery for corrupt legacy metadata.
- Metadata write adapter around wsstore: see `agents-plugin-tool/internal/wsagent/agent.go#L438-L562` — manager opens `wsstore`, maps runtime struct fields to store definitions, and writes through store APIs instead of JSON authority.
- Contract-field round-trip tests for migrated metadata: see `agents-plugin-tool/internal/wsagent/agent_test.go#L2290-L2308` and `agents-plugin-tool/internal/wsagent/agent_test.go#L2429-L2455` — verifies no new `agent.json` write authority, legacy import, and manager restart survival.
- Source-local inventory coverage without reverse imports: see `agents-plugin-tool/internal/wsstore/store_test.go#L383-L433` — parses consumer struct tags from source files to keep metadata inventory exhaustive without importing runtime consumers.
- MCP exec dispatch is a thin adapter: see `agents-plugin-tool/internal/mcp/server.go#L454-L526` — all `exec.*` tools resolve root, forward arguments to `execjob`, and return JSON text responses.
- No-agent/wsflow hides the whole exec family: see `agents-plugin-tool/internal/mcp/server.go#L2188-L2238` and `agents-plugin-tool/internal/mcp/server.go#L2886-L2898` — tool registry includes full `exec.*`, while no-agent mode rejects names with `exec.` prefix.

## Relevant Interfaces
- `agents-plugin-tool/internal/wsstore/store.go#L758-L768` — existing `exec_jobs` schema: currently skeletal columns for key/status/lease and stream paths; likely insufficient for the Phase 3 field inventory as written.
- `agents-plugin-tool/internal/wsstore/metadata_inventory.go#L135-L138` — exec inventory classification: currently marks `Record` metadata as SQLite-authoritative candidates and stream bodies as file-backed payloads.
- `agents-plugin-tool/internal/execjob/execjob.go#L106-L207` — `Launch`: creates stream files, writes state before/after process start, tracks active worker, and returns foreground-complete or running response.
- `agents-plugin-tool/internal/execjob/execjob.go#L278-L348` — `finalize`, `refreshSizes`, `reconcile`: updates terminal/lost-worker lifecycle and byte counts from stream files.
- `agents-plugin-tool/internal/execjob/execjob.go#L351-L368` — `responseFor`: preserves fixed inline budget behavior and reads stdout/stderr files only when including terminal output.
- `agents-plugin-tool/internal/execjob/execjob.go#L375-L407` — working-directory and shell resolution: keeps omitted/relative working directories inside the root and handles Windows shell defaults.
- `agents-plugin-tool/internal/mcp/server_test.go#L1900-L1971` — MCP exec flow/no-agent test: covers tools/list, spawn/shell, status/result/raw readers, and hidden no-agent behavior.
- `agents-plugin-tool/internal/execjob/execjob_test.go#L36-L150` — exec package tests: cover launch/result/raw readers, working-dir escape rejection, abort, large-output guidance, and lost-worker reconciliation.

## Constraints
- `ai-docs/spec/mcp-tools.md#L363-L397` fixes the public `exec.*` behavior: bounded foreground launch, fixed 4096-byte inline budget, durable status/result/abort, lost-worker reconciliation, and literal-by-default raw grep.
- `ai-docs/spec/mcp-tools.md#L398-L410` and `ai-docs/mental-model/mcp-runtime.md#L49-L51` require metadata/payload separation and short SQLite transactions; stream bytes stay in files.
- `ai-docs/mental-model/mcp-runtime.md#L74-L75` requires the exec family to move together, preserve root/working-dir handling, reconcile lost workers, keep wsflow hidden behavior, and avoid reverse-import cycles in `wsstore` tests.
- `agents-plugin-tool/internal/wsstore/store.go#L146-L157` plus `agents-plugin-tool/internal/wsstore/store.go#L209-L252` use one open DB connection and process-local write locks; exec writes should reuse this rather than long transactions around subprocess execution.
- `agents-plugin-tool/internal/execjob/process_windows.go#L1-L35` and `agents-plugin-tool/internal/execjob/execjob.go#L390-L407` contain Windows-specific cancellation/shell behavior; tests that add coverage should avoid Unix-only assumptions unless explicitly skipped.

## Risk Signals
- `agents-plugin-tool/internal/execjob/execjob.go#L174-L193` and `agents-plugin-tool/internal/execjob/execjob.go#L278-L348` — Possible contract risk: lifecycle and size updates still write `state.json`; Phase 3 forbids parallel JSON write authority after migration, so every launch/finalize/abort/reconcile path needs inspection.
- `agents-plugin-tool/internal/wsstore/store.go#L758-L768` — Possible contract risk: existing `exec_jobs` table lacks command/argv/shell/root/working-dir/env/stdin/pid/timestamps/exit/error/byte-count/retention/cleanup fields named in the brief.
- `agents-plugin-tool/internal/execjob/execjob.go#L351-L368` — Possible recoverability risk: missing stdout/stderr files are ignored when inline output is included, so callers can see empty output without a recoverable consistency explanation.
- `agents-plugin-tool/internal/execjob/execjob.go#L429-L437` — Possible reuse/config risk: `jobDir` opens `wsstate.NewManager(wsstate.Options{})` without propagating a manager `CacheHome`; current tests set `WS_CACHE_HOME`, but direct options-based wsstore use may need consistent cache-root handling.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L26-L75` — Possible scope risk: there is no public `exec` CLI mirror today, despite the brief naming `cmd/ws-mcp/` if touched; implementation should avoid inventing a CLI surface unless lead/planner confirms it.
- `ai-docs/spec/mcp-tools.md#L398-L402` and `ai-docs/mental-model/mcp-runtime.md#L28-L28` — Possible doc drift risk: current docs still say exec metadata remains later/deferred, which is true before Phase 3 closeout but can mislead implementation if read as current target behavior.

## Opinion
- Survey found enough code-local precedent to proceed without research: Phase 2's named-agent migration pattern is directly reusable, but exec has more lifecycle write sites and stream consistency cases than the existing skeletal `exec_jobs` table covers.
- The biggest uncertainty is not public `exec.*` behavior; it is the internal compatibility boundary for corrupt/unimportable legacy `state.json` records and how that bounded recovery should be represented in existing JSON response shapes without public schema churn.
