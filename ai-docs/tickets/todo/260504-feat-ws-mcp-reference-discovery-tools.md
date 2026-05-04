---
title: ws-mcp workflow reference discovery tools
parent: 260503-epic-ws-mcp-vcs-reference-tools
related:
  260503-epic-ws-mcp-vcs-reference-tools: parent roadmap for portable Git and workflow reference tooling
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
