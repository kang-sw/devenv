---
title: api namespace documentation and hierarchical memory tooling
related:
  260616-refactor-remove-agent-backed-api-tools: predecessor; frees the api namespace from agent-backed ask semantics
  260605-research-ws-native-subagent-pivot: pivot boundary that MCP tools should not own model-spawn orchestration
related-mental-model:
  - api-documentation-cache
  - documentation-system
  - mcp-runtime
  - workflow-skills
---

# api namespace documentation and hierarchical memory tooling

## Scope

Rebuild the `api.*` MCP namespace after the playbook-factory pivot as a
pure-tooling area for documentation, corpus, hierarchical memory, and agent
playbook-manual support. The namespace should expose deterministic operations
that agents and playbooks can compose, while reasoning and synthesis remain in
host-native subagents, ordinary lead context, or explicit mercenary delegation.

Candidate product areas:

- Dependency and external API documentation corpus management.
- Index and metadata tooling for local documentation caches.
- Hierarchical memory structures that are queryable and maintainable without
  hiding model delegation inside MCP calls.
- Agent playbook/manual support: generated or validated manuals, corpus maps,
  and source/staleness metadata that general-purpose agents can read directly.

## Non-Scope

- Reintroducing `api.ask`, `api.ask_async`, or an MCP-owned model routing layer.
- Blocking `260605-epic-ws-playbook-factory-pivot` M4 or the post-M4 wsflow
  convergence work.
- Designing downstream application-domain memory; this epic is for the ws
  workflow system and its documentation/tooling substrate.

## Child Tickets

- Planned: dependency-documentation corpus convention — define cache layout,
  index shape, source/staleness metadata, update discipline, and official-source
  citation expectations.
- Planned: hierarchical memory model — define layers, ownership, lookup
  semantics, update boundaries, and how memory entries relate to specs,
  tickets, mental models, and external documentation.
- Planned: api namespace tool inventory — choose deterministic MCP tools for
  listing, validating, indexing, and maintaining documentation/memory corpora.
- Planned: playbook/manual integration — define how workflow manual text and
  specialized playbooks instruct native agents to use the deterministic api
  tooling without embedding model calls in the tools.

## Cross-Child Decisions

- `api.*` is a pure tooling namespace: no tool in this epic owns agent
  delegation, manager-session continuity, or model-based routing.
- Agents remain responsible for reading, reasoning, and synthesis. MCP tools
  expose structured data operations, validation, indexing, metadata, and
  artifact generation.
- Corpus routing quality should be debuggable as data: indexes, metadata, and
  validation errors should be inspectable and manually fixable.
- Cache and memory writes should prefer whole-file or otherwise atomic,
  recoverable operations with clear ownership boundaries.

## Completion Criteria

- Done: child tickets define and implement a deterministic api namespace for
  documentation/memory/tooling support, and shipped workflow guidance uses it
  without reintroducing agent-backed `api.ask` behavior.
- Dropped: direction reversal is recorded here and in the predecessor deletion
  ticket.
- Deferred: downstream project-specific documentation stores and application
  memory systems leave through downstream tickets.
