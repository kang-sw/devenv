---
title: MCP actor setup state
related:
  260524-epic-async-exec-job-surface: async exec readers should align with actor-scoped runtime state
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
  - plugin-runtime
---

# MCP actor setup state

## Scope

Move ws MCP session recovery toward a cooperative actor model backed by
root-aware persistent state instead of process-local setup memory.

The epic owns decomposition for:

- actor-aware `ws.setup` bootstrap and recovery;
- root/worktree-scoped persistent metadata, preferably SQLite for transactional
  multi-process access;
- setup-required call-time gating for tools that need an actor or root context;
- child actor creation for `agents.call`, `subquery`, and later model-backed
  reader tools;
- lead workflow manual guidance that makes lead bootstrap explicit and
  recoverable after MCP restarts;
- automatic retention and pruning so persistent runtime metadata and
  file-backed artifacts do not grow without bound;
- gradual migration of async job and agent-call metadata toward actor-owned
  state and worker leases.

## Non-Scope

- A hard security boundary against a delegated model intentionally escalating.
- Multi-user authentication, remote authorization, or dashboard-owned ws MCP
  authority.
- Hiding every tool from `tools/list` before setup; initialize-time visibility
  may remain broader than call-time access.
- Moving large stdout, stderr, transcripts, or runtime logs into SQLite. The
  database should index metadata and file paths while append-heavy streams stay
  file-backed.
- Requiring environment-variable propagation from parent agents to nested MCP
  servers.

## Child Tickets

- `260524-feat-mcp-actor-setup-bootstrap` - introduce cooperative actor setup,
  lead bootstrap, setup recovery, setup-required tool gating, and minimal
  root-aware actor persistence.
- `260524-feat-mcp-child-actor-bootstrap` - create child actor ids for delegated
  agent calls and inject setup recovery instructions into child system prompts.
- `260524-feat-mcp-state-store-prune` - introduce the SQLite metadata store,
  retention model, and pruning foundation while leaving stream output files on
  disk.
- Planned: migrate exec and named-agent async metadata to the actor-owned
  SQLite lease model once the state store and setup model are stable.
- Planned: align model-backed readers such as `exec.ask` with actor-scoped
  context and visibility once the actor setup foundation exists.

## Cross-Child Decisions

- Treat actor authority as cooperative workflow state unless a future host
  launch boundary supplies trusted isolation. The model reduces accidental
  misuse and improves restart recovery; it does not defend against malicious or
  intentionally non-cooperative subagents.
- Do not accept `role: "lead"` or similar self-asserted elevation from
  `ws.setup`. Authority is a property of a stored actor id or a deliberate lead
  bootstrap method.
- Lead bootstrap uses an intentional soft-guard method documented for
  lead-facing workflow guidance, not delegate orientation or subquery prompts,
  such as `ws.setup(method: "lead-workflow-bootstrap", root: "<cwd>")` or an
  absolute root path.
- Setup guidance must avoid `"."` examples. Use an absolute path or the literal
  placeholder `"<cwd>"` so model callers do not confuse plugin cache cwd with
  the intended project root.
- `ws.setup(method: ...)` responses should print the actor id prominently and
  warn the lead not to forget it. Recovery guidance should say to call
  `ws.setup(id: "<actor-id>")` after MCP restart.
- Setup-gated tools may be advertised before setup, but calls should fail with
  actionable recovery text when no current actor is bound.
- Child actors are minted by parent lead workflows and injected into delegated
  system prompts as `ws.setup(id: "<child-actor-id>")` instructions on every
  call. Environment propagation is a best-effort fast path only, not the
  recovery mechanism.
- SQLite, when introduced, owns metadata transactions only. Long-running
  subprocess execution must not hold a database transaction open; workers should
  update status, byte counts, leases, and completion records with short writes.
- Auto-prune is part of the persistent-state contract. It must preserve active,
  running, cancel-requested, leased, and pinned state; large file-backed
  artifacts are removed through retention rules and retryable cleanup records,
  not by blindly deleting directories.
- Windows behavior is a first-class regression boundary for this epic because
  SQLite file access, process liveness, subprocess cleanup, and artifact
  deletion can diverge across platforms.

## Completion Criteria

- Done: ws MCP can recover lead and child actor context across MCP restarts,
  root-omitted privileged tool calls are guided through setup recovery, child
  agent calls receive actor setup instructions, persistent metadata has bounded
  retention, Windows regression coverage is recorded for the changed runtime
  paths, and async metadata has a clear actor-owned persistence path.
- Dropped: the repo chooses to keep process-local setup memory and prompt-only
  delegation conventions as the accepted long-term model.
- Deferred: hard security isolation, host-specific launch profiles, and full
  async job lease migration may move to later epics if cooperative actor setup
  ships first.
