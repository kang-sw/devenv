---
title: MCP actor setup bootstrap
parent: 260524-epic-mcp-actor-setup-state
related:
  260524-epic-async-exec-job-surface: exec tools should follow the same setup-gated actor model
  260524-feat-mcp-state-store-prune: supplies or constrains the persistent actor metadata store
related-mental-model:
  - mcp-runtime
  - plugin-runtime
---

# MCP actor setup bootstrap

## Background

`ws.setup` currently stores volatile root state in a single MCP server process.
That leaves restarted or parallel MCP processes without a recoverable actor
context, and it makes root-omitted tool calls depend on process-local memory.
The next setup model should be cooperative, persistent, and explicit about its
soft authority boundary.

## Decisions

- Extend `ws.setup` into an actor bootstrap and recovery surface.
- `ws.setup(method: "lead-workflow-bootstrap", root: "<cwd>")`, or the same
  call with an absolute root path, deliberately creates or resumes a lead actor
  for the current workflow conversation.
- `ws.setup(id: "<actor-id>")` restores a previously minted actor and binds it
  to the current MCP server process.
- Do not implement `ws.setup(role: "lead")` or any other self-asserted role
  elevation.
- `ws.setup()` without `method` or `id` may report limited setup state, but it
  must not mint a privileged lead actor.
- Setup output should prominently show the actor id and include recovery
  guidance such as: `Do not forget this actor_id. If MCP restarts, call
  ws.setup(id: "<actor-id>").`
- Lead workflow instructions should use an absolute path or `"<cwd>"` for root
  examples, not `"."`.

## Constraints

- This is a cooperative guard, not a security boundary against malicious
  delegated models.
- Tool visibility before setup may remain imperfect because MCP tool listing
  happens before model setup behavior. Enforcement should happen at tool-call
  time.
- Environment-variable propagation to nested MCP servers must not be required
  for correctness.
- Persistent actor metadata should be root/worktree aware and safe for multiple
  low-rate MCP processes. SQLite is preferred for transactional metadata; large
  append streams remain file-backed.
- Existing compatibility root setup behavior should remain recoverable during
  migration, but new privileged workflows should use actor setup.
- Windows behavior is in scope for regression testing because setup recovery
  must work across fresh MCP server processes and platform-specific filesystem
  behavior.

## Phases

### Phase 1: Add cooperative lead actor setup

Implement the lead actor bootstrap foundation:

- root/worktree-aware actor persistence with short transactional writes;
- `ws.setup(method: "lead-workflow-bootstrap", root: "<cwd>")`, or the same
  call with an absolute root path, for lead actor creation;
- `ws.setup(id: "<actor-id>")` for recovery after MCP restart;
- current-process actor binding for subsequent tool calls;
- setup-required call-time rejection for tools that need actor context;
- recovery messages that guide the caller to `ws.setup(id: "<actor-id>")` when
  an actor id is known, or to `lead-workflow-manual` when lead bootstrap is
  required;
- `lead-workflow-manual` bootstrap instructions and runtime/spec updates.

Verification should cover restart-style recovery with a fresh MCP server
instance, root guidance that uses absolute paths or `"<cwd>"`, id-less setup not
minting lead authority, setup-gated calls before setup, compatibility with
existing root-omitted tool behavior, and Windows coverage for the same setup and
recovery paths.
