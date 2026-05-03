---
title: agents-plugin sidecar skill drafts
parent: 260503-epic-agents-plugin-skill-porting
related:
  260429-research-host-neutral-ws-plugin: research anchor for host-neutral ws plugin architecture
  260503-feat-agents-plugin-runtime-boundary: MCP/runtime boundary for helper calls
completed: 2026-05-03
---

# agents-plugin sidecar skill drafts

## Background

The next migration slice is the lead-run sidecar productivity track. These skills
are single-session workflows with limited orchestration dependency, so they can be
ported together under the MCP-first mechanical porting policy.

This slice deliberately excludes `manual-think` and `workflow`. `manual-think` is
not a current migration target. `workflow` is unsafe to port before the Codex
delegation/runtime primitives exist because the Claude source document is a
session-resident reference to named-agent and PATH script primitives that do not
yet have a shared MCP contract.

## Phases

### Phase 1: Mental-model listing MCP surface

Add the minimal MCP surface needed by `add-rule`:

- `ws.mental_models.list`
- input: optional repository root
- output: mental-model document catalog with domain, path, description, and
  sources when available

Success criteria:

- `tools/list` advertises `ws.mental_models.list`.
- A direct JSON-RPC smoke call returns this repository's mental-model documents.

### Result (TBD) - 2026-05-03

Added `ws.mental_models.list` to `ws-mcp`. The tool renders a compact
`mental-models:` catalog with file paths, domains, descriptions, and sources.
`agents-plugin/runtime.json` now lists the tool as required so launcher
tool-surface drift checks can repair stale cache-local binaries.

### Phase 2: Port lead-run sidecar skills

Add host-neutral MCP-first draft skills under `agents-plugin/skills/`:

- `add-rule`
- `ship`
- `exit-session`

Success criteria:

- Skill bodies call MCP tools for conventions or catalog surfaces instead of
  repo-local plugin source paths or `ws-*` helpers.
- `workflow` and `manual-think` are not added in this slice.
- Source workflow wording and step shape are preserved where possible.

### Result (TBD) - 2026-05-03

Added draft MCP-first sidecar skills:

- `agents-plugin/skills/add-rule/SKILL.md`
- `agents-plugin/skills/ship/SKILL.md`
- `agents-plugin/skills/exit-session/SKILL.md`

`add-rule` uses `ws.convention.read` and `ws.mental_models.list` instead of
repo-local convention paths or CLI helpers. `ship` remains config-driven and
keeps the final user approval gate before publishing or pushing tags.
`exit-session` keeps the approval gate before committing `_index.md` and uses
`git commit -F` for multi-paragraph commit messages.

`manual-think` is excluded by user decision. `workflow` remains deferred because
the Claude source document is a named-agent/PATH primitive reference and the
shared Codex delegation runtime contract is not ready.

### Phase 3: Verification

Verify the plugin and runtime after adding the sidecar skills.

Success criteria:

- `go test ./...` passes in `agents-plugin-tool/`.
- MCP smoke tests pass.
- `claude plugin validate agents-plugin` passes.
- `git diff --check` passes.

### Result (TBD) - 2026-05-03

Verification:

- `go test ./...` from `agents-plugin-tool/`
- `scripts/smoke-ws-mcp.sh ..` from `agents-plugin-tool/`
- direct JSON-RPC `ws.mental_models.list` call through `ws-mcp serve --stdio`
- `jq . agents-plugin/runtime.json`
- inspected new sidecar skill files
- `claude plugin validate agents-plugin`
- `git diff --check`
