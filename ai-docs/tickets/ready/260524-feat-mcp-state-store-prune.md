---
title: MCP state store and pruning
parent: 260524-epic-mcp-actor-setup-state
related:
  260524-feat-mcp-actor-setup-bootstrap: uses the persistent actor state foundation
  260524-feat-mcp-child-actor-bootstrap: uses actor and child-call metadata
  260524-epic-async-exec-job-surface: exec jobs should move metadata into the shared state model
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
  - plugin-runtime
---

# MCP state store and pruning

## Background

Actor setup, named-agent calls, exec jobs, and future model-backed readers need
a shared root-aware metadata store that survives MCP restarts and parallel MCP
processes. The store should not grow forever and should not move append-heavy
stdout, stderr, transcripts, or runtime logs into SQLite.

## Decisions

- Use SQLite for root/worktree-aware runtime metadata, identity, lifecycle
  state, indexes, leases, and retention bookkeeping.
- Keep large append streams file-backed. SQLite rows should reference stream
  paths, byte counts, ownership, retention state, and timestamps.
- Candidate metadata areas include actors, MCP session bindings, named-agent
  definitions, agent calls, exec jobs, worker leases, artifact or stream
  indexes, inbox or interrupt records, settings, retention policies, prune runs,
  and deletion tombstones.
- Auto-prune should be part of the state-store design from the beginning.
- Pruning must never delete active, running, cancel-requested, leased, or pinned
  state.
- Pruning should run opportunistically with a small budget after setup or
  startup when the previous prune is stale; longer cleanup should remain a
  maintenance action.
- Transactions should be short. Long-running command or model execution must
  not hold a database transaction open.
- Cross-platform behavior, especially Windows file locking and process-liveness
  behavior, is a required regression boundary.

## Constraints

- Preserve existing file-backed stream reader behavior for stdout, stderr,
  combined exec output, agent stdout/stderr, runtime logs, prompts, and final
  output artifacts.
- Avoid a migration that forces all existing JSON state to disappear in one
  step. Compatibility reads or phased migration are acceptable when they reduce
  regression risk.
- SQLite access should be safe for multiple low-rate MCP processes on local
  disks. Network-synced or unreliable filesystem semantics are out of scope
  unless a later ticket adds explicit support.
- Prune file deletion and database row deletion must be recoverable. Failed
  file cleanup should leave retryable tombstones or equivalent bookkeeping.
- Do not let automatic pruning hide useful recent diagnostics while an agent,
  exec job, or reader session may still be relevant.

## Spec Impact

Target spec area: `mcp-tools`, `named-agent-runtime`, and `plugin-runtime`.

Expected caller-visible change: none in this foundation phase beyond preserving
current tool behavior while making future actor setup and async state recovery
possible. The implementation should not introduce new public MCP tools or
change existing tool arguments in Phase 1.

Contract-first spec: no. This phase establishes internal metadata storage,
retention, pruning, and regression coverage. Follow-up actor setup or async job
migration tickets own public contract changes.

## Phases

### Phase 1: Add metadata store and retention foundation

Introduce the shared state-store foundation and retention model without forcing
all agents or exec jobs to migrate at once.

The phase should establish:

- database location under the existing wsstate root/worktree layout;
- schema versioning and migration bookkeeping;
- short transaction helpers for root/worktree metadata updates;
- retention policy and prune-run records;
- artifact or stream metadata rows for file-backed payloads;
- tombstone or retry records for failed artifact cleanup;
- a small-budget opportunistic prune entry point;
- clear compatibility boundaries for existing JSON-backed agent and exec state.

Verification should include macOS/Linux unit tests and Windows test coverage for
database open/close behavior, concurrent short writes, file-backed stream paths,
prune skipping active state, prune cleanup of completed expired artifacts,
tombstone retry behavior, and no regression to existing named-agent and exec job
tests.
