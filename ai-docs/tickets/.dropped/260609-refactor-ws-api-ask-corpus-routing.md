---
title: ws api.ask redesign — corpus-routed api-doc playbook
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: api.ask redesign direction and rejected alternatives
  260609-feat-ws-playbook-surface-mvp: prerequisite — the api-doc playbook ships on the playbook surface
  260609-refactor-ws-spawn-runtime-deletion-session-auth: coordinated — api.ask spawn/async machinery dies with spawn removal
  260616-refactor-wsflow-product-mode-convergence: follow-up — after M4, collapse wsflow onto product-mode playbook rendering
related-mental-model:
  - api-documentation-cache
  - mcp-runtime
---

# ws api.ask redesign — corpus-routed api-doc playbook

## Dropped

Dropped 2026-06-16. The accepted M4 scope changed from redesigning `api.ask`
into a corpus-routed playbook methodology to removing the agent-backed
`api.ask` tool family from the playbook pivot entirely. The deletion work is
tracked by `260616-refactor-remove-agent-backed-api-tools`. The broader future
api namespace direction is tracked outside this epic by
`260616-epic-api-namespace-documentation-memory-tooling`.

## Background

Milestone M4 of the playbook-factory pivot (epic
`260605-epic-ws-playbook-factory-pivot`). The current `api.ask` architecture —
a model-judgment pre-router picks a domain, a per-domain manager session answers,
with runtime-owned cache access and stale checks — exists because ws-spawned
agents were expensive and domain sessions were durable. Under retained native
subagents that premise is gone, so routing moves from model judgment to corpus
design.

Full direction and rejected alternatives: `260605-research-ws-native-subagent-pivot`
("api.ask redesign — routing moves from model judgment to corpus design").

## Decisions

Binding decisions from the research ticket:

- **One autonomous native subagent explores the cache corpus directly.** The
  two-stage routing (pre-router + per-domain manager) is deleted.
- **api.ask becomes a dependency-documentation exploration method, not a special
  agent runtime.** The behavior should be expressed as a playbook and corpus
  convention that a general-purpose native subagent can follow. Any shell
  processing technique, documentation/modularization policy, cache traversal
  rule, and staleness rule belongs in the playbook/corpus contract rather than
  in an api-namespace model orchestration layer.
- **Routing → index file.** `api.list`'s role degrades into a cache-root
  `index.md` (domain list + one-line descriptions). The api-doc playbook
  instructs "read the index first, then descend into the domain doc". Routing
  accuracy becomes a property of index quality — a debuggable, human-fixable
  surface.
- **Durable domain-manager session continuity is dropped.** Its main value
  (avoiding cache re-reads) is cheap for an Explore-style agent.
- **Staleness → corpus metadata.** `fetched_at` / `source_url` frontmatter on
  cached docs; the playbook states the staleness rule. Runtime staleness logic
  goes to zero.
- **Async job machinery dies** (`api.ask_async` / `api.status` / `api.result` /
  `api.cancel`) with the rest of spawn removal; native background subagents
  replace it. Coordinated with M3.
- **Cache write discipline:** one file per domain, whole-file replacement, to
  preclude partial-update collisions (concurrency is low and doc caches tolerate
  last-write-wins).
- Net: the whole `api.*` surface reduces to one api-doc playbook plus a cache
  directory convention — small enough that the research judged it first-epic
  scope.
- **Do not solve wsflow convergence inside M4.** Finish the api-doc playbook
  methodology first. wsflow is treated as temporarily not usable until the
  follow-up product-mode convergence ticket removes the curated skill body /
  `prompt.render` split.

## Constraints

- Depends on M1: the api-doc playbook ships on the playbook surface
  (`playbook.print`/`render` + rsrc tree).
- Coordinated with M3: the `api.ask` spawn machinery and async job surface are
  removed in concert with spawn deletion; do not strand `api.*` callers between
  milestones.
- Caller-visible API contract change (`api.*` surface) — ask-first per repo
  Approval Protocol; ready promotion requires a contract-first spec.

## Phases

### Phase 1: corpus-routed api-doc playbook and cache convention

Add the api-doc playbook (read `index.md` first, descend into the domain doc,
apply the staleness rule). Define the cache-root `index.md` convention and the
`fetched_at` / `source_url` frontmatter, plus the one-file-per-domain
whole-file-replacement write discipline. Move api-specific shell processing
techniques and documentation/modularization policy into the playbook so a
general-purpose native subagent can perform dependency/API documentation
exploration without a bespoke api runtime agent. Reduce `api.list` to index
discovery. Remove the two-stage routing, per-domain manager sessions, and the
async job surface (coordinated with M3 spawn removal). Verification: a native
subagent answers an API question by reading the index then the domain doc;
staleness is decided from corpus metadata; shell/documentation policy is
recoverable from the playbook; no spawn/async machinery remains in `api.*`.

## Spec Impact

- Target spec area: `mcp-tools.md` — rewrite `#260505-api-documentation-mcp-tools`
  (api.ask becomes playbook-driven; api.list becomes index discovery) and remove
  `#260508-api-documentation-async-mcp-tools`. Update the `api-documentation-cache`
  mental model for the index/staleness/write conventions.
- Expected caller-visible change: `api.ask` answered by a corpus-routed native
  subagent via a general dependency-documentation exploration playbook; the
  async `api.*` surface removed; cache routing becomes an `index.md` convention.
- Contract-first spec: yes. Resolve at ready promotion via `lead-write-spec`.
