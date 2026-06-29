---
title: design gap: session-scope prefer_mercenary=hide not reflected in tools list
sage-review: skipped
---

# design gap: session-scope prefer_mercenary=hide not reflected in tools list

## Background

`mercenaryHiddenFromConfig()` in `agents-plugin-tool/internal/mcp/server.go`
reads project and global config scopes only (empty session key). A lead who
sets `prefer_mercenary=hide` at session scope sees mercenary-on playbook blocks
suppressed correctly (the render path reads session scope), but
`ws.mercenary.*` tools remain in the tools list until the same value is set at
project or global scope.

The current server.go comment acknowledges this limitation. No regression: the
session is ephemeral, and `ws.lead.prefer_mercenary` stays visible regardless
(it has a `ws.lead.` prefix), so a lead can always change the setting.

## Direction

Selected: **Document only** — clarify in `config.workflow_prefer_mercenary` tool
description that tool-list suppression (`hide`) requires project or global scope;
session-scope `hide` suppresses mercenary playbook blocks but does not remove
`ws.mercenary.*` from the tools list.

Rejected: extending `mercenaryHiddenFromConfig` to accept a session key — deferred
to a future ticket if session-only hide becomes a common pattern.

## Phases

### Phase 1: Update tool description

Add a sentence to the `config.workflow_prefer_mercenary` tool description in
`agents-plugin-tool/internal/mcp/server.go` clarifying that `hide` at session
scope suppresses mercenary playbook blocks but does not remove `ws.mercenary.*`
tools from the tools list; project or global scope is required for tool-list
suppression.

Completion boundary: description text updated; no behavior change.

## Spec Impact

Target spec area: `mcp-tools.md` — `workflow.prefer_mercenary` config tool entry.
Expected caller-visible change: none (documentation clarification only).
Contract-first spec: no — post-implementation closeout updates the spec note if needed.
