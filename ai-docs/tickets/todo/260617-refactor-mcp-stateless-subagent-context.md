---
title: make MCP subagent context stateless and filesystem-backed
related:
  260605-research-ws-native-subagent-pivot: native-subagent pivot depends on predictable delegate access to workflow context
  260616-refactor-wsflow-product-mode-convergence: dogfood surfaced unknown session keys from rendered delegate prompts
  260617-refactor-ws-session-bootstrap-obscurity: independent lever — storage substrate here vs bootstrap-tool discovery obscurity there
related-mental-model:
  - mcp-runtime
  - prompt-bundle
---

# make MCP subagent context stateless and filesystem-backed

## Background

Dogfooding during wsflow product-mode convergence surfaced an unstable MCP
instance boundary: a native subagent that received a rendered playbook prompt
with a minted session key reported the key as `unknown_session` when it tried to
use ws MCP tools. The likely cause is that subagents sometimes receive a fresh
MCP server instance instead of sharing the lead's in-memory session registry.
The behavior appears harness-dependent or unstable, so in-memory lead-to-delegate
session state is not a reliable delegation contract.

The desired direction is to make MCP delegation context stateless from the
server process perspective. Data needed by subagents should be recoverable from
filesystem-backed artifacts or explicit prompt material, not from an in-memory
registry that assumes a shared MCP process.

## Decisions

- Treat subagent MCP process identity as unstable: a subagent may or may not
  share the lead's MCP server instance.
- Do not rely on render-minted in-memory session keys as the only path for
  subagent access to repository context.
- Prefer filesystem-backed context materialization for delegate prompts,
  credentials, root binding, and workflow handoff data where feasible.
- Keep this as a separate design ticket; do not block narrow runtime-surface
  cleanup work that can proceed without delegated MCP calls.
- **Concrete substrate: per-session filesystem store, not an in-memory map.**
  Replace the in-memory `sync.Map` session registry with per-session files
  (`keys/<session-id>.json` under the proj cache root). The word-chain session
  key is the filename; the JSON body holds `root`, `scope`, and
  `prefer_mercenary`, and is extensible to render lineage and future
  permission/capability metadata with no schema migration.
- **The file is the source of truth, not the process.** A subagent that starts
  with a fresh MCP instance, or a lead that restarts mid-delegation, resolves
  keys by reading the file. This dissolves both the lead-restart failure and the
  `unknown_session`-on-fresh-instance symptom in Background without
  reintroducing a shared in-memory registry.
- **Write profile.** Writes occur at login/render time (lead side); subagents
  are read-only key resolvers. Same-file writes use atomic temp-write + rename;
  sharding by session id removes cross-session write contention (no single-file
  lock).
- **Scope boundary.** This ticket covers the session/delegation context store
  only. Migrating exec job metadata off wsstore SQLite is a separate evaluation,
  not bundled here (exec has higher concurrent-write and status-tracking needs
  and heavier test coverage). SQLite is not categorically wrong; it is a poor
  fit for this workload profile (many short-lived processes, KV-shaped lookups,
  rare structured queries).

### Rejected Alternatives

- **Persist the in-memory map as-is (SQLite- or key-file-backed).** Reintroduces
  durable server-owned session state that the spawn-removal work deliberately
  shed; a single SQLite file revives the `260524` busy-contention surface.
- **Stateless signed capability tokens** (scope+root signed, server-validated,
  no storage). Survives restart but loses LLM-friendly word-chain keys, cannot
  revoke, and cannot carry extensible metadata — per-session files keep all
  three.

### Forward Guardrails

- spec `mcp-runtime.md` currently states "no persistence across server
  restarts"; Phase 3 must update it to describe the filesystem-backed store as
  the source of truth.
- Per-file storage makes eviction-by-delete physically possible; whether to open
  logout/eviction semantics stays a deferred decision (current model is
  no-logout, no-eviction).

## Phases

### Phase 1: characterize current subagent MCP instance behavior

Survey Codex and Claude native subagent MCP startup behavior. Record when a
subagent shares the lead MCP instance, when it starts a separate instance, and
which prompt/render flows currently depend on in-memory session continuity.
Verification: a short matrix with reproducible probes for same-instance,
fresh-instance, and unknown-session outcomes.

### Phase 2: design stateless filesystem-backed delegation context

Adopt the per-session filesystem store from `## Decisions` as the delegation
context contract. Finalize: file layout and proj-cache location, key→filename
mapping and sanitization, read-fresh vs cached read policy across instances, how
a fresh-instance subagent discovers its file, and which existing MCP tools stop
depending on per-process session registry continuity. Verification: spec or
mental-model updates identify the new source of truth and rejected alternatives.

### Phase 3: implement stateless delegate context path

Implement the chosen filesystem-backed or prompt-embedded context path and
update playbook rendering, MCP root resolution, tests, and docs accordingly.
Verification: native subagents can use rendered delegate prompts even when they
start with a fresh MCP server instance; existing lead-session behavior remains
compatible or has an explicit migration path.
