---
title: ws api.ask redesign — corpus-routed api-doc playbook
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: api.ask redesign direction and rejected alternatives
  260609-feat-ws-playbook-surface-mvp: prerequisite — the api-doc playbook ships on the playbook surface
  260609-refactor-ws-spawn-runtime-deletion-session-auth: coordinated — api.ask spawn/async machinery dies with spawn removal
related-mental-model:
  - api-documentation-cache
  - mcp-runtime
---

# ws api.ask redesign — corpus-routed api-doc playbook

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
whole-file-replacement write discipline. Reduce `api.list` to index discovery.
Remove the two-stage routing, per-domain manager sessions, and the async job
surface (coordinated with M3 spawn removal). Verification: a native subagent
answers an API question by reading the index then the domain doc; staleness is
decided from corpus metadata; no spawn/async machinery remains in `api.*`.

## Spec Impact

- Target spec area: `mcp-tools.md` — rewrite `#260505-api-documentation-mcp-tools`
  (api.ask becomes playbook-driven; api.list becomes index discovery) and remove
  `#260508-api-documentation-async-mcp-tools`. Update the `api-documentation-cache`
  mental model for the index/staleness/write conventions.
- Expected caller-visible change: `api.ask` answered by a corpus-routed native
  subagent via a playbook; the async `api.*` surface removed; cache routing
  becomes an `index.md` convention.
- Contract-first spec: yes. Resolve at ready promotion via `lead-write-spec`.
