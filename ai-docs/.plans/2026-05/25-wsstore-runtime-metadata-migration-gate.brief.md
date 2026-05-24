# Brief: 260524-feat-wsstore-runtime-metadata-migration-gate Phase 1

## Intent

Implement Phase 1 of `260524-feat-wsstore-runtime-metadata-migration-gate` by
making the runtime migration gate concrete in code and tests. The result should
define the SQLite contention boundary, classify named-agent and exec runtime
state into SQLite metadata versus file-backed payloads, and add verification
coverage that prevents future metadata migrations from treating SQLite as an
event stream or preserving `agent.json` as a long-lived write authority.

## Scope Boundary

Selected scope: Phase 1 only, "Define the SQLite contention and migration
gate".

Deferred scope: do not migrate named-agent metadata or exec job metadata as the
first real SQLite consumer. That is Phase 2. Phase 1 may add small helper types,
documentation constants, tests, or validation surfaces needed to express and
enforce the gate.

## Caller-Visible Contract

Public MCP APIs remain stable. The visible outcome is implementation-ready
runtime behavior and tests that establish:

- SQLite remains lifecycle metadata/control-plane storage only.
- Append-heavy payloads remain file-backed.
- `agent.json`, if retained during transition, cannot be a long-lived write
  authority for metadata that fits the SQLite boundary.
- Actor-bound named-agent identity can be specified in terms of actor-scoped
  internal keys while public `agents.*` names remain unchanged.
- Cross-process SQLite write contention is handled by a defined strategy before
  mutable runtime metadata writes depend on SQLite authority.

## Contract Instructions

Read and preserve:

- `ai-docs/tickets/ready/260524-feat-wsstore-runtime-metadata-migration-gate.md`
- `ai-docs/mental-model/mcp-runtime.md`
- `ai-docs/mental-model/named-agent-runtime.md`
- `agents-plugin-tool/internal/wsstore/`
- `agents-plugin-tool/internal/wsagent/`
- `agents-plugin-tool/internal/execjob/`
- `agents-plugin-tool/internal/mcp/`

Do not add a parallel root module directory. Use the existing
`agents-plugin-tool/internal/wsstore`, `wsagent`, `execjob`, and `mcp`
boundaries.

Do not make SQLite store stdout, stderr, combined output, prompts, transcripts,
runtime logs, backend raw output, agent event JSONL, or final output bodies.

Do not keep `agent.json` as a write authority for metadata that the field
inventory classifies as SQLite-authoritative.

## Integration Test Instructions

Add or extend Go tests around the concrete Phase 1 gate. Coverage should include
the strongest feasible local subset of:

- Independent process or independent store-handle writes against one worktree
  state database without persistent `SQLITE_BUSY`/`SQLITE_LOCKED`.
- Rejection or classification of append-heavy payloads as file-backed.
- Field inventory coverage for `agent.json`, `current/state.json`, and exec job
  `state.json`.
- Actor-scoped named-agent identity rules, including duplicate public names
  across actors.
- A row that points to a missing file-backed payload and the expected
  consistency/recovery classification.
- Temporary `agent.json` compatibility is not write authority.

Run the smallest focused test package first, then the relevant broader package
tests. Include native Windows coverage only when feasible from this environment;
otherwise preserve an explicit test or documented verification hook that can run
there later.

## Implementation Strategy Decisions

- Prefer a shared bounded retry/backoff helper around short SQLite write/open
  operations over a single-writer daemon for Phase 1. A daemon is more machinery
  than the current local metadata store needs and should remain a future option
  only if bounded retries and short transactions prove insufficient.
- Keep the existing process-local mutex; extend the strategy to tolerate
  independent processes or independent handles.
- Express the metadata gate in code close to `wsstore`, so future migrations can
  reuse one authoritative field classification and contention policy rather
  than rediscovering ticket text.
- Treat pre-existing file-backed named-agent registrations as volatile
  resources. If compatibility is implemented, make it bounded and read-only.

## Rejected Alternatives

- Single-writer daemon as the default Phase 1 answer: rejected for now because
  it adds a new IPC boundary before evidence shows bounded retry/backoff is
  insufficient.
- SQLite as event stream or payload store: rejected by ticket and mental-model
  constraints.
- Permanent `agent.json` compatibility: rejected because it preserves a
  surprising second metadata authority.
- Migrating a full runtime surface in Phase 1: rejected because Phase 1 is the
  gate; Phase 2 owns first consumer migration.

## Approach

- Inspect the current `wsstore` write/open path, schema, and tests.
- Add bounded busy/locked retry behavior for short writes and migration/open
  setup if missing.
- Add a small metadata classification model for named-agent and exec runtime
  state fields.
- Add tests for retry behavior, classification, actor-scoped identity shape, and
  payload/file-backed guardrails.
- Keep implementation scoped to Phase 1; do not wire `wsagent` or `execjob`
  runtime writes to SQLite as the authoritative path.

## Constraints

- SQLite transactions must stay short.
- Subprocess/model calls must never run while a SQLite transaction is held.
- Existing public MCP tool schemas stay stable.
- Hidden explicit-root compatibility remains an implementation bridge only.
- Tests must not depend on timing-only flakes for lock contention.

## Out of scope

- Full named-agent metadata migration.
- Full exec job metadata migration.
- Public MCP schema changes.
- Moving append-heavy file payloads into SQLite.
- Dashboard or activity console behavior.

## Details

Field classification should distinguish:

- SQLite authoritative metadata: identities, lifecycle states, actor/session
  binding, model-selection metadata, session ids, path indexes, byte counts,
  retention visibility, leases, tombstones, and prune bookkeeping.
- File-backed payloads: prompts, system prompt text, stdout, stderr, combined
  output, runtime logs, event JSONL, transcripts, backend raw output, and final
  output bodies.
- Temporary compatibility data: any old file-backed registration input used only
  to produce a bounded re-registration/import/tombstone behavior.

Actor-scoped identity should preserve public `agents.*` names while storing an
internal identity that includes actor id for actor-bound sessions. Compatibility
for unbound or hidden explicit-root callers must be explicit.

## Verification Contract

At minimum, run:

- `go test ./internal/wsstore` from `agents-plugin-tool/`

Run broader tests if implementation touches other packages:

- `go test ./internal/wsagent ./internal/execjob ./internal/mcp` from
  `agents-plugin-tool/`

Read full output before reporting pass.

## References

- [Must] `ai-docs/tickets/ready/260524-feat-wsstore-runtime-metadata-migration-gate.md` - selected Phase 1 contract.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - `wsstore` metadata/control-plane boundary, SQLite transaction rules, MCP coupling.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - current file-backed named-agent state and migration coupling.
- [Must] `agents-plugin-tool/internal/wsstore/` - SQLite store and pruning foundation.
- [Must] `agents-plugin-tool/internal/wsagent/` - named-agent metadata and current file layout.
- [Must] `agents-plugin-tool/internal/execjob/` - exec metadata and file-backed stream layout.
- [Maybe] `agents-plugin-tool/internal/mcp/` - actor setup, child actor, and exec/agent tool dispatch.
