# Plan: 260524-feat-wsstore-runtime-metadata-migration-gate Phase 1

## Objective

Define the SQLite contention and runtime metadata migration gate without moving
named-agent or exec runtime metadata into SQLite yet. Phase 1 should make the
boundary explicit in code and tests, while Phase 2 remains the first real
runtime migration.

## Current Baseline

- `agents-plugin-tool/internal/wsstore` already owns the SQLite state-store
  foundation, including actor setup/recovery, prune bookkeeping, tombstones,
  `busy_timeout`, and process-local serialization.
- `agents-plugin-tool/internal/wsagent` still owns the file-backed named-agent
  registry, current-call state, prompt snapshots, streams, events, and outputs.
- `agents-plugin-tool/internal/execjob` still owns the file-backed exec job
  state record and stdout/stderr/combined payload files.
- Current tests prove same-process runtime behavior, but they do not yet prove
  the Phase 1 gate requirements for cross-process contention handling or a
  field-by-field migration inventory.

## Smallest Implementation Shape

1. Add a small `wsstore`-local metadata inventory that classifies the current
   runtime fields into:
   - SQLite metadata / control-plane rows
   - file-backed payloads or path indexes
   - temporary compatibility-only data
   This inventory must explicitly cover `agent.json`, `current/state.json`, and
   exec job `state.json`.
2. Add a bounded busy/locked retry helper in `internal/wsstore` and use it for
   short database open/configure/migrate/write operations only.
   Keep the existing process-local mutexes. Do not add a single-writer daemon
   and do not move append-heavy payloads into SQLite.
3. Add tests that prove:
   - independent handles or independent processes can contend on one worktree
     state database without persistent `SQLITE_BUSY` / `SQLITE_LOCKED`
   - append-heavy payloads remain file-backed
   - actor-bound named-agent identity can reuse a public agent name across
     actors without collision
   - rows pointing at missing file-backed payloads surface a recoverable /
     tombstone / reconciliation classification
   - `agent.json` compatibility, if retained, is read-only and not write
     authority
4. Keep the first real runtime metadata migration out of this phase.

## Field Inventory to Codify

### `agent.json`

- SQLite metadata: `schema_version`, `name`, `backend`, `harness`, `tier`,
  `model`, `effort`, `session_id`, `status`, `created_at`, `last_seen_at`,
  `last_call_at`, `prompt_refs`, `child_actor_id`, `child_actor_authority`,
  `capabilities`, `ephemeral`
- File-backed payload/path indexes: `system_prompt_path`, `last_output_path`
- Temporary compatibility only: any read-only importer or diagnostic snapshot
  used to bridge pre-migration files

### `current/state.json`

- SQLite metadata: `schema_version`, `agent_name`, `call_seq`, `execution_id`,
  `status`, `pid`, `started_at`, `updated_at`, `finished_at`, `exit_code`,
  `session_id`, `error`, `cleanup_needed`, `cancel_pid`
- File-backed payload/path indexes: `prompt_path`, `stdout_path`, `stderr_path`

### Exec job `state.json`

- SQLite metadata: `schema_version`, `exec_key`, `status`, `root`,
  `working_dir`, `pid`, `started_at`, `updated_at`, `completed_at`,
  `exit_code`, `error`, `cancel_requested`, `stdout_bytes`, `stderr_bytes`,
  `combined_bytes`
- File-backed payloads remain on disk: `stdout`, `stderr`, `combined`

## Steps

1. Codify the field inventory close to `internal/wsstore` so future migration
   code can reuse one authoritative classification.
2. Add the bounded retry/backoff helper around SQLite open/configure/migrate
   and short write paths, keeping the current process-local serialization.
3. Add focused tests in `internal/wsstore` first, then only the minimal
   `wsagent` / `execjob` / `mcp` integration coverage needed to prove the gate.
4. Add native Windows coverage or a documented Windows-only hook for the
   contention and file-locking cases; do not treat Unix-only proof as complete.
5. Keep Phase 1 out of the actual migration path; Phase 2 owns the first
   runtime consumer move.

## Validation

- `cd agents-plugin-tool && go test ./internal/wsstore`
- `cd agents-plugin-tool && go test ./internal/wsagent ./internal/execjob ./internal/mcp`
- Any new cross-process contention test must fail if a future regression
  reintroduces persistent lock errors or starts storing large payloads in
  SQLite.

## References

### Must

- `ai-docs/tickets/ready/260524-feat-wsstore-runtime-metadata-migration-gate.md` — selected Phase 1 contract and acceptance shape.
- `ai-docs/mental-model/mcp-runtime.md` — wsstore metadata/control-plane boundary, short-transaction rule, and runtime coupling.
- `ai-docs/mental-model/named-agent-runtime.md` — current file-backed named-agent lifecycle, current-call state, and recovery behavior.
- `agents-plugin-tool/internal/wsstore/store.go` — SQLite state foundation, actor setup, prune/tombstone flow, and current open/write serialization.
- `agents-plugin-tool/internal/wsstore/store_test.go` — current same-process wsstore coverage and prune/tombstone behavior.
- `agents-plugin-tool/internal/wsagent/agent.go` — current `agent.json`, `current/state.json`, stream, event, and output file ownership.
- `agents-plugin-tool/internal/wsagent/agent_test.go` — current named-agent lifecycle and file-backed contract coverage.
- `agents-plugin-tool/internal/execjob/execjob.go` — current exec job file-backed state and durable stream layout.
- `agents-plugin-tool/internal/execjob/execjob_test.go` — current exec runtime reader and recovery coverage.
- `agents-plugin-tool/internal/mcp/server.go` — actor setup, child actor binding, and root/session recovery plumbing.
- `agents-plugin-tool/internal/mcp/server_test.go` — actor bootstrap and actor-bound root behavior coverage.

### Maybe

- `ai-docs/spec/mcp-tools.md` — canonical MCP runtime coupling and wsstore boundary language.
- `ai-docs/spec/named-agent-runtime.md` — spec mirror for file-backed named-agent behavior.
- `ai-docs/spec/plugin-runtime.md` — runtime contract metadata context for future launcher checks.
- `ai-docs/tickets/todo/260524-bug-wsstore-ci-sqlite-busy.md` — CI contention follow-up context for the current failure mode.
- `agents-plugin-tool/internal/wsstate/` — worktree layout basis for the SQLite path and future migration keys.
