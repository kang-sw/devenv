---
title: LLM-readable MCP output defaults
spec:
  - 260512-mcp-llm-readable-output-defaults
  - 260512-git-readable-output-defaults
  - 260512-metadata-trace-readable-output-defaults
  - 260512-documentation-discovery-readable-output-defaults
related-mental-model:
  - mcp-runtime
---

# LLM-readable MCP output defaults

## Background

Dogfooding showed that several ws MCP tools return JSON serialized into MCP text
content even when the primary consumer is an LLM. The shape is technically
usable, but it adds escaping, boilerplate field names, and large nested payloads
where compact prose or line-oriented text would be easier to read and cheaper to
carry through context.

The `mcp-runtime` mental model now records the governing rule: default MCP tool
responses should optimize for compact, readable LLM consumption unless stable
machine parsing, protocol metadata, or compatibility-preserving JSON is needed.

This ticket captures the concrete implementation surface. Promotion to `ready/`
should add planned MCP spec entries for the chosen default-output changes and
any new `format`, `detail`, or compatibility parameters.

## Decisions

- Prefer compact text as the default response for read-only discovery and status
  tools whose normal caller is an LLM.
- Preserve JSON where callers plausibly need stable machine parsing, launcher
  compatibility, or structured protocol metadata.
- Where compatibility risk exists, add an explicit `format: "json"` or similar
  escape hatch rather than removing structured output entirely.
- Keep mutation tool inputs structured. This ticket is about response shape, not
  loosening commit/config/job input contracts.

## Candidate Surfaces

- `git.status`: default to a branch/worktree summary plus changed-file lines;
  preserve enough status codes for safe commit hygiene.
- `git.diff`: default to raw diff/stat/name-only text instead of wrapping it in
  a JSON object with `mode` and `output`.
- `git.log`: render bounded commit blocks directly, including body text without
  JSON escaping when requested.
- `git.merge_base`: return a short line with the merge-base hash and inputs.
- `api.list`: return one domain per line.
- `runtime.info`, `config.show`, and `session.get_default_root`: return compact
  labeled text by default, while preserving JSON for compatibility probes if
  needed.
- `references.trace`: render ticket/spec/mental-model trace summaries in
  readable sections.
- `tickets.list`, `tickets.find`, and `tickets.status`: default to compact
  status lines with phases/snippets only when relevant or explicitly requested.
- `specs.list`, `specs.find`, and `specs.status`: default to file-level and
  matching-anchor summaries; avoid expanding every anchor on broad list calls
  unless a full-detail mode is requested.
- `mental_models.find` and `mental_models.status`: match the existing
  `mental_models.list` line-oriented style where possible.

## Phases

### Phase 1: Add output-format policy and helpers

Add or document shared formatting helpers for MCP text responses so individual
tools do not hand-roll inconsistent layouts. Decide the compatibility parameter
shape (`format`, `detail`, or both) before changing public defaults.

Success criteria:

- MCP spec entries describe the LLM-readable default policy and any JSON escape
  hatch.
- Tests cover text defaults and any compatibility JSON mode.
- Existing MCP error envelopes and `isError` behavior remain unchanged.

### Phase 2: Convert Git read surfaces

Change `git.status`, `git.diff`, `git.log`, and `git.merge_base` defaults to
compact text responses. Keep output precise enough for workflow safety: branch,
upstream/ahead/behind, clean/dirty state, changed file codes, diff content, and
commit metadata must remain visible.

Success criteria:

- `git.status` is readable at a glance for clean and dirty repositories.
- `git.diff` returns the actual selected diff text directly.
- `git.log` avoids JSON escaping in commit bodies.
- Tests exercise clean, dirty, untracked, and body-included log output.

### Phase 3: Convert small metadata and trace surfaces

Change small read-only metadata tools where text is clearly sufficient:
`api.list`, `session.get_default_root`, selected `runtime.info`/`config.show`
views, and `references.trace`. Preserve JSON where launchers or compatibility
checks rely on stable fields.

Success criteria:

- Human/LLM default output is compact and labeled.
- Launcher-facing compatibility data remains available and tested.
- Existing docs identify which surfaces remain structured intentionally.

### Phase 4: Convert documentation discovery surfaces

Change broad `specs.*`, `tickets.*`, and `mental_models.*` discovery/status
defaults to compact summaries, with optional full-detail or JSON mode for callers
that need anchors, phases, snippets, related maps, or source/spec reference
arrays.

Success criteria:

- Broad list/find calls avoid dumping large nested JSON by default.
- Exact status calls still make the important path, title, status, and matching
  anchor or phase visible.
- Full-detail behavior remains available for implementation workflows that need
  precise metadata without manual tree scans.
