---
title: ws MCP reference ownership cleanup
related-mental-model:
  - documentation-system
  - mcp-runtime
  - plugin-runtime
---

# ws MCP reference ownership cleanup

## Background

`ai-docs/ref/ws-mcp.md` has grown into a mixed document: runtime operations,
tool schemas, behavior contracts, implementation warnings, release notes, and
historical POC context. That makes it expensive to maintain and conflicts with
the documentation convention that source-derived facts should be read from
source, not copied into durable docs.

The desired ownership split is:

- specs own caller-visible behavior contracts;
- mental models own non-obvious modification coupling and common mistakes;
- references own operational runbooks, troubleshooting, and stable links to
  generated or runtime-discoverable inventory.

## Spec Impact

Target spec areas: documentation system, MCP tools, and plugin runtime.

Expected caller-visible change: workflow agents should treat `ref/ws-mcp.md` as
an operational runbook and link hub, not as the source of truth for MCP tool
contracts or current tool inventory. Tool schemas and tool lists remain
runtime-discoverable from code-backed surfaces such as `tools/list` and
`runtime capabilities`.

Contract-first spec: no. This ticket implements the documentation ownership
cleanup directly, and the implementation itself updates the affected specs.

## Phases

### Phase 1: Re-home ws MCP reference content

Reduce `ai-docs/ref/ws-mcp.md` to operational material that remains useful
without duplicating source-derived tool schemas. Move or summarize durable
behavior ownership into specs and modification-risk ownership into mental
models. Update project read-before-edit guidance so future sessions know which
document owns contract, modification, and runbook facts.

Verification boundary: spec index verification passes, repository status shows
only the intended documentation/ticket changes before commit, and source-derived
MCP tool schema or inventory blocks no longer remain in `ref/ws-mcp.md`.
