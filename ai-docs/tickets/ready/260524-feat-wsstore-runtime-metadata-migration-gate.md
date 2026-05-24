---
title: wsstore runtime metadata migration gate
related:
  260524-feat-mcp-state-store-prune: created the SQLite metadata foundation and future schema surface
  260524-bug-wsstore-ci-sqlite-busy: exposed same-database write contention during release CI
  260524-epic-async-exec-job-surface: exec job metadata is a candidate migration surface
  260524-feat-mcp-actor-setup-bootstrap: currently uses SQLite for actor setup and recovery
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
---

# wsstore runtime metadata migration gate

## Background

`internal/wsstore` already defines tables for actors, MCP sessions, named-agent
definitions and calls, exec jobs, worker leases, retention bookkeeping,
artifacts, prune runs, and tombstones. The current live SQLite write surface is
much narrower: actor setup and recovery use the store, while named-agent and
exec runtime records still rely on the existing file/JSON-backed state layouts.

The release of `v0.29.0` exposed `SQLITE_BUSY` during concurrent short writes
against the same state database. `v0.29.1` reduced same-process contention and
avoids repeated WAL setup on already-created databases, but that hotfix is not a
complete cross-process IPC contention strategy.

## Decisions

- Treat named-agent and exec metadata migration into SQLite as gated work, not a
  mechanical follow-up to the schema foundation.
- SQLite is acceptable for this migration only as a local lifecycle metadata
  catalog, not as an IPC bus, event stream, or high-frequency append log.
- Prefer SQLite over atomic JSON rewrites for agent and exec lifecycle
  metadata when the data benefits from transactional updates, indexed lookup,
  relationship tracking, crash recovery, stale-worker reconciliation, or
  retention queries.
- Actor-bound named-agent metadata should use the actor id as an internal
  namespace for agent names. Public `agents.*` inputs remain name-based, so
  `agents.register(name: "implementer")` keeps the same API shape while the
  stored identity becomes actor-local.
- Unbound or hidden explicit-root compatibility calls may preserve the existing
  worktree-global namespace during migration, but actor-bound calls should
  resolve actor-local agent names first.
- `agent.json` should not remain the long-term source of truth for agent
  metadata. Metadata fields that currently behave like lifecycle state, tags,
  flags, indexes, model-selection records, session ids, child actor bindings, or
  retention visibility should migrate into SQLite-backed rows.
- Agent and exec lifecycle removal should be logical removal plus retention
  eligibility, not immediate artifact deletion. Cancelled, erased, consumed
  ephemeral, or otherwise hidden runtime records should leave execution history
  available until prune policy removes it.
- Keep large append-heavy payloads file-backed: stdout, stderr, combined output,
  prompts, transcripts, runtime logs, and final result bodies should remain
  files with SQLite storing only identity, lifecycle, paths, byte counts,
  retention state, leases, and indexes.
- Before wiring high-frequency named-agent or exec writes to SQLite, define and
  test a cross-process write contention strategy for local worktree state
  databases.
- The contention strategy should cover database open/configuration, migrations,
  short write transactions, and opportunistic prune/tombstone writes.

## Constraints

- Preserve existing JSON/file-backed compatibility reads until each runtime
  surface has explicit migration and recovery coverage.
- Existing file-backed agent registrations do not require long-lived migration
  compatibility. If their metadata fits the SQLite metadata boundary above,
  implement the named-agent metadata path as a SQLite-authoritative path rather
  than preserving `agent.json` as a parallel source of truth.
- If `agent.json` is retained during migration, it must be read-only
  compatibility input or a generated diagnostic snapshot. It must not remain a
  write authority, and the implementation plan must include the condition that
  removes or disables any temporary compatibility reader.
- The migration must define the user-visible behavior for pre-existing
  file-backed agent registrations: ignored, best-effort imported once,
  tombstoned, or rejected with a bounded re-registration message. They must not
  disappear silently in a way that looks like state corruption.
- SQLite-backed state should be limited to metadata such as agent definition
  rows, current call or exec job lifecycle state, actor/session binding, worker
  leases, artifact indexes, retention policies, prune runs, and tombstones.
- Keep append-heavy or large data out of SQLite, including stdout, stderr,
  combined streams, agent event JSONL, prompt snapshots, final output bodies,
  runtime logs, transcripts, and backend raw output.
- `agents.cancel`, `agents.erase`, and ephemeral result consumption must not
  synchronously delete prompt, output, stream, event, or diagnostic artifacts.
  They should update lifecycle, visibility, tombstone, or retention metadata and
  let prune perform physical cleanup later.
- SQLite rows that point to file-backed artifacts need explicit consistency and
  recovery rules for partial failures, including orphaned files, missing
  payload paths, stale output pointers, partially registered calls, failed
  cleanup, and prune/tombstone retries.
- Pruning must skip active/running/cancel-requested/leased/pinned records and
  retain enough recent diagnostics for failed or cancelled agent and exec runs
  to be inspected after the public surface no longer lists them.
- Do not use SQLite as the coordination mechanism for every agent event or tool
  event. If a future design needs frequent multi-process event appends, choose
  a file-backed log, a single-writer daemon, or another explicit IPC boundary.
- Do not hold SQLite transactions while subprocesses or model calls are running.
- Handle `SQLITE_BUSY` and `SQLITE_LOCKED` with bounded retry/backoff or an
  equivalent repo/worktree writer coordination strategy.
- Include native Windows coverage because file locking and process-liveness
  behavior are release-critical for plugin users.
- Avoid moving stream contents into SQLite as a workaround; that increases lock
  pressure and weakens raw reader behavior.

## Spec Impact

Target spec areas: `ai-docs/spec/named-agent-runtime.md` and
`ai-docs/spec/mcp-tools.md`.

Expected caller-visible change: future named-agent and exec runtime metadata
storage may become SQLite-authoritative while public MCP APIs stay stable,
actor-scoped named-agent identity prevents common public names from colliding,
and result/diagnostic readers continue to expose file-backed payloads.

Contract-first spec: no. This ticket defines and implements the migration gate;
the implementation closeout should update the existing runtime specs once the
selected contention strategy, metadata field inventory, and first migration
surface are concrete.

## Phases

### Phase 1: Define the SQLite contention and migration gate

Turn this idea into an accepted backlog item by choosing the migration boundary:
whether `wsstore` gets a shared retry/backoff helper, a repo/worktree file lock
for migrations and WAL setup, a single writer-owner process model, or a
combination.

The selected boundary must explicitly state which named-agent and exec fields
become SQLite metadata and which existing files remain the source of truth for
streams, prompts, event logs, and result bodies.

For named agents, the phase must include a field-by-field inventory of
`agent.json`, `current/state.json`, and adjacent agent state files. Classify each
field as SQLite authoritative metadata, retained file-backed payload, or
temporary compatibility data. The intended end state is that `agent.json` is not
required as an authoritative metadata store. Metadata that meets the SQLite
boundary in this ticket should move cleanly to SQLite instead of being left in a
file-backed compatibility path.

It must also define the internal namespace rules for actor-bound named agents:
public `agents.*` names remain unchanged, actor id participates in persisted
identity for actor-bound registrations and calls, and compatibility behavior is
explicit for root-explicit or unbound callers.

The namespace design must pin down the unique key and lookup path for
actor-bound sessions, recovered sessions, hidden explicit-root compatibility,
and prune/retention queries so common public names do not collide across actors
or reappear as ambiguous records.

The phase should define acceptance tests that spawn independent processes or MCP
server instances against the same worktree state database and exercise actor
setup, child actor creation, exec job lifecycle writes, and prune/tombstone
bookkeeping without persistent `SQLITE_BUSY` failures. It should also include a
negative design check that rejects moving append-heavy streams or event logs
into SQLite as a contention workaround.

Named-agent migration tests should cover at least fresh SQLite-backed
registration, a pre-existing file-backed registration, concurrent mutable status
updates, actor-bound sessions registering the same public name, a SQLite row
whose file-backed payload is missing, temporary `agent.json` compatibility
removal, and native Windows file-locking/cleanup behavior.

### Result (43935812) - 2026-05-25

Implemented the Phase 1 gate in `internal/wsstore` without migrating
named-agent or exec runtime metadata into SQLite authority. The implementation
adds bounded `SQLITE_BUSY`/`SQLITE_LOCKED` retry around short configure,
migrate, and write paths while preserving process-local write serialization.

The runtime metadata inventory now classifies current `agent.json`,
`current/state.json`, and exec job `state.json` fields. Metadata fields,
including path indexes and byte counts, are SQLite-authoritative candidates;
payload bodies such as prompts, streams, logs, event JSONL, and final outputs
remain file-backed. `agent.json` compatibility is represented as bounded
read-only compatibility data rather than write authority.

Review fixes tightened the gate by proving contention tests observe a real
busy/locked retry before releasing a held transaction, making inventory coverage
exhaustive against current JSON-tag fields, and avoiding reverse imports from
`wsstore` tests into future wsstore consumers. Specs and mental models were
updated for the implemented migration gate and path-versus-payload boundary.

### Phase 2: Migrate one runtime surface behind the gate

After the gate exists, migrate either exec job metadata or named-agent metadata
as the first real consumer. Preserve file-backed streams and existing recovery
semantics, add compatibility reads where needed, and verify macOS/Linux plus
native Windows behavior. If named-agent metadata is the first surface, verify
that two actor-bound sessions can both register `implementer` without colliding
while legacy unbound behavior remains compatible. If exec metadata is first,
verify that cancel/result/erase-style cleanup is logical and that physical
artifact removal happens only through prune.
