# Brief: 260524-feat-wsstore-runtime-metadata-migration-gate Phase 3

## Intent

Migrate exec job lifecycle metadata behind the Phase 1 SQLite metadata gate so
exec records become SQLite-authoritative while public `exec.*` behavior stays
stable and command output payloads remain file-backed.

## Scope Boundary

Implement Phase 3 only: exec job metadata currently stored through the
file-backed exec runtime becomes SQLite-authoritative. Do not migrate
named-agent behavior, API-doc async jobs, or future model-backed exec output
questions in this slice. Do not move stdout, stderr, combined output, raw
command output bodies, runtime diagnostics, or future transcript bodies into
SQLite.

## Caller-Visible Contract

`exec.shell`, `exec.spawn`, `exec.status`, `exec.result`, `exec.abort`,
`exec.raw.tail`, `exec.raw.read`, and `exec.raw.grep` keep the same public MCP
and CLI behavior. Callers should not need to know whether job metadata came
from SQLite or an imported legacy file-backed record.

Launch calls still create an `exec_key`, start or foreground-complete a command,
persist stdout and stderr to job-owned files, report stream sizes, and return
bounded inline output only when the existing inline-budget rules allow it.
Status/result/abort/raw readers continue to expose lifecycle state, terminal
metadata, byte offsets, tails, grep results, and follow-up guidance.

## Contract Instructions

Use `internal/wsstore` for persisted exec job metadata and path indexes. SQLite
is the write authority for job identity, lifecycle status, command or argv
metadata, shell selection, working-directory metadata, environment/stdin
metadata, process id, lost-worker reconciliation state, timestamps, exit status,
stream path indexes, stream byte counts, retention visibility, tombstone/prune
eligibility, and cleanup state.

Keep stdout, stderr, combined output, raw command output bodies, runtime
diagnostics, and future transcript/model-readable output bodies file-backed.
SQLite may store paths and byte counts for these artifacts but must not store
their payload bytes.

Existing file-backed exec records need bounded compatibility behavior. Prefer
best-effort import into SQLite when the existing state metadata fits the Phase 1
boundary. If import cannot be completed, return a bounded recoverable state that
explains the record cannot be migrated. Do not keep a parallel JSON write
authority after migration.

Do not hold SQLite transactions while subprocesses run or while raw readers scan
large output files. Use brief metadata writes with the existing bounded
`SQLITE_BUSY`/`SQLITE_LOCKED` retry behavior.

## Integration Test Instructions

Extend package tests under `agents-plugin-tool/internal/execjob`,
`agents-plugin-tool/internal/wsstore`, and `agents-plugin-tool/internal/mcp` as
needed. Cover at least:

- fresh SQLite-backed exec launch through shell and structured spawn paths;
- `exec.status`, `exec.result`, `exec.abort`, and `exec.raw.*` behavior against
  SQLite-backed metadata;
- metadata survival across MCP process or manager restart;
- compatibility/import behavior for a pre-existing file-backed exec record;
- bounded recovery for corrupt or unimportable legacy metadata;
- missing stdout/stderr/combined stream payload reporting as recoverable
  file-backed payload consistency state;
- lost-worker reconciliation without leaving jobs indefinitely running;
- concurrent short metadata writes against the same worktree database;
- prune/tombstone eligibility metadata without deleting active or leased
  payload files.

Verification must include:

```bash
cd agents-plugin-tool
go test -count=1 ./internal/wsstore ./internal/execjob ./internal/mcp
```

Broaden to `go test -count=1 ./...` when the implementation touches CLI
mirrors, runtime capability metadata, wsstate, wsflow behavior, or shared MCP
formatting.

## Implementation Strategy Decisions

Follow the Phase 2 named-agent migration pattern: make SQLite metadata
authoritative, keep payload directories/files as the body store, and treat
legacy file-backed state as bounded read-only import input.

Prefer source-local wsstore tests or fixtures over importing execjob or mcp into
`internal/wsstore` tests, so future wiring does not create reverse-import
cycles.

Use logical metadata transitions for abort/result/cleanup. Physical payload file
removal belongs to prune/tombstone behavior and must skip active, running,
cancel-requested, leased, or pinned records.

## Rejected Alternatives

- Defer exec metadata to another ticket: rejected by user direction; Phase 3 is
  the remaining scope in this ticket.
- Keep file-backed exec JSON/state as a parallel write authority: rejected
  because Phase 3 requires SQLite-authoritative metadata.
- Move stream/output bytes into SQLite: rejected because stream payloads are
  append-heavy and raw readers require file-backed behavior.
- Use long-running SQLite transactions around subprocess execution or raw
  output scans: rejected because the Phase 1 gate requires short writes.

## Approach

- Add or extend wsstore exec job metadata operations and schema coverage.
- Wire execjob launch/status/result/abort/raw-reader metadata paths through the
  SQLite metadata authority.
- Keep stream files in the existing job-owned file locations and update SQLite
  path/byte-count metadata through short writes.
- Add read-only compatibility import or bounded recovery for existing
  file-backed exec records.
- Preserve MCP and CLI output contracts while adding tests around restart,
  missing payloads, lost workers, and concurrent metadata writes.

## Constraints

- Public `exec.*` schemas and command names must remain stable.
- Omitted `working_dir` still resolves through the ws root, and relative
  `working_dir` values stay constrained under the worktree root.
- Raw readers continue to read files by offset, tail, or grep; do not hide file
  payload errors behind generic SQLite errors.
- Active/running/cancel-requested/leased/pinned records must not be physically
  pruned by this migration.
- Native Windows behavior remains release-critical; when direct Windows
  execution is unavailable, tests must still cover path and cleanup abstractions
  without Unix-only assumptions.

## Out of scope

- Named-agent registry migration changes.
- API documentation async job migration.
- Model-backed `exec.ask` behavior.
- Moving stdout/stderr/combined output or transcript bodies into SQLite.
- Public schema or runtime capability additions unless required by preserving
  existing exec tool behavior.

## Details

Path strings and byte counts are SQLite metadata. Bytes at those paths are
file-backed payloads. Missing files at stored payload paths are recoverable
consistency states. A migrated record must never require callers to inspect a
legacy state file to determine lifecycle, stream size, or terminal status.

## Verification Contract

Required command:

```bash
cd agents-plugin-tool
go test -count=1 ./internal/wsstore ./internal/execjob ./internal/mcp
```

The implementer must report whether native Windows verification was run. If it
was not run in this environment, they must state that explicitly and ensure the
added tests do not encode Unix-only cleanup or path assumptions.

## References

- [Must] `ai-docs/spec/mcp-tools.md` - `exec.*` public behavior and runtime
  metadata migration gate.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP dispatch, exec surface,
  wsstore metadata boundary, and test guidance.
- [Must] `agents-plugin-tool/internal/execjob/` - exec runtime manager,
  lifecycle, raw readers, and tests.
- [Must] `agents-plugin-tool/internal/wsstore/` - SQLite schema, retry helper,
  migration inventory, and store tests.
- [Must] `agents-plugin-tool/internal/mcp/` - MCP exec tool dispatch and tests.
- [Must] `agents-plugin-tool/cmd/ws-mcp/` - CLI mirrors if touched.
- [Maybe] `ai-docs/mental-model/plugin-runtime.md` - runtime/wsflow contract
  coupling if tool visibility or runtime metadata changes.
- [Maybe] `ai-docs/spec/plugin-runtime.md` - runtime capability contract if
  launcher-visible metadata changes.
