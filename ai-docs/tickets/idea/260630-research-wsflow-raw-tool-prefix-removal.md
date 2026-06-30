---
title: Research wsflow raw MCP tool prefix removal
related:
  260605-research-ws-native-subagent-pivot: wsflow product identity and prompt-factory convergence anchor
  260627-bug-playbook-render-uses-stale-plugin-cache-during-source-dogfood: adjacent dogfood cache/surface surprise
related-mental-model:
  - mcp-runtime
  - prompt-bundle
  - workflow-skills
---

# Research wsflow raw MCP tool prefix removal

## Problem

During wsflow dogfooding, the raw MCP surface still advertises many tool names
with the `ws.` prefix even when the package/server namespace is `wsflow`.
Examples from `runtime capabilities` in no-agent mode:

- `ws.ferrule`
- `ws.workflow_manual`
- `ws.enter.proceed`
- `ws.enter.implement`
- `ws.todo.*`
- `ws.agenda.*`

The current contract says `WS_MCP_NAMESPACE=wsflow` changes user-facing
namespace text but does not rename generic MCP tool identifiers. That preserves
full-ws compatibility, but it leaks `ws` naming through the wsflow product API
and makes calls such as `wsflow/ws.ferrule(...)` read as double-namespaced.

## Observed Impact

- `wsflow` users see `ws.` raw tool identifiers in `tools/list` and runtime
  capability output.
- Workflow manual prose can still name `ws.ferrule` directly, which weakens the
  package's separate product identity.
- The ambiguity is especially visible for session bootstrap and state tools
  because they are invoked frequently and appear in errors and examples.

## Research Questions

1. Should wsflow expose product-local raw tool names without `ws.` prefixes,
   for example `ferrule`, `workflow_manual`, `enter.proceed`, and `todo.list`?
2. If yes, should full `ws` keep the current names while wsflow uses a
   product-mode alias table, or should both products move to neutral names?
3. What compatibility bridge is needed for existing wsflow installs and cached
   playbooks that still call `ws.*` raw tools?
4. Which specs, runtime contract files, launcher compatibility checks, tests,
   and rsrc playbooks must change together so tool discovery, calls, and
   documentation remain aligned?

## Constraints

- This is an MCP API/protocol surface change, not a wording-only cleanup.
- Exact wsflow runtime capability matching means any rename must update
  `agents-plugin-wsflow/runtime.json` and package tests in the same slice.
- Root-aware session tools have special safety behavior; renaming must preserve
  mandatory `session_key`, `root` bootstrap constraints, and role gating.
- Any compatibility aliases must not reintroduce hidden root fallback or weaken
  no-agent mode filtering.
