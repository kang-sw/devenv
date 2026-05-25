# Brief: 260524-feat-wsstore-runtime-metadata-migration-gate Phase 2

## Intent

Migrate named-agent registry metadata behind the Phase 1 SQLite metadata gate so
registered agent definitions become SQLite-authoritative while public
`agents.*` behavior stays stable and payload bodies remain file-backed.

## Scope Boundary

Implement Phase 2 only: named-agent registry and definition metadata currently
stored in `agent.json` becomes SQLite-authoritative. Defer exec job metadata
migration and defer moving append-heavy or body-like payload bytes into SQLite.
Current-call state may be touched only when required to preserve registration,
status, result, erase, or restart behavior for the migrated registry metadata.

## Caller-Visible Contract

`agents.register`, `agents.status`, `agents.call`, `agents.wait`,
`agents.result`, `agents.tail`, `agents.cancel`, `agents.erase`, and subquery
delegation keep their public MCP and CLI behavior. Actor-bound registrations
may use the same public name under different actor ids without colliding.
Unbound or explicit-root compatibility calls continue to resolve through the
global compatibility namespace.

Pre-existing file-backed registrations are handled explicitly and boundedly:
best-effort import into SQLite is preferred when the existing `agent.json`
metadata fits the Phase 1 metadata boundary. If import cannot be completed,
surface a bounded recovery or re-registration state instead of silently treating
the agent as missing. `agent.json` must not remain a parallel write authority.

## Contract Instructions

Use `internal/wsstore` for persisted registry metadata and actor-scoped internal
keys. Use the Phase 1 helper shape for actor-bound keys:
`actor:<escaped actor id>:name:<escaped public name>` for bound sessions and
`global:<escaped public name>` for unbound compatibility.

Persist migrated metadata fields that currently live in `agent.json`, including
backend/model selection, prompt references, materialized system-prompt path,
session id, lifecycle status, actor binding, timestamps, capability flags,
ephemeral visibility, and last-output path indexes. SQLite owns these metadata
writes after migration.

Keep prompt text, materialized system prompt bytes, stdout, stderr, runtime
logs, event JSONL, transcripts, and final output bodies file-backed. SQLite may
store path indexes and byte counts for these artifacts but must not store their
payload bytes.

Temporary `agent.json` handling may exist only as read-only compatibility input
for import or as a generated diagnostic snapshot. Do not add a new file-backed
metadata write path and do not preserve `agent.json` as the long-term source of
truth.

Do not hold SQLite transactions while subprocesses or model calls are running.
Use brief metadata writes and the existing bounded busy/locked retry gate.

## Integration Test Instructions

Extend package tests under `agents-plugin-tool/internal/wsagent`,
`agents-plugin-tool/internal/wsstore`, and `agents-plugin-tool/internal/mcp` as
needed. Cover at least:

- fresh SQLite-backed named-agent registration;
- pre-existing `agent.json` compatibility/import behavior;
- two actor-bound sessions registering the same public name without collision;
- unbound or hidden explicit-root compatibility lookup behavior;
- migrated registry metadata surviving MCP process or manager restart;
- missing file-backed payload path reporting as a recoverable consistency
  state;
- unchanged file-backed payload behavior for prompts, outputs, logs, and event
  JSONL.

Verification must include:

```bash
cd agents-plugin-tool
go test -count=1 ./internal/wsstore ./internal/wsagent ./internal/mcp
```

Broaden to `go test -count=1 ./...` if the implementation touches shared CLI,
runtime, config, or wsstate behavior outside the named-agent/store/MCP path.

## Implementation Strategy Decisions

Prefer a one-time read-only import path for existing `agent.json` records when
the metadata can be represented in SQLite. Preserve bounded compatibility for
unbound/global names, but actor-bound registrations must be actor-scoped in
SQLite from the start.

Use existing file-backed artifact paths for payload bodies. Treat missing files
at stored payload paths as recoverable consistency states surfaced through
status/result/debug behavior, not as justification to store payload bytes in
SQLite.

## Rejected Alternatives

- Keep `agent.json` as a second write authority: rejected because the ticket
  explicitly wants SQLite to become the metadata source of truth.
- Store prompt/output/log/event/final-output bytes in SQLite: rejected because
  Phase 1 classified those as file-backed payloads and moving them would
  increase lock pressure and weaken raw diagnostics.
- Key named agents only by public name: rejected because actor-bound sessions
  must be able to reuse common names such as `implementer`.
- Migrate exec job metadata in this slice: rejected because the user selected
  named-agent metadata first.

## Approach

- Add wsstore registry metadata operations and tests without importing future
  consumers into wsstore tests.
- Wire `wsagent.Manager` registration and registry reads through the SQLite
  metadata path while preserving file-backed payload paths.
- Add bounded compatibility import/recovery for existing file-backed
  `agent.json` records.
- Thread actor scope from MCP setup/root resolution into named-agent registry
  key selection without changing public `agents.*` schemas.
- Keep current-call, output, event, and diagnostic body files on disk.

## Constraints

- Public `agents.*` schemas remain rootless; hidden explicit-root compatibility
  behavior must not become the canonical public API.
- Existing model alias, backend selection, prompt resolution, child actor, and
  ephemeral result-consumption semantics must stay stable.
- Registration must not reset or overwrite an active current call.
- `Result` still requires terminal completion and an output file before
  returning final output.
- wsstore tests must avoid reverse imports from `wsagent`, `execjob`, or `mcp`.

## Out of scope

- Exec job metadata migration.
- Moving prompt/output/log/event/final-output bytes into SQLite.
- Changing public MCP schemas or CLI command names.
- Reworking backend runner invocation, cancellation, or event-log format beyond
  what is required to read/write migrated registry metadata.

## Details

The implementation should preserve the Phase 1 metadata inventory distinction:
path strings are SQLite metadata indexes even when the bytes at those paths are
file-backed payloads. Actor-bound registry lookups should resolve actor-local
names first. Global compatibility lookup exists for unbound or explicit-root
callers only.

When importing a legacy `agent.json`, avoid silent disappearance. Either create
the equivalent SQLite metadata row, preserving file-backed payload paths, or
return a bounded diagnostic/recovery state that tells callers the registration
must be recreated.

## Verification Contract

Required command:

```bash
cd agents-plugin-tool
go test -count=1 ./internal/wsstore ./internal/wsagent ./internal/mcp
```

The implementer must report any skipped Windows-native verification explicitly.
Where direct Windows execution is unavailable, include tests that exercise the
same path and file-locking abstractions without platform-specific assumptions.

## References

- [Must] `ai-docs/spec/named-agent-runtime.md` - named-agent registry metadata
  boundary and public lifecycle behavior.
- [Must] `ai-docs/spec/mcp-tools.md` - public `agents.*`, setup/root, and
  migration gate contracts.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - manager lifecycle,
  actor-scoped keying, payload boundary, and common mistakes.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - rootless MCP tool behavior,
  actor setup, wsstore retry gate, and metadata-only SQLite rule.
- [Must] `agents-plugin-tool/internal/wsagent/` - named-agent manager and tests.
- [Must] `agents-plugin-tool/internal/wsstore/` - SQLite state-store schema,
  retry helper, and migration inventory tests.
- [Must] `agents-plugin-tool/internal/wsstate/` - current file-backed path
  derivation and compatibility layout.
- [Must] `agents-plugin-tool/internal/mcp/` - MCP setup/root and `agents.*`
  dispatch behavior.
- [Maybe] `ai-docs/mental-model/prompt-bundle.md` - prompt reference and
  materialized system prompt semantics.
