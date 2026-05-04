---
title: ws-mcp workflow reference discovery tools
parent: 260503-epic-ws-mcp-vcs-reference-tools
related:
  260503-epic-ws-mcp-vcs-reference-tools: parent roadmap for portable Git and workflow reference tooling
completed: 2026-05-04
---

# ws-mcp workflow reference discovery tools

## Background

The ws MCP runtime now exposes portable Git primitives, including a constrained
workflow commit builder. The remaining reference portability gap is not ordinary
file reading or editing: agents can already use host-native file reads, edits,
and path moves. The gap is deterministic workflow reference discovery that
currently requires ad hoc shell combinations, manual frontmatter inspection,
and repeated `rg`/`sed` searches.

This ticket adds direct-scan MCP tools that report paths, statuses, stems,
frontmatter relationships, unresolved phases, spec anchors, and cross-reference
matches. The tools should help agents decide which native files to open or edit;
they should not become document editing APIs or broad content readers.

## Decisions

- Use direct filesystem scans for the first implementation. The corpus is small,
  and generated index invalidation would add more policy than value.
- Keep parameter names domain-specific: `ticket_stem` for tickets and
  `spec_stem` for spec anchors. Do not accept a generic `stem` parameter where
  both concepts may appear.
- Return relative paths and structured status metadata first. Include short
  snippets only for search matches that need disambiguation.
- Do not add tools whose primary job is "read this document body." Agents should
  use native file-reading capabilities after the discovery tool returns a path.
- Do not add editing, moving, status-transition, or spec/mental-model mutation
  behavior in this ticket.

## Constraints

- Do not update `ai-docs/spec/` or `ai-docs/mental-model/` on this branch.
- Preserve the existing ticket status directories and spec anchor conventions.
- Tools must be path-safe on Windows and avoid shell parsing.
- Archived ticket directories are opt-in separately: `include_done` includes
  `.done/`, and `include_dropped` includes `.dropped/`. Active-default
  discovery should favor `idea/`, `todo/`, and `wip/`.
- Outputs should be compact enough for workflow prompts to consume directly.

## Phases

### Phase 1: Ticket discovery tools

Add direct-scan ticket discovery primitives with CLI fallbacks and MCP schemas.
Suggested surface:

- `ws/tickets.list(statuses?: [...], include_done?: false, include_dropped?: false)`
- `ws/tickets.find(query?: "...", ticket_stem?: "...", mentions_ticket_stem?: "...")`
- `ws/tickets.status(ticket_stem: "...")`

The tools should return ticket paths, directory-derived status, title, parent,
related ticket stems, spec/spec-remove frontmatter, plans, skeletons, completed
date, unresolved phase headings, and result-present status. `tickets.find`
should support finding tickets that mention another `ticket_stem` without
requiring callers to compose shell searches.

Success criteria:

- Unit tests cover active scanning and the separate `include_done` /
  `include_dropped` flags.
- Unit tests distinguish `ticket_stem` from `spec_stem` naming.
- Integration tests cover MCP tool listing and calls.
- CLI smoke verifies a ticket mention query without shell pipelines.

### Result (pending) - 2026-05-04

Implemented the Phase 1 ticket discovery surface in the ws MCP runtime:

- Added path-first `tickets.list`, `tickets.find`, and `tickets.status` helpers
  that scan `ai-docs/tickets/` directly and return structured metadata rather
  than full document bodies.
- Exposed the helpers through MCP schemas and `ws-mcp tickets` CLI fallbacks.
- Kept ticket and spec identifiers distinct by using `ticket_stem` /
  `mentions_ticket_stem` and rejecting `spec_stem` arguments on ticket tools.
- Preserved separate archive gates: `include_done` controls `.done/`, and
  `include_dropped` controls `.dropped/`.

Verification: `cd agents-plugin-tool && go test ./...`; CLI smoke with
`go run ./cmd/ws-mcp tickets find --root .. --mentions-ticket-stem
260503-epic-ws-mcp-vcs-reference-tools`.

### Phase 2: Spec anchor discovery tools

Extend the existing direct spec anchor scan into path-first discovery tools.
Suggested surface:

- `ws/specs.list()`
- `ws/specs.find(query?: "...", spec_stem?: "...", ticket_stem?: "...")`
- `ws/specs.status(spec_stem: "...")`

The tools should return spec file paths, frontmatter title/summary, anchor
locations, nearest heading, planned/WIP marker context when detectable, and any
ticket-stem references found in frontmatter or feature entries. They should not
return full document bodies.

Success criteria:

- Unit tests cover duplicate file names, nested spec directories, and anchor
  lookup by `spec_stem`.
- MCP tests cover query and exact-anchor calls.
- Existing `ws/spec_stem.generate` and `ws/spec_index.verify` continue to pass.

### Result (pending) - 2026-05-04

Implemented the Phase 2 spec anchor discovery surface in the ws MCP runtime:

- Added path-first `specs.list`, `specs.find`, and `specs.status` helpers that
  scan `ai-docs/spec/` recursively and return metadata rather than full spec
  bodies.
- Returned spec file paths, duplicate-safe filenames, title/summary
  frontmatter, anchor locations with line/nearest heading, ticket references,
  and WIP/planned marker contexts.
- Exposed the helpers through MCP schemas and `ws-mcp specs` CLI fallbacks.
- Kept spec and ticket identifiers distinct: `spec_stem` selects anchors, while
  `ticket_stem` filters references only on `specs.find`.

Verification: `cd agents-plugin-tool && go test ./...`; CLI smoke with
`go run ./cmd/ws-mcp specs find --root .. --spec-stem 260421-plugin-json`.

### Phase 3: Mental-model reference discovery tools

Extend `ws/mental_models.list` with deterministic reference search and path
selection. Suggested surface:

- `ws/mental_models.find(query?: "...", spec_stem?: "...", domain?: "...")`
- `ws/mental_models.status(domain?: "...", path?: "...")`

The tools should return mental-model paths, domain, description, source
frontmatter, matching spec references, and ancestor/index hints. They should
guide agents toward the files to open natively rather than returning full
mental-model bodies.

Success criteria:

- Unit tests cover flat and nested mental-model layouts.
- Unit tests cover `domain` and `spec_stem` searches.
- MCP tests cover path-first results without full document body output.

### Result (pending) - 2026-05-04

Implemented the Phase 3 mental-model discovery surface in the ws MCP runtime:

- Added path-first `mental_models.find` and `mental_models.status` helpers that
  scan `ai-docs/mental-model/` recursively and return metadata rather than full
  mental-model bodies.
- Returned mental-model paths, domain, description, source frontmatter, matched
  spec refs, ancestor directory hints, nearby index hints, and short query
  snippets.
- Exposed the helpers through MCP schemas and `ws-mcp mental-models` CLI
  fallbacks while leaving `mental_models.list` intact.
- Kept filtering roles distinct: `spec_stem` is accepted on find, while status
  selects by `domain` or relative `path`.

Verification: `cd agents-plugin-tool && go test ./...`; CLI smoke with
`go run ./cmd/ws-mcp mental-models find --root .. --domain workflow-routing`.

### Phase 4: Cross-reference trace and workflow cleanup

Add one unified trace surface after the domain-specific tools are stable.
Suggested surface:

- `ws/references.trace(ticket_stem?: "...", spec_stem?: "...")`

Exactly one of `ticket_stem` or `spec_stem` must be supplied. The result should
compose ticket, spec, and mental-model discovery outputs into a compact graph of
paths and relationships.

After the trace surface exists, update `agents-plugin` skills and embedded
prompts to prefer these MCP tools over direct shell search for ws-owned
reference discovery. Native file reads, edits, and `git mv`-style path moves
remain host capabilities.

Success criteria:

- Tests reject calls that provide both `ticket_stem` and `spec_stem`.
- Tests cover trace results from ticket-to-spec and spec-to-mental-model links.
- Skill/prompt cleanup removes direct shell-search wording where the new MCP
  tools cover the workflow operation.

### Result (pending) - 2026-05-04

Implemented the Phase 4 cross-reference trace and cleanup slice:

- Added `references.trace` and `ws-mcp references trace`, requiring exactly one
  of `ticket_stem` or `spec_stem`.
- Composed the existing ticket, spec, and mental-model discovery helpers into a
  compact JSON graph without returning document bodies.
- Covered both ticket-to-spec-to-mental-model and spec-to-ticket-to-mental-model
  trace paths in unit tests and MCP integration tests.
- Updated `lead-workflow`, `lead-edit`, `code-reviewer`, and `impl-playbook`
  guidance to prefer path-first discovery tools where they now cover the
  workflow operation.

Verification: `cd agents-plugin-tool && go test ./...`; `git diff --check`;
CLI smoke with `go run ./cmd/ws-mcp references trace --root ..
--ticket-stem 260504-feat-ws-mcp-reference-discovery-tools`.
